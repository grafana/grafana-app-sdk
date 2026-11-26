package operator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

var _ Controller = &InformerController{}

type ResourceAction string

const (
	ResourceActionCreate = ResourceAction("CREATE")
	ResourceActionUpdate = ResourceAction("UPDATE")
	ResourceActionDelete = ResourceAction("DELETE")
	ResourceActionResync = ResourceAction("RESYNC")
)

// ErrNilObject indicates that a provided resource.Object is nil, and cannot be processed
var ErrNilObject = errors.New("object cannot be nil")

// ErrInformerAlreadyAdded indicates that there is already an informer for the resource kind mapped
var ErrInformerAlreadyAdded = errors.New("informer for resource kind already added")

// DefaultRetryPolicy is an Exponential Backoff RetryPolicy with an initial 5-second delay and a max of 5 attempts
var DefaultRetryPolicy = ExponentialBackoffRetryPolicy(5*time.Second, 5)

// DefaultErrorHandler is an error handler function which simply logs the error with the logger in the context
var DefaultErrorHandler = func(ctx context.Context, err error) {
	logging.FromContext(ctx).Error(err.Error(), "component", "InformerController", "error", err)
}

// Informer is an interface describing an informer which can be managed by InformerController
type Informer interface {
	app.Runnable
	WaitForSync(ctx context.Context) error
	AddEventHandler(handler ResourceWatcher) error
}

// ResourceWatcher describes an object which handles Add/Update/Delete actions for a resource
type ResourceWatcher interface {
	Add(context.Context, resource.Object) error
	Update(ctx context.Context, src, tgt resource.Object) error
	Delete(context.Context, resource.Object) error
}

// RetryPolicy is a function that defines whether an event should be retried, based on the error and number of attempts.
// It returns a boolean indicating whether another attempt should be made, and a time.Duration after which that attempt should be made again.
type RetryPolicy func(err error, attempt int) (bool, time.Duration)

// ExponentialBackoffRetryPolicy returns an Exponential Backoff RetryPolicy function, which follows the following formula:
// retry time = initialDelay * (2^attempt).
// If maxAttempts is exceeded, it will return false for the retry.
func ExponentialBackoffRetryPolicy(initialDelay time.Duration, maxAttempts int) RetryPolicy {
	return func(_ error, attempt int) (bool, time.Duration) {
		if attempt > maxAttempts {
			return false, 0
		}

		return true, initialDelay * time.Duration(math.Pow(2, float64(attempt)))
	}
}

// RetryDequeuePolicy is a function that defines when a retry should be dequeued when a new action is taken on a resource.
// It accepts information about the new action being taken, and information about the current queued retry,
// and returns `true` if the retry should be dequeued.
// A RetryDequeuePolicy may be called multiple times for the same action, depending on the number of pending retries for the object.
type RetryDequeuePolicy func(newAction ResourceAction, newObject resource.Object, retryAction ResourceAction, retryObject resource.Object, retryError error) bool

// OpinionatedRetryDequeuePolicy is a RetryDequeuePolicy which has the following logic:
// 1. If the newAction is a delete, dequeue the retry
// 2. If newObject is being deleted and retryObject is not, dequeue the retry unless it is a delete
// 3. If the newAction and retryAction are different, keep the retry (for example, a queued create retry, and a received update action)
// 4. If the generation of newObject and retryObject is the same, keep the retry
// 5. Otherwise, dequeue the retry
var OpinionatedRetryDequeuePolicy = func(newAction ResourceAction, newObject resource.Object, retryAction ResourceAction, retryObject resource.Object, _ error) bool {
	if newAction == ResourceActionDelete {
		return true
	}
	// The deletion supersedes the state the retry holds: replaying it after cleanup could recreate what cleanup removed.
	// A deletion need not change the generation, so rule 4 doesn't catch it. A delete retry belongs to an earlier
	// object with the same name, since retries are keyed by namespace and name, so it is kept.
	if retryAction != ResourceActionDelete && newObject != nil && retryObject != nil &&
		newObject.GetDeletionTimestamp() != nil && retryObject.GetDeletionTimestamp() == nil {
		return true
	}
	if newAction != retryAction {
		return false
	}
	if newObject.GetGeneration() == retryObject.GetGeneration() {
		return false
	}
	return true
}

