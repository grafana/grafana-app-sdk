package operator

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// RetryRequest represents a single retry to be processed.
type RetryRequest struct {
	// Key is the unique identifier for the resource being retried, used for hash routing.
	Key string
	// RetryAfter is the earliest time at which this retry should be executed.
	RetryAfter time.Time
	// RetryFunc is the function to call when the retry is due. It returns an optional
	// requeue duration and an error.
	RetryFunc func() (*time.Duration, error)
	// Attempt is the current retry attempt number (0-based).
	Attempt int
	// Action is the resource action that originally triggered this retry.
	Action ResourceAction
	// Object is a snapshot of the resource at the time the retry was enqueued.
	Object resource.Object
	// LastError is the error from the most recent failed attempt.
	LastError error
}

// RetryProcessor manages concurrent retry processing with worker-pool sharding.
type RetryProcessor interface {
	// Enqueue adds a retry request. It is routed to a worker based on hash(key).
	Enqueue(req RetryRequest)
	// Dequeue removes items for the given key that match the predicate.
	Dequeue(key string, predicate func(RetryRequest) bool)
	// DequeueAll removes all items for a key.
	DequeueAll(key string)
	// Run starts all workers and blocks until ctx is canceled.
	Run(ctx context.Context) error
	// Len returns total number of pending retries across all workers.
	Len() int
}

// RetryProcessorConfig holds configuration for a RetryProcessor.
type RetryProcessorConfig struct {
	// WorkerPoolSize is the number of concurrent workers. Default: 4.
	WorkerPoolSize int
	// CheckInterval is how often workers check for due retries. Default: 1s.
	CheckInterval time.Duration
	// MetricsConfig controls the namespace and histogram settings for Prometheus metrics.
	// Metrics are always created; an empty Namespace produces unprefixed metric names.
	MetricsConfig metrics.Config
}

// NewRetryProcessor creates a new defaultRetryProcessor.
// retryPolicyFn is called to get the current RetryPolicy (allows dynamic updates).
func NewRetryProcessor(cfg RetryProcessorConfig, retryPolicyFn func() RetryPolicy) RetryProcessor {
	if cfg.WorkerPoolSize <= 0 {
		cfg.WorkerPoolSize = 4
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = time.Second
	}
	if retryPolicyFn == nil {
		retryPolicyFn = func() RetryPolicy { return nil }
	}

	workers := make([]*retryWorker, cfg.WorkerPoolSize)
	for i := range workers {
		workers[i] = &retryWorker{
			queue:         make(retryPriorityQueue, 0),
			wake:          make(chan struct{}, 1),
			checkInterval: cfg.CheckInterval,
		}
	}

	p := &defaultRetryProcessor{
		workers:       workers,
		workerCount:   uint64(cfg.WorkerPoolSize), //nolint:gosec
		retryPolicyFn: retryPolicyFn,
	}

	ns := cfg.MetricsConfig.Namespace
	m := &retryProcessorMetrics{
		enqueuedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: "retry_processor",
			Name:      "enqueued_total",
			Help:      "Total number of retry requests enqueued, by triggering action.",
		}, []string{"action"}),
		executionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Subsystem: "retry_processor",
			Name:      "executions_total",
			Help:      "Total number of retry executions by action and result. result: success=no error and no requeue, requeue=explicit RequeueAfter returned, retry=error with policy allowing another attempt, failed=error with policy exhausted.",
		}, []string{"action", "result"}),
		pendingTotal: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: ns,
			Subsystem: "retry_processor",
			Name:      "pending_total",
			Help:      "Current number of retry requests waiting across all workers.",
		}, func() float64 { return float64(p.Len()) }),
		waitDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       ns,
			Subsystem:                       "retry_processor",
			Name:                            "queue_wait_duration_seconds",
			Help:                            "Time between a retry's scheduled RetryAfter time and when it actually executes.",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"action"}),
	}
	p.processorMetrics = m
	for _, w := range p.workers {
		w.processorMetrics = m
	}

	return p
}

