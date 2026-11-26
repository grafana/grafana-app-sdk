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
	// Dequeue cancels matching pending retries and prevents matching running retries
	// from being rescheduled. It does not wait for their callbacks to finish.
	Dequeue(key string, predicate func(RetryRequest) bool)
	// DequeueAll applies Dequeue to all retries for the key.
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
	return newRetryProcessor(cfg, retryPolicyFn)
}

func newRetryProcessor(cfg RetryProcessorConfig, retryPolicyFn func() RetryPolicy) *defaultRetryProcessor {
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
			active:        make(map[*retryAttempt]struct{}),
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
	p.enqueue(req, nil)
}

type queuedRetry struct {
	RetryRequest
	syncRetry *watcherSyncRetry
}

func (p *defaultRetryProcessor) enqueue(req RetryRequest, syncRetry *watcherSyncRetry) {
	p.processorMetrics.incEnqueuedTotal(req.Action)

	if req.Object != nil {
		// Dequeue can examine this snapshot while the callback is using its own object.
		req.Object = req.Object.Copy()
	}
	w := p.workers[xxhash.Sum64([]byte(req.Key))%p.workerCount]
	w.mu.Lock()
	heap.Push(&w.queue, queuedRetry{req, syncRetry})
	w.mu.Unlock()

	// Non-blocking wake signal
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (p *defaultRetryProcessor) Dequeue(key string, predicate func(RetryRequest) bool) {
	p.dequeue(key, func(req queuedRetry) (bool, bool) {
		discard := predicate == nil || predicate(req.RetryRequest)
		return discard, discard
	})
}

func (p *defaultRetryProcessor) DequeueAll(key string) {
	p.Dequeue(key, nil)
}

// dequeue returns completion signals for matching running callbacks. Callers
// that need to order another operation after them wait outside the worker lock.
func (p *defaultRetryProcessor) dequeue(key string, decide func(queuedRetry) (discard, wait bool)) []<-chan struct{} {
	w := p.workers[xxhash.Sum64([]byte(key))%p.workerCount]
	w.mu.Lock()
	defer w.mu.Unlock()

	filtered := make(retryPriorityQueue, 0, len(w.queue))
	for _, req := range w.queue {
		if req.Key == key {
			if discard, _ := decide(req); discard {
				continue
			}
		}
		filtered = append(filtered, req)
	}

	if len(filtered) != len(w.queue) {
		w.queue = filtered
		heap.Init(&w.queue)
	}

	var running []<-chan struct{}
	for attempt := range w.active {
		if attempt.request.Key != key {
			continue
		}
		discard, wait := decide(attempt.request)
		attempt.canceled = attempt.canceled || discard
		if attempt.running && (wait || attempt.canceled) {
			if attempt.done == nil {
				attempt.done = make(chan struct{})
			}
			running = append(running, attempt.done)
		}
	}
	return running
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
		for attempt := range w.active {
			if !attempt.started && !attempt.canceled {
				total++
			}
		}
		w.mu.Unlock()
	}
	return total
}

// retryWorker processes retries for its shard.
type retryWorker struct {
	mu               sync.Mutex
	queue            retryPriorityQueue
	active           map[*retryAttempt]struct{}
	wake             chan struct{}
	checkInterval    time.Duration
	processorMetrics *retryProcessorMetrics
}

// retryAttempt remains visible to dequeue until the worker decides whether to requeue it.
// The worker lock protects its execution state and the completion signal.
type retryAttempt struct {
	request  queuedRetry
	started  bool
	running  bool
	canceled bool
	done     chan struct{}

	// Only the goroutine processing this attempt accesses the requested requeue.
	requeue *time.Duration
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
	var ready []retryAttempt
	for w.queue.Len() > 0 && !w.queue[0].RetryAfter.After(now) {
		req, ok := heap.Pop(&w.queue).(queuedRetry)
		if !ok {
			continue
		}
		ready = append(ready, retryAttempt{request: req})
	}
	for i := range ready {
		w.active[&ready[i]] = struct{}{}
	}
	w.mu.Unlock()

	if len(ready) == 0 {
		return
	}

	for i := range ready {
		w.execute(&ready[i])
	}

	// Phase 3 (LOCKED): Re-enqueue failures per RetryPolicy or RequeueAfter
	w.mu.Lock()
	policy := retryPolicyFn()
	for i := range ready {
		attempt := &ready[i]
		delete(w.active, attempt)
		if !attempt.canceled {
			w.finish(attempt, policy)
		}
	}
	w.mu.Unlock()
}

func (w *retryWorker) execute(attempt *retryAttempt) {
	w.mu.Lock()
	if attempt.canceled {
		w.mu.Unlock()
		return
	}
	attempt.started = true
	attempt.running = true
	w.mu.Unlock()

	req := attempt.request
	w.processorMetrics.observeWaitDuration(req.Action, req.RetryAfter)
	requeue, err := req.RetryFunc()

	w.mu.Lock()
	attempt.requeue = requeue
	attempt.request.LastError = err
	attempt.running = false
	if requeue == nil && err == nil {
		delete(w.active, attempt)
	}
	if attempt.done != nil {
		close(attempt.done)
	}
	w.mu.Unlock()
}

// finish accounts for an attempt and reschedules it when needed. The caller holds the worker lock.
func (w *retryWorker) finish(attempt *retryAttempt, policy RetryPolicy) {
	req := attempt.request
	switch {
	case attempt.requeue != nil:
		w.processorMetrics.incExecutionsTotal(req.Action, retryResultRequeue)
		req.RetryAfter = time.Now().Add(*attempt.requeue)
	case req.LastError != nil:
		req.Attempt++
		if policy == nil {
			w.processorMetrics.incExecutionsTotal(req.Action, retryResultFailed)
			return
		}
		ok, after := policy(req.LastError, req.Attempt)
		if !ok {
			w.processorMetrics.incExecutionsTotal(req.Action, retryResultFailed)
			return
		}
		w.processorMetrics.incExecutionsTotal(req.Action, retryResultRetry)
		req.RetryAfter = time.Now().Add(after)
	default:
		w.processorMetrics.incExecutionsTotal(req.Action, retryResultSuccess)
		return
	}
	heap.Push(&w.queue, req)
}

// retryPriorityQueue implements heap.Interface, sorted by RetryAfter (min-heap).
type retryPriorityQueue []queuedRetry

func (pq retryPriorityQueue) Len() int           { return len(pq) }
func (pq retryPriorityQueue) Less(i, j int) bool { return pq[i].RetryAfter.Before(pq[j].RetryAfter) }
func (pq retryPriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
func (pq *retryPriorityQueue) Push(x any)        { *pq = append(*pq, x.(queuedRetry)) }
func (pq *retryPriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = queuedRetry{}
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