// InformerController is an object that handles coordinating informers and observers.
// Unlike adding a Watcher directly to an Informer with AddEventHandler, the InformerController
// guarantees sequential execution of watchers, based on add order.
type InformerController struct {
	// ErrorHandler is a user-specified error handling function. This is typically for logging/metrics use,
	// as retry logic is covered by the RetryPolicy.
	ErrorHandler func(context.Context, error)
	// RetryPolicy is a user-specified retry logic function which will be used when ResourceWatcher function calls fail.
	RetryPolicy RetryPolicy
	// RetryDequeuePolicy is a user-specified retry dequeue logic function which will be used for new informer actions
	// when one or more retries for the object are still pending. If not present, existing retries are always dequeued.
	// If using an OpinionatedWatcher or OpinionatedReconciler, this should be set to OpinionatedRetryDequeuePolicy,
	// or something with similar logic, as the OpinionatedWatcher and OpinionatedReconciler trigger updates on adds which error,
	// which will cause the add retry to be dequeued if RetryDequeuePolicy is absent.
	RetryDequeuePolicy  RetryDequeuePolicy
	informers           *ListMap[string, *registeredInformer]
	watchers            *handlerList[ResourceWatcher]
	deliveryMu          sync.Mutex
	deliveries          map[deliveryKey]*deliveryOrder
	reconcilers         *handlerList[Reconciler]
	toRetry             *xsync.Map[string, *tickerRetryQueue]
	retryProcessor      *defaultRetryProcessor
	retryTickerInterval time.Duration
	runner              *app.DynamicMultiRunner
	totalEvents         *prometheus.CounterVec
	reconcileLatency    *prometheus.HistogramVec
	reconcilerLatency   *prometheus.HistogramVec
	watcherLatency      *prometheus.HistogramVec
	inflightActions     *prometheus.GaugeVec
	inflightEvents      *prometheus.GaugeVec
	reconcilerErrors    *prometheus.CounterVec
	watcherErrors       *prometheus.CounterVec
}

// InformerControllerConfig contains configuration options for an InformerController
type InformerControllerConfig struct {
	MetricsConfig metrics.Config
	// ErrorHandler is a user-specified error handling function. This is typically for logging/metrics use,
	// as retry logic is covered by the RetryPolicy. If left nil, DefaultErrorHandler will be used.
	ErrorHandler func(context.Context, error)
	// RetryPolicy is a user-specified retry logic function which will be used when ResourceWatcher function calls fail.
	// If left nil, DefaultRetryPolicy will be used.
	RetryPolicy RetryPolicy
	// RetryDequeuePolicy is a user-specified retry dequeue logic function which will be used for new informer actions
	// when one or more retries for the object are still pending. If not present, existing retries are always dequeued.
	// If left nil, no RetryDequeuePolicy will be used, and retries will only be dequeued when RetryPolicy returns false.
	RetryDequeuePolicy RetryDequeuePolicy
	// RetryProcessorConfig enables the concurrent retry processor when non-nil.
	// When set, retries are handled by a sharded worker pool instead of the legacy ticker-based approach.
	RetryProcessorConfig *RetryProcessorConfig
}

// DefaultInformerControllerConfig returns an InformerControllerConfig with default values
func DefaultInformerControllerConfig() InformerControllerConfig {
	return InformerControllerConfig{
		MetricsConfig:      metrics.DefaultConfig(""),
		ErrorHandler:       DefaultErrorHandler,
		RetryPolicy:        DefaultRetryPolicy,
		RetryDequeuePolicy: OpinionatedRetryDequeuePolicy,
	}
}