// defaultRetryProcessor implements RetryProcessor using a sharded worker pool.
type defaultRetryProcessor struct {
	workers          []*retryWorker
	workerCount      uint64
	retryPolicyFn    func() RetryPolicy
	processorMetrics *retryProcessorMetrics
}

// PrometheusCollectors implements metrics.Provider.
func (p *defaultRetryProcessor) PrometheusCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		p.processorMetrics.enqueuedTotal,
		p.processorMetrics.executionsTotal,
		p.processorMetrics.pendingTotal,
		p.processorMetrics.waitDuration,
	}
}

// Enqueue adds a retry request, routing it to a worker based on hash(key).
func (p *defaultRetryProcessor) Enqueue(req RetryRequest) {
	p.processorMetrics.incEnqueuedTotal(req.Action)

	w := p.workers[xxhash.Sum64([]byte(req.Key))%p.workerCount]
	w.mu.Lock()
	heap.Push(&w.queue, req)
	w.mu.Unlock()

	// Non-blocking wake signal
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Dequeue removes items for the given key that match the predicate.
func (p *defaultRetryProcessor) Dequeue(key string, predicate func(RetryRequest) bool) {
	if predicate == nil {
		p.DequeueAll(key)
		return
	}
	w := p.workers[xxhash.Sum64([]byte(key))%p.workerCount]
	w.mu.Lock()
	defer w.mu.Unlock()

	filtered := make(retryPriorityQueue, 0, len(w.queue))
	for _, req := range w.queue {
		if req.Key == key && predicate(req) {
			continue
		}
		filtered = append(filtered, req)
	}

	if len(filtered) != len(w.queue) {
		w.queue = filtered
		heap.Init(&w.queue)
	}
}

// DequeueAll removes all items for the given key.
func (p *defaultRetryProcessor) DequeueAll(key string) {
	w := p.workers[xxhash.Sum64([]byte(key))%p.workerCount]
	w.mu.Lock()
	defer w.mu.Unlock()

	filtered := make(retryPriorityQueue, 0, len(w.queue))
	for _, req := range w.queue {
		if req.Key != key {
			filtered = append(filtered, req)
		}
	}

	if len(filtered) != len(w.queue) {
		w.queue = filtered
		heap.Init(&w.queue)
	}
}

// Run starts all workers and blocks until ctx is canceled.
func (p *defaultRetryProcessor) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, w := range p.workers {
		wg.Go(func() {
			w.run(ctx, p.retryPolicyFn)
		})
	}
	wg.Wait()
	return nil
}

// Len returns total number of pending retries across all workers.
func (p *defaultRetryProcessor) Len() int {
	total := 0
	for _, w := range p.workers {
		w.mu.Lock()
		total += len(w.queue)
		w.mu.Unlock()
	}
	return total
}

// retryWorker processes retries for its shard.
type retryWorker struct {
	mu               sync.Mutex
	queue            retryPriorityQueue
	wake             chan struct{}
	checkInterval    time.Duration
	processorMetrics *retryProcessorMetrics
}

// run executes the worker loop, processing due retries on wake signals or periodic ticks.
func (w *retryWorker) run(ctx context.Context, retryPolicyFn func() RetryPolicy) {
	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
			w.processReady(retryPolicyFn)
		case <-ticker.C:
			w.processReady(retryPolicyFn)
		}
	}
}