// NewInformerController creates a new controller
func NewInformerController(cfg InformerControllerConfig) *InformerController {
	inf := &InformerController{
		RetryPolicy:         DefaultRetryPolicy,
		ErrorHandler:        DefaultErrorHandler,
		informers:           NewListMap[*registeredInformer](),
		watchers:            newHandlerList[ResourceWatcher](),
		deliveries:          make(map[deliveryKey]*deliveryOrder),
		reconcilers:         newHandlerList[Reconciler](),
		toRetry:             xsync.NewMap[string, *tickerRetryQueue](),
		retryTickerInterval: time.Second,
		runner:              app.NewDynamicMultiRunner(),
		reconcileLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.MetricsConfig.Namespace,
			Subsystem:                       "informer",
			Name:                            "reconcile_duration_seconds",
			Help:                            "Time (in seconds) spent performing all reconcile actions for an event.",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"event_type", "kind"}),
		reconcilerLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.MetricsConfig.Namespace,
			Subsystem:                       "reconciler",
			Name:                            "process_duration_seconds",
			Help:                            "Time (in seconds) spent performing individual reconciler actions.",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"event_type", "kind"}),
		watcherLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       cfg.MetricsConfig.Namespace,
			Subsystem:                       "watcher",
			Name:                            "process_duration_seconds",
			Help:                            "Time (in seconds) spent perfoming individual watcher actions.",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"event_type", "kind"}),
		totalEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:      "events_total",
			Subsystem: "informer",
			Namespace: cfg.MetricsConfig.Namespace,
			Help:      "Total number of informer events",
		}, []string{"event_type", "kind"}),
		inflightActions: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:      "ongoing_reconcile_processes",
			Namespace: cfg.MetricsConfig.Namespace,
			Help:      "Current number of ongoing reconciliation (reconciler or watcher) processes",
		}, []string{"event_type", "kind"}),
		inflightEvents: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:      "ongoing_reconcile_events",
			Namespace: cfg.MetricsConfig.Namespace,
			Help:      "Current number of events which have active reconcile processes",
		}, []string{"event_type", "kind"}),
		reconcilerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Subsystem: "reconciler",
			Name:      "errors_total",
			Help:      "Total number of reconciler errors, before any retry.",
		}, []string{"event_type", "kind"}),
		watcherErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Subsystem: "watcher",
			Name:      "errors_total",
			Help:      "Total number of watcher errors, before any retry.",
		}, []string{"event_type", "kind"}),
	}
	if cfg.ErrorHandler != nil {
		inf.ErrorHandler = cfg.ErrorHandler
	}
	if cfg.RetryPolicy != nil {
		inf.RetryPolicy = cfg.RetryPolicy
	}
	if cfg.RetryDequeuePolicy != nil {
		inf.RetryDequeuePolicy = cfg.RetryDequeuePolicy
	}
	if cfg.RetryProcessorConfig != nil {
		retryPolicyFn := func() RetryPolicy { return inf.RetryPolicy }
		processorCfg := *cfg.RetryProcessorConfig
		// Inherit the controller's MetricsConfig so all metrics share a consistent namespace.
		processorCfg.MetricsConfig = cfg.MetricsConfig
		inf.retryProcessor = newRetryProcessor(processorCfg, retryPolicyFn)
	}
	return inf
}

// AddInformer adds an informer for a specific resourceKind.
// The `resourceKind` string is used for internal tracking and correlation to observers,
// and does not necessarily need to match the informer's type.
//
// Multiple informers may be added for the same resource kind,
// and each will trigger all watchers for that resource kind.
// The most common usage of this is to have informers partitioned by namespace or labels for the same resource kind,
// which share a watcher.
//
//nolint:gocognit,funlen,dupl
func (c *InformerController) AddInformer(informer Informer, resourceKind string) error {
	if informer == nil {
		return errors.New("informer cannot be nil")
	}
	if resourceKind == "" {
		return errors.New("resourceKind cannot be empty")
	}

	source := &registeredInformer{Informer: informer}
	err := informer.AddEventHandler(&SimpleWatcher{
		AddFunc: func(ctx context.Context, obj resource.Object) error {
			return c.informerChange(ctx, resourceKind, source, ResourceActionCreate, nil, obj)
		},
		UpdateFunc: func(ctx context.Context, old, obj resource.Object) error {
			return c.informerChange(ctx, resourceKind, source, ResourceActionUpdate, old, obj)
		},
		DeleteFunc: func(ctx context.Context, obj resource.Object) error {
			return c.informerChange(ctx, resourceKind, source, ResourceActionDelete, nil, obj)
		},
	})
	if err != nil {
		return err
	}

	c.runner.AddRunnable(source)
	c.informers.AddItem(resourceKind, source)
	return nil
}

// RemoveInformer removes the provided informer, stopping it if it is currently running.
func (c *InformerController) RemoveInformer(informer Informer, resourceKind string) {
	c.informers.RemoveItem(resourceKind, func(i *registeredInformer) bool {
		if i.Informer != informer {
			return false
		}
		c.runner.RemoveRunnable(i)
		return true
	})
}

// AddWatcher adds an observer to an informer with a matching `resourceKind`.
// Any time the informer sees an add, update, or delete, it will call the observer's corresponding method.
// Multiple watchers can exist for the same resource kind.
// They will be run in the order they were added to the informer.
func (c *InformerController) AddWatcher(watcher ResourceWatcher, resourceKind string) error {
	if watcher == nil {
		return errors.New("watcher cannot be nil")
	}
	if resourceKind == "" {
		return errors.New("resourceKind cannot be empty")
	}
	c.watchers.add(resourceKind, watcher)
	return nil
}

// RemoveWatcher removes the given ResourceWatcher from the list for the resourceKind, provided it exists in the list.
func (c *InformerController) RemoveWatcher(watcher ResourceWatcher, resourceKind string) {
	c.watchers.remove(resourceKind, watcher)
}

// RemoveAllWatchersForResource removes all watchers for a specific resourceKind
func (c *InformerController) RemoveAllWatchersForResource(resourceKind string) {
	c.watchers.removeAll(resourceKind)
}

// AddReconciler adds a reconciler to an informer with a matching `resourceKind`.
// Any time the informer sees an add, update, or delete, it will call reconciler.Reconcile.
// Multiple reconcilers can exist for the same resource kind. If multiple reconcilers exist,
// they will be run in the order they were added to the informer.
func (c *InformerController) AddReconciler(reconciler Reconciler, resourceKind string) error {
	if reconciler == nil {
		return errors.New("reconciler cannot be nil")
	}
	if resourceKind == "" {
		return errors.New("resourceKind cannot be empty")
	}
	c.reconcilers.add(resourceKind, reconciler)
	return nil
}

// RemoveReconciler removes the given Reconciler from the list for the resourceKind, provided it exists in the list.
func (c *InformerController) RemoveReconciler(reconciler Reconciler, resourceKind string) {
	c.reconcilers.remove(resourceKind, reconciler)
}

// RemoveAllReconcilersForResource removes all Reconcilers for a specific resourceKind
func (c *InformerController) RemoveAllReconcilersForResource(resourceKind string) {
	c.reconcilers.removeAll(resourceKind)
}

// Run runs the controller, which starts all informers, until stopCh is closed
//
//nolint:errcheck
func (c *InformerController) Run(ctx context.Context) error {
	// Using derivedCtx ensures that if c.runner exits prematurely due to an error, c.retryTicker will also stop
	derivedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if c.retryProcessor != nil {
		go c.retryProcessor.Run(derivedCtx) //nolint:errcheck
	} else {
		go c.retryTicker(derivedCtx)
	}
	return c.runner.Run(ctx)
}

// PrometheusCollectors returns the prometheus metric collectors used by this informer, as well as collectors used by
// any registered informer or watcher which implements metrics.Provider, to allow for registration
func (c *InformerController) PrometheusCollectors() []prometheus.Collector {
	collectors := []prometheus.Collector{
		c.totalEvents, c.reconcileLatency, c.inflightEvents, c.inflightActions, c.reconcilerLatency, c.watcherLatency,
		c.reconcilerErrors, c.watcherErrors,
	}
	if c.retryProcessor != nil {
		collectors = append(collectors, c.retryProcessor.PrometheusCollectors()...)
	}
	c.informers.RangeAll(func(_ string, _ int, source *registeredInformer) {
		value := source.Informer
		if cast, ok := value.(metrics.Provider); ok {
			collectors = append(collectors, cast.PrometheusCollectors()...)
		}
	})
	c.watchers.RangeAll(func(_ string, _ int, reg *handlerRegistration[ResourceWatcher]) {
		value := reg.handler
		if cast, ok := value.(metrics.Provider); ok {
			collectors = append(collectors, cast.PrometheusCollectors()...)
		}
	})
	return collectors
}