// processReady pops all due items from the queue, executes them (unlocked), then re-enqueues
// failures according to the RetryPolicy.
func (w *retryWorker) processReady(retryPolicyFn func() RetryPolicy) {
	now := time.Now()

	// Phase 1 (LOCKED): Pop all items where retryAfter <= now
	w.mu.Lock()
	var ready []RetryRequest
	for w.queue.Len() > 0 && !w.queue[0].RetryAfter.After(now) {
		req, ok := heap.Pop(&w.queue).(RetryRequest)
		if !ok {
			continue
		}
		ready = append(ready, req)
	}
	w.mu.Unlock()

	if len(ready) == 0 {
		return
	}

	// Phase 2 (UNLOCKED): Execute each RetryFunc
	type result struct {
		req     RetryRequest
		requeue *time.Duration
		err     error
	}
	results := make([]result, len(ready))
	for i, req := range ready {
		w.processorMetrics.observeWaitDuration(req.Action, req.RetryAfter)
		requeue, err := req.RetryFunc()
		results[i] = result{req: req, requeue: requeue, err: err}
	}

	// Phase 3 (LOCKED): Re-enqueue failures per RetryPolicy or RequeueAfter
	w.mu.Lock()
	policy := retryPolicyFn()
	for _, res := range results {
		if res.requeue != nil {
			w.processorMetrics.incExecutionsTotal(res.req.Action, retryResultRequeue)
			heap.Push(&w.queue, RetryRequest{
				Key:        res.req.Key,
				RetryAfter: time.Now().Add(*res.requeue),
				RetryFunc:  res.req.RetryFunc,
				Attempt:    res.req.Attempt, // Keep attempt count for explicit requeue
				Action:     res.req.Action,
				Object:     res.req.Object,
				LastError:  res.err,
			})
		} else if res.err != nil {
			ok, after := false, time.Duration(0)
			if policy != nil {
				ok, after = policy(res.err, res.req.Attempt+1)
			}
			if ok {
				w.processorMetrics.incExecutionsTotal(res.req.Action, retryResultRetry)
				heap.Push(&w.queue, RetryRequest{
					Key:        res.req.Key,
					RetryAfter: time.Now().Add(after),
					RetryFunc:  res.req.RetryFunc,
					Attempt:    res.req.Attempt + 1,
					Action:     res.req.Action,
					Object:     res.req.Object,
					LastError:  res.err,
				})
			} else {
				w.processorMetrics.incExecutionsTotal(res.req.Action, retryResultFailed)
			}
		} else {
			w.processorMetrics.incExecutionsTotal(res.req.Action, retryResultSuccess)
		}
	}
	w.mu.Unlock()
}

// retryPriorityQueue implements heap.Interface, sorted by RetryAfter (min-heap).
type retryPriorityQueue []RetryRequest

func (pq retryPriorityQueue) Len() int           { return len(pq) }
func (pq retryPriorityQueue) Less(i, j int) bool { return pq[i].RetryAfter.Before(pq[j].RetryAfter) }
func (pq retryPriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
func (pq *retryPriorityQueue) Push(x any)        { *pq = append(*pq, x.(RetryRequest)) }
func (pq *retryPriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

// retryResultLabel values for the "result" label on retry_processor_executions_total.
const (
	retryResultSuccess = "success"
	retryResultRequeue = "requeue"
	retryResultRetry   = "retry"
	retryResultFailed  = "failed"
)

// retryProcessorMetrics holds the Prometheus metric instruments shared across a processor and its workers.
type retryProcessorMetrics struct {
	enqueuedTotal   *prometheus.CounterVec
	executionsTotal *prometheus.CounterVec
	pendingTotal    prometheus.GaugeFunc
	waitDuration    *prometheus.HistogramVec
}

func (m *retryProcessorMetrics) incEnqueuedTotal(action ResourceAction) {
	m.enqueuedTotal.WithLabelValues(string(action)).Inc()
}

func (m *retryProcessorMetrics) incExecutionsTotal(action ResourceAction, result string) {
	m.executionsTotal.WithLabelValues(string(action), result).Inc()
}

func (m *retryProcessorMetrics) observeWaitDuration(action ResourceAction, scheduledAt time.Time) {
	m.waitDuration.WithLabelValues(string(action)).Observe(time.Since(scheduledAt).Seconds())
}