// HealthChecks
func (c *InformerController) HealthChecks() []health.Check {
	checks := make([]health.Check, 0, c.informers.Size()+c.watchers.Size()+c.reconcilers.Size())

	c.informers.RangeAll(func(_ string, _ int, source *registeredInformer) {
		value := source.Informer
		if cast, ok := value.(health.Check); ok {
			checks = append(checks, cast)
		}

		if cast, ok := value.(health.Checker); ok {
			checks = append(checks, cast.HealthChecks()...)
		}
	})
	c.watchers.RangeAll(func(_ string, _ int, reg *handlerRegistration[ResourceWatcher]) {
		value := reg.handler
		if cast, ok := value.(health.Check); ok {
			checks = append(checks, cast)
		}

		if cast, ok := value.(health.Checker); ok {
			checks = append(checks, cast.HealthChecks()...)
		}
	})
	c.reconcilers.RangeAll(func(_ string, _ int, reg *handlerRegistration[Reconciler]) {
		value := reg.handler
		if cast, ok := value.(health.Check); ok {
			checks = append(checks, cast)
		}

		if cast, ok := value.(health.Checker); ok {
			checks = append(checks, cast.HealthChecks()...)
		}
	})
	return checks
}

// informerChange delivers each notification to watchers, then reconcilers, in registration order.
func (c *InformerController) informerChange(ctx context.Context, kind string, source *registeredInformer, action ResourceAction, old, obj resource.Object) (err error) {
	if obj == nil {
		return ErrNilObject
	}
	metadata := obj.GetStaticMetadata()
	resourceKind := metadata.Kind
	start := c.startEvent(string(action), resourceKind)
	defer c.completeEvent(string(action), resourceKind, start)
	var spanName string
	switch action {
	case ResourceActionUpdate:
		spanName = "controller-event-update"
	case ResourceActionDelete:
		spanName = "controller-event-delete"
	default:
		spanName = "controller-event-add"
	}
	ctx, span := GetTracer().Start(ctx, spanName)
	defer span.End()

	var interrupted error
	run := source.run.Load()
	var pending []deferredDelivery
	order, joined := c.beginDelivery(deliveryKey{kind, metadata.Identifier()}, action, obj, run)
	defer func() {
		c.finishDelivery(order, run, pending)
		if joined && err == nil {
			err = waitForRetries(ctx, []<-chan struct{}{order.done})
		}
	}()
	prior := order.previous
	timeout := informerEventTimeoutDuration(ctx)
	deliver := func(position cleanupOrder, owner context.Context, key string, retryAction ResourceAction, cleanup, orderActive bool,
		call func(context.Context, resource.Object, resource.Object, time.Duration) error) {
		if owner.Err() != nil {
			return
		}
		running, recheck, err := c.deliveryRetryWait(ctx, key, obj, retryAction, prior, interrupted)
		interrupted = err
		if !joined && interrupted == nil {
			if cleanup || orderActive || prior != nil {
				interrupted = waitForRetries(ctx, running)
			}
			if interrupted == nil {
				if owner.Err() != nil {
					return
				}
				interrupted = call(ctx, old, obj, 0)
				if interrupted == nil {
					return
				}
			}
		}
		live := run.workContext(owner)
		if live == nil {
			return
		}
		previous, target := old, obj.Copy()
		if old != nil {
			previous = old.Copy()
		}
		pending = append(pending, deferredDelivery{order: position, cleanup: cleanup, owner: owner, live: live, running: running, dispatch: func(live context.Context) {
			// A joined notification runs behind earlier callbacks in this
			// queue, including any retry they schedule before returning.
			if (recheck || joined) && waitForRetries(live, c.dequeueIfRequired(key, target, retryAction)) != nil {
				return
			}
			if cleanup {
				live = context.WithValue(live, refreshCleanupKey{}, true)
			}
			_ = call(live, previous, target, timeout)
		}})
	}

	// The serial ticker orders every same-key event behind the running callback.
	orderActive := action == ResourceActionDelete || c.retryProcessor == nil
	c.watchers.Range(kind, func(_ int, reg *handlerRegistration[ResourceWatcher]) {
		key := c.keyForWatcherEvent(kind, reg.id, obj)
		owner, ownsFinalizers := reg.handler.(finalizerOwner)
		cleanup := order.terminating && ownsFinalizers && owner.needsCleanup(obj)
		deliver(cleanupOrder{watcherCleanup, reg.id}, reg.lifetime, key, action, cleanup, ownsFinalizers || orderActive,
			func(delivery context.Context, previous, target resource.Object, timeout time.Duration) error {
				return c.dispatchWatcher(delivery, run, reg, key, action, previous, target, timeout)
			})
	})
	reconcileAction := action
	if action == ResourceActionUpdate && old != nil && old.GetResourceVersion() == obj.GetResourceVersion() {
		reconcileAction = ResourceActionResync
	}
	c.reconcilers.Range(kind, func(_ int, reg *handlerRegistration[Reconciler]) {
		key := c.keyForReconcilerEvent(kind, reg.id, obj)
		owner, ok := reg.handler.(finalizerOwner)
		cleanup := order.terminating && ok && owner.needsCleanup(obj)
		deliver(cleanupOrder{reconcilerCleanup, reg.id}, reg.lifetime, key, reconcileAction, cleanup, orderActive,
			func(delivery context.Context, _, target resource.Object, timeout time.Duration) error {
				live := run.workContext(reg.lifetime)
				req := ReconcileRequest{Action: ReconcileActionFromResourceAction(reconcileAction), Object: target}
				return c.doReconcile(delivery, live, reg.handler, req, key, timeout)
			})
	})
	return interrupted
}

// A retained update can queue a retry just before it finishes. If the event
// expires waiting for it, inspect the retries on resumption. A deletion must
// also invalidate stale work immediately, even if a redelivery is coalesced.
func (c *InformerController) deliveryRetryWait(ctx context.Context, key string, obj resource.Object, action ResourceAction, prior <-chan struct{}, interrupted error) ([]<-chan struct{}, bool, error) {
	if interrupted == nil {
		interrupted = ctx.Err()
	}
	if prior != nil && interrupted == nil {
		interrupted = waitForRetries(ctx, []<-chan struct{}{prior})
	}
	if prior == nil || interrupted == nil {
		return c.dequeueIfRequired(key, obj, action), false, interrupted
	}
	if action == ResourceActionDelete || obj.GetDeletionTimestamp() != nil {
		return append(c.dequeueIfRequired(key, obj, action), prior), true, interrupted
	}
	return []<-chan struct{}{prior}, true, interrupted
}

type finalizerOwner interface {
	needsCleanup(resource.Object) bool
}

// Keep the public handler as the entry point, including for wrappers that
// embed an SDK watcher or reconciler and override its methods.
type refreshCleanupKey struct{}

func (c *InformerController) needsOrderedCleanup(handler any, obj resource.Object) bool {
	owner, ok := handler.(finalizerOwner)
	return c.retryProcessor != nil && ok && owner.needsCleanup(obj)
}

func (c *InformerController) cleanupContext(ctx context.Context, handler any, obj resource.Object) context.Context {
	if c.needsOrderedCleanup(handler, obj) {
		return context.WithValue(ctx, refreshCleanupKey{}, true)
	}
	return ctx
}

func (c *InformerController) dispatchWatcher(delivery context.Context, run *informerRun, reg *handlerRegistration[ResourceWatcher], key string, action ResourceAction, old, obj resource.Object, timeout time.Duration) error {
	ctx, cancel := informerEventContext(delivery, timeout)
	defer cancel()
	watcher := reg.handler
	ctx = c.cleanupContext(ctx, watcher, obj)
	call := func(ctx context.Context, old, obj resource.Object) error {
		switch action {
		case ResourceActionCreate:
			return watcher.Add(ctx, obj)
		case ResourceActionDelete:
			return watcher.Delete(ctx, obj)
		}
		return watcher.Update(ctx, old, obj)
	}
	kind := obj.GetStaticMetadata().Kind
	var err error
	var syncRetry *watcherSyncRetry
	c.wrapWatcherCall(string(action), kind, func() {
		// A startup Add and a cache Update can both call Sync. Record the
		// actual failing callback without changing the action or error seen by policies.
		if _, ok := watcher.(finalizerOwner); ok && obj.GetGeneration() == 0 && obj.GetDeletionTimestamp() == nil && action != ResourceActionDelete {
			var failed atomic.Pointer[OpinionatedWatcher]
			err = call(context.WithValue(ctx, syncFailureKey{}, &failed), old, obj)
			if owner := failed.Load(); err != nil && owner != nil {
				syncRetry = &watcherSyncRetry{target: obj.Copy(), finalizer: owner.finalizer}
			}
			return
		}
		err = call(ctx, old, obj)
	})
	if err == nil {
		return nil
	}
	// An expired delivery needs a handoff. If only its deferred attempt
	// expired, the registration is still live and the retry policy applies.
	if interrupted, ok := errors.AsType[interruptedDelivery](err); ok && delivery.Err() != nil {
		return interrupted.Unwrap()
	}
	c.watcherErrors.WithLabelValues(string(action), kind).Inc()
	if c.ErrorHandler != nil {
		c.ErrorHandler(ctx, err)
	}
	// The informer can release its event context immediately after delivery.
	// A finalizer retry still belongs to the active informer and watcher.
	var live context.Context
	if run != nil {
		live = run.workContext(reg.lifetime)
		if live == nil {
			// The handler or informer stopped during the callback.
			return nil
		}
	}
	c.queueRetry(key, err, retryInformerEvent(ctx, live, func(retryCtx context.Context) (*time.Duration, error) {
		if reg.lifetime.Err() != nil {
			return nil, nil
		}
		if syncRetry != nil {
			target := syncRetry.snapshot()
			// Equal versions deliver a cache Update as Sync, even after retargeting.
			return nil, call(retryCtx, target, target)
		}
		return nil, call(c.cleanupContext(retryCtx, watcher, obj), old, obj)
	}), action, obj, syncRetry)
	return nil
}

// A legacy retry can finish after the event that asked to dequeue it.
type retryDequeueEvent struct {
	action ResourceAction
	object resource.Object
	policy RetryDequeuePolicy
}

func (e retryDequeueEvent) decide(req queuedRetry) (discard, wait bool) {
	if e.policy == nil || e.policy(e.action, e.object, req.Action, req.Object, req.LastError) {
		return true, true
	}
	if req.syncRetry != nil {
		// A retained Sync must use the latest eligible snapshot. A callback
		// already working on the previous snapshot must finish before delivery.
		return !req.syncRetry.observe(e.action, e.object), true
	}
	return false, false
}

func (c *InformerController) dequeueIfRequired(retryKey string, currentObjectState resource.Object, action ResourceAction) []<-chan struct{} {
	event := retryDequeueEvent{action: action, object: currentObjectState, policy: c.RetryDequeuePolicy}
	if c.retryProcessor != nil {
		return c.retryProcessor.dequeue(retryKey, event.decide)
	}
	if queue, ok := c.toRetry.Load(retryKey); ok {
		return queue.dequeue(event)
	}
	return nil
}

func (c *InformerController) doReconcile(delivery, live context.Context, reconciler Reconciler, req ReconcileRequest, retryKey string, timeout time.Duration) error {
	ctx, cancel := informerEventContext(delivery, timeout)
	defer cancel()
	// Metrics for the reconcile action
	action := ResourceActionFromReconcileAction(req.Action)
	if c.inflightActions != nil {
		c.inflightActions.WithLabelValues(string(action), req.Object.GetStaticMetadata().Kind).Inc()
		defer c.inflightActions.WithLabelValues(string(action), req.Object.GetStaticMetadata().Kind).Dec()
	}
	if c.reconcilerLatency != nil {
		start := time.Now()
		defer func() {
			c.reconcilerLatency.WithLabelValues(string(action), req.Object.GetStaticMetadata().Kind).Observe(time.Since(start).Seconds())
		}()
	}

	ctx, span := GetTracer().Start(ctx, "controller-event-reconcile")
	defer span.End()
	// Do the reconcile
	ctx = c.cleanupContext(ctx, reconciler, req.Object)
	res, err := reconciler.Reconcile(ctx, req)
	if interrupted, ok := errors.AsType[interruptedDelivery](err); ok && delivery.Err() != nil {
		return interrupted.Unwrap()
	}
	// If the response contains a state, add it to the request for future retries
	if res.State != nil {
		req.State = res.State
	}
	if err == nil && res.RequeueAfter == nil {
		return nil
	}
	retry := retryInformerEvent(ctx, live, func(retryCtx context.Context) (*time.Duration, error) {
		result, err := reconciler.Reconcile(c.cleanupContext(retryCtx, reconciler, req.Object), req)
		if result.State != nil {
			req.State = result.State
		}
		return result.RequeueAfter, err
	})
	if res.RequeueAfter != nil {
		// If RequeueAfter is non-nil, add a retry to the queue for now+RequeueAfter
		c.enqueueRetry(RetryRequest{
			Key:        retryKey,
			RetryAfter: time.Now().Add(*res.RequeueAfter),
			RetryFunc:  retry,
			Action:     action,
			Object:     req.Object,
			LastError:  err,
		}, nil)
	} else if err != nil {
		// Otherwise, if err is non-nil, queue a retry according to the RetryPolicy
		c.reconcilerErrors.WithLabelValues(string(action), req.Object.GetStaticMetadata().Kind).Inc()
		if c.ErrorHandler != nil {
			// Call the ErrorHandler function as well if it's set
			c.ErrorHandler(ctx, err)
		}
		c.queueRetry(retryKey, err, retry, ResourceActionFromReconcileAction(req.Action), req.Object, nil)
	}
	return nil
}

func (c *InformerController) startEvent(eventType string, resourceKind string) time.Time {
	if c.totalEvents != nil {
		c.totalEvents.WithLabelValues(eventType, resourceKind).Inc()
	}
	if c.inflightEvents != nil {
		c.inflightEvents.WithLabelValues(eventType, resourceKind).Inc()
	}
	return time.Now()
}

func (c *InformerController) completeEvent(eventType string, resourceKind string, startTime time.Time) {
	if c.inflightEvents != nil {
		c.inflightEvents.WithLabelValues(eventType, resourceKind).Dec()
	}
	if c.reconcileLatency != nil {
		c.reconcileLatency.WithLabelValues(eventType, resourceKind).Observe(time.Since(startTime).Seconds())
	}
}

func (c *InformerController) wrapWatcherCall(eventType string, resourceKind string, f func()) {
	if c.inflightActions != nil {
		c.inflightActions.WithLabelValues(eventType, resourceKind).Inc()
		defer c.inflightActions.WithLabelValues(eventType, resourceKind).Dec()
	}
	start := time.Now()
	f()
	if c.watcherLatency != nil {
		c.watcherLatency.WithLabelValues(eventType, resourceKind).Observe(time.Since(start).Seconds())
	}
}

func (*InformerController) keyForWatcherEvent(resourceKind string, watcherIndex int, obj resource.Object) string {
	if obj == nil {
		return fmt.Sprintf("%s:%d:nil:nil", resourceKind, watcherIndex)
	}
	return fmt.Sprintf("%s:%d:%s:%s", resourceKind, watcherIndex, obj.GetNamespace(), obj.GetName())
}

func (*InformerController) keyForReconcilerEvent(resourceKind string, reconcilerIndex int, obj resource.Object) string {
	if obj == nil {
		return fmt.Sprintf("reconcile:%s:%d:nil:nil", resourceKind, reconcilerIndex)
	}
	return fmt.Sprintf("reconcile:%s:%d:%s:%s", resourceKind, reconcilerIndex, obj.GetNamespace(), obj.GetName())
}

func (c *InformerController) queueRetry(key string, err error, toRetry func() (*time.Duration, error), action ResourceAction, obj resource.Object, syncRetry *watcherSyncRetry) {
	if c.RetryPolicy == nil {
		return
	}

	if ok, after := c.RetryPolicy(err, 0); ok {
		c.enqueueRetry(RetryRequest{
			Key:        key,
			RetryAfter: time.Now().Add(after),
			RetryFunc:  toRetry,
			Action:     action,
			Object:     obj,
			LastError:  err,
		}, syncRetry)
	}
}
