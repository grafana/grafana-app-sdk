package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusPayload selects which object a BusInformer hands its watchers for a live event.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusPayload string

const (
	// PayloadIdentity delivers the kind's zero value with namespace, name and resource version set. It suits a
	// reconciler which treats an event as a signal and reads the object itself, and costs no API request per event.
	// Nothing else on the object is populated, so a watcher which reads the spec, the deletion timestamp or the
	// finalizer list must not use it; see the opinionated-watcher constraint in the docs.
	PayloadIdentity BusPayload = "identity"
	// PayloadHydrated reads the object from the API before dispatching, so watchers receive a full object as they
	// would from a watch-backed informer, at the cost of one request per event. The ResourceLister must return full
	// objects in this mode: a lister which returns identities alone would make re-list deliveries carry less than
	// live ones.
	PayloadHydrated BusPayload = "hydrated"
)

const (
	// DefaultBusResyncInterval is the re-list cadence a BusInformer uses when none is configured.
	DefaultBusResyncInterval = 5 * time.Minute
	// DefaultBusResyncJitterFactor spreads the re-list cadence by up to this fraction, so that informers which
	// started together do not re-list in lockstep and stampede the API server.
	DefaultBusResyncJitterFactor = 0.1
	// DefaultBusRetryInterval is how often a BusInformer retries a failing subscription or initial list.
	DefaultBusRetryInterval = 5 * time.Second
	// DefaultBusEventQueueSize is how many live events a BusInformer buffers when none is configured.
	DefaultBusEventQueueSize = 1000
	// DefaultBusEventDrainBatch is how many queued events a BusInformer applies per pass when none is configured.
	DefaultBusEventDrainBatch = 64
	// DefaultBusOverflowRelistInterval is the shortest gap between dropped-event re-lists when none is configured.
	DefaultBusOverflowRelistInterval = 30 * time.Second
)

// relistPass distinguishes the initial list from a later one. They differ in one way: an add on a later pass is a
// change the live stream did not deliver here, so its resource version dates the recovery, while the initial list's
// objects may be arbitrarily old.
type relistPass int

const (
	initialRelist relistPass = iota
	periodicRelist
)

// BusObjectGetter reads a single object, for the hydrated payload mode. resource.Client satisfies it.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusObjectGetter interface {
	Get(ctx context.Context, identifier resource.Identifier) (resource.Object, error)
}

// BusInformerOptions are the options for a BusInformer.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerOptions struct {
	// Namespace scopes the subscription and must match the namespace the ResourceLister lists.
	// An empty namespace covers every namespace.
	Namespace string
	// ListWatchOptions are the namespace and filters the informer was configured with. The bus contract carries a
	// namespace and nothing else, so a label or field selector here is rejected by NewBusInformer rather than
	// applied to the list alone: live events would then arrive for objects the selector excludes, be written into
	// the snapshot, and be diffed as deletes by the next filtered re-list. See NewBusInformer.
	ListWatchOptions ListWatchOptions
	// Payload selects which object live events deliver. An empty value is PayloadIdentity.
	Payload BusPayload
	// Getter reads objects for PayloadHydrated. It is required in that mode and ignored otherwise.
	Getter BusObjectGetter
	// ResyncInterval is how often the full set is re-listed. A non-positive value uses DefaultBusResyncInterval.
	ResyncInterval time.Duration
	// Group joins this app's replicas into a group across which the bus shares delivery, so that each event reaches
	// one replica rather than all of them. A zero Group delivers every event to every replica. Either way the
	// periodic re-list is what keeps a replica reconciled, so a group trades live volume, not correctness.
	Group resourcebus.Group
	// AllowDegradedStart lets Run fall back to re-list-only operation when the subscription cannot be opened,
	// instead of holding the initial list (and so WaitForSync) until it succeeds. The subscription keeps retrying in
	// the background and forces a re-list once it opens. Consumers whose progress must not stall with the bus opt
	// in; the default suits a consumer which prefers not to start against a snapshot with no live updates.
	AllowDegradedStart bool
	// RetryInterval is how often a failing subscription or initial list is retried.
	// A non-positive value uses DefaultBusRetryInterval.
	RetryInterval time.Duration
	// ErrorHandler is called for errors which do not stop the informer, such as a watcher returning an error or an
	// event dropped because its object could not be read.
	ErrorHandler func(context.Context, error)
	// EventQueueSize is how many live events may be waiting to be applied. The subscriber callback must not block,
	// so events are queued rather than handled on the delivery path; a full queue drops the event and forces a
	// re-list to recover it. A non-positive value uses DefaultBusEventQueueSize.
	EventQueueSize int
	// EventDrainBatch is how many queued events are applied before the informer offers the re-list cases again.
	// It is what stops a recovery re-list from competing one-for-one with the queue it is meant to relieve, while
	// keeping the re-list from being starved by a steady stream. A non-positive value uses
	// DefaultBusEventDrainBatch.
	EventDrainBatch int
	// OverflowRelistInterval is the shortest gap between re-lists triggered by a dropped event. A non-positive
	// value uses DefaultBusOverflowRelistInterval. See EventQueueSize.
	OverflowRelistInterval time.Duration
	// Metrics are the shared delivery metrics. A nil value disables observation.
	Metrics *BusInformerMetrics
}

// BusInformer drives watchers from a resourcebus.Subscriber instead of a kubernetes watch. It keeps no per-object
// cache that a reader can serve from: on each live event it dispatches either the event's identity or an object it
// reads for that event, and it periodically re-lists the full set.
//
// The periodic re-list is what makes the informer correct rather than merely live. An event routed to another
// replica of a shared group, published while the subscription was down, or never announced at all is caught by the
// next re-list, which diffs the listed set against the previous snapshot and dispatches the difference: a key seen
// for the first time as an add, a key seen before as an update, and a key which has vanished as a delete carrying
// its last-known state.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformer struct {
	kind       resource.Kind
	gvr        schema.GroupVersionResource
	subscriber resourcebus.Subscriber
	lister     ResourceLister
	opts       BusInformerOptions
	metrics    kindMetrics

	store *busStore

	// events carries live events from the subscriber callback to the run loop, which applies and dispatches them.
	// See onEvent.
	events chan resourcebus.Event

	// relist signals the run loop to re-list out of band, after a reconnect or a dropped event. Buffered depth 1
	// with a non-blocking send coalesces bursts into one re-list.
	relistSignal chan struct{}

	mu       sync.Mutex
	handlers []ResourceWatcher
	synced   atomic.Bool
	syncedCh chan struct{}
	stopped  atomic.Bool

	// lastListRV is the resource version of the most recently installed snapshot, so a queued event older than it
	// can be recognized as superseded. Zero means no list has been dated, which disables the check.
	lastListRV atomic.Int64

	// lastOverflowRelist is when a dropped event last scheduled a re-list, in unix nanoseconds, so a backlog does
	// not schedule one per drop. See signalOverflowRelist.
	lastOverflowRelist atomic.Int64
}

var _ Informer = (*BusInformer)(nil)

// NewBusInformer creates a BusInformer for one kind. subscriber announces live changes; lister reads the full set
// for the initial list and each re-list, and must list the same namespace and filters the subscription covers.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewBusInformer(kind resource.Kind, subscriber resourcebus.Subscriber, lister ResourceLister, opts BusInformerOptions) (*BusInformer, error) {
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if subscriber == nil {
		return nil, errors.New("subscriber cannot be nil")
	}
	if lister == nil {
		return nil, errors.New("lister cannot be nil")
	}
	if opts.Payload == "" {
		opts.Payload = PayloadIdentity
	}
	if opts.Payload != PayloadIdentity && opts.Payload != PayloadHydrated {
		return nil, fmt.Errorf("unknown payload '%s'", opts.Payload)
	}
	if opts.Payload == PayloadHydrated && opts.Getter == nil {
		return nil, errors.New("getter is required for the hydrated payload")
	}
	// The subscription is scoped by namespace alone, so a selector could only ever be applied to the list. The live
	// stream would keep delivering excluded objects, each written into the snapshot and then diffed as a delete by
	// the next re-list — a stream of phantom adds and deletes for objects the app never asked to watch.
	if len(opts.ListWatchOptions.LabelFilters) > 0 || len(opts.ListWatchOptions.FieldSelectors) > 0 {
		return nil, errors.New("label and field selectors are not supported by the bus informer: the bus delivers events by namespace only")
	}
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = DefaultBusResyncInterval
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = DefaultBusRetryInterval
	}
	if opts.EventQueueSize <= 0 {
		opts.EventQueueSize = DefaultBusEventQueueSize
	}
	if opts.EventDrainBatch <= 0 {
		opts.EventDrainBatch = DefaultBusEventDrainBatch
	}
	if opts.OverflowRelistInterval <= 0 {
		opts.OverflowRelistInterval = DefaultBusOverflowRelistInterval
	}
	return &BusInformer{
		kind:         kind,
		gvr:          kind.GroupVersionResource(),
		subscriber:   subscriber,
		lister:       lister,
		opts:         opts,
		metrics:      newKindMetrics(opts.Metrics, kind),
		store:        newBusStore(),
		events:       make(chan resourcebus.Event, opts.EventQueueSize),
		relistSignal: make(chan struct{}, 1),
		syncedCh:     make(chan struct{}),
	}, nil
}

// logger returns the informer's logger, tagged with the component and kind.
func (b *BusInformer) logger(ctx context.Context) logging.Logger {
	return logging.FromContext(ctx).With("component", "BusInformer", "kind", b.kind.GroupVersionKind().String())
}

// IdentityPayload reports whether the informer delivers identity-only objects.
func (b *BusInformer) IdentityPayload() bool { return b.opts.Payload == PayloadIdentity }

// AddEventHandler adds a ResourceWatcher as an event handler. There is no cache to replay, so a watcher added after
// Run only sees events from the next live event or re-list; add every watcher before Run.
func (b *BusInformer) AddEventHandler(handler ResourceWatcher) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, handler)
	return nil
}

// WaitForSync waits for the initial list to complete, or for the context to be done.
func (b *BusInformer) WaitForSync(ctx context.Context) error {
	select {
	case <-b.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HasSynced reports whether the initial list has completed.
func (b *BusInformer) HasSynced() bool { return b.synced.Load() }

// HealthCheck reports an error until the initial list has completed, and once the informer has stopped.
func (b *BusInformer) HealthCheck(context.Context) error {
	if b.stopped.Load() {
		return errors.New("informer is stopped")
	}
	if !b.synced.Load() {
		return errors.New("informer has not synced")
	}
	return nil
}

// HealthCheckName returns the name of the informer's health check.
func (b *BusInformer) HealthCheckName() string {
	return "bus-informer-" + b.kind.GroupVersionKind().String()
}

// Schema returns the resource.Schema the informer is set up for.
func (b *BusInformer) Schema() resource.Schema { return b.kind.Schema }

// Snapshot returns the identities the informer last reconciled with storage: every object its most recent re-list
// listed, plus the live adds delivered since, minus the live deletes.
//
// It is not a cache for serving reads. It lags storage by up to one re-list interval, it holds only what this
// informer was delivered — under a shared delivery group that is a subset of the live stream until the next re-list
// — and it is the informer's own diffing state rather than a view of the API. It suits a staleness-tolerant count,
// such as a quota check which would otherwise cost an API list per reconcile, and nothing a reconcile's correctness
// depends on. Read the object from the API for that.
//
// Identities, rather than objects, are deliberate: the informer keeps no per-object cache worth reading, and in
// identity payload mode the stored objects carry nothing else anyway.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func (b *BusInformer) Snapshot() []resource.Identifier { return b.store.Identifiers() }

// PrometheusCollectors implements metrics.Provider. The collectors are shared by every informer given the same
// BusInformerMetrics, so registering an already-registered collector is expected and must be tolerated.
func (b *BusInformer) PrometheusCollectors() []prometheus.Collector {
	return b.opts.Metrics.PrometheusCollectors()
}

// Run opens the live subscription, performs the initial list, then serves live events and a periodic re-list until
// ctx is done. It blocks.
//
// The subscription is opened before the initial list, so that a change published while the list runs is delivered
// rather than dropped in a list-to-subscribe gap. If it cannot be opened the default is to keep retrying and not
// list: re-listing a kind we cannot watch would start the consumer against a snapshot with no live updates until the
// next re-list. BusInformerOptions.AllowDegradedStart opts into exactly that instead.
func (b *BusInformer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	b.stopped.Store(false)
	defer b.stopped.Store(true)

	b.metrics.observeSubscriptionClosed()
	defer b.metrics.observeSubscriptionClosed()

	sub := b.subscribe(ctx)
	if sub != nil {
		defer func() {
			if err := sub.Close(); err != nil {
				b.logger(ctx).Debug("failed to close subscription", "error", err)
			}
		}()
	}
	if ctx.Err() != nil {
		return nil
	}

	// Retry the initial list until it succeeds: WaitForSync releases the consumer, so syncing after a failed list
	// would start it against an empty snapshot, leaving every existing object unreconciled until the next re-list.
	for {
		if err := b.relist(ctx, initialRelist); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(b.opts.RetryInterval):
		}
	}
	b.synced.Store(true)
	close(b.syncedCh)

	// A fresh timer per pass, rather than a ticker, lets the jitter vary every time. Re-arming before the re-list
	// keeps the cadence start-to-start rather than re-list duration plus interval.
	resync := time.NewTimer(wait.Jitter(b.opts.ResyncInterval, DefaultBusResyncJitterFactor))
	defer resync.Stop()
	for {
		// Drain a bounded run of queued events before offering the re-list cases. A re-list drains nothing and
		// costs a full list, so letting it compete one-for-one with the queue lets a backlog sustain itself:
		// select picks at random among ready cases, so roughly every other turn would be a list. Falling through
		// to the select rather than looping is what keeps the re-list from being starved the other way — it is
		// the correctness mechanism, so a steady stream must not stop it running.
		b.drainEvents(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-resync.C:
			resync.Reset(wait.Jitter(b.opts.ResyncInterval, DefaultBusResyncJitterFactor))
			_ = b.relist(ctx, periodicRelist)
		case <-b.relistSignal:
			b.logger(ctx).Debug("re-listing out of band")
			_ = b.relist(ctx, periodicRelist)
		case evt := <-b.events:
			b.handleEvent(ctx, evt)
		}
	}
}

// drainEvents applies up to EventDrainBatch queued events, stopping early when the queue empties or the context
// ends, so an idle informer falls straight through to the select.
func (b *BusInformer) drainEvents(ctx context.Context) {
	for range b.opts.EventDrainBatch {
		// Checked before the select rather than as a case in it: with both a done context and a queued event
		// ready, a select would pick between them at random and could keep applying events while shutting down.
		if ctx.Err() != nil {
			return
		}
		select {
		case evt := <-b.events:
			b.handleEvent(ctx, evt)
		default:
			return
		}
	}
}

// subscribe opens the live subscription, retrying while it fails. It returns a nil Subscription when the informer is
// starting degraded, in which case a background goroutine keeps retrying and owns the subscription it opens.
func (b *BusInformer) subscribe(ctx context.Context) resourcebus.Subscription {
	opts := []resourcebus.SubscribeOption{resourcebus.WithOnReconnect(b.signalReconnect)}
	if b.opts.Group.Name != "" {
		opts = append(opts, resourcebus.WithGroup(b.opts.Group))
	}

	sub, err := b.subscriber.Subscribe(ctx, b.gvr, b.opts.Namespace, b.onEvent, opts...)
	if err == nil {
		b.metrics.observeSubscriptionOpen()
		return sub
	}
	if b.opts.AllowDegradedStart {
		b.logger(ctx).Warn("failed to subscribe, starting in re-list-only degraded mode", "error", err)
		go b.retrySubscribe(ctx, opts)
		return nil
	}
	for {
		b.logger(ctx).Warn("failed to subscribe, will retry", "error", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(b.opts.RetryInterval):
		}
		sub, err = b.subscriber.Subscribe(ctx, b.gvr, b.opts.Namespace, b.onEvent, opts...)
		if err == nil {
			b.metrics.observeSubscriptionOpen()
			return sub
		}
	}
}

// retrySubscribe keeps trying to open the subscription while the informer runs degraded. Once it opens, a reconnect
// signal forces a re-list to cover whatever was published in between, and the goroutine holds the subscription until
// shutdown so its lifecycle needs no shared state with Run.
func (b *BusInformer) retrySubscribe(ctx context.Context, opts []resourcebus.SubscribeOption) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.opts.RetryInterval):
		}
		sub, err := b.subscriber.Subscribe(ctx, b.gvr, b.opts.Namespace, b.onEvent, opts...)
		if err != nil {
			b.logger(ctx).Warn("failed to subscribe, will retry", "error", err)
			continue
		}
		b.metrics.observeSubscriptionOpen()
		b.logger(ctx).Info("subscription opened, leaving re-list-only degraded mode")
		b.signalReconnect()
		<-ctx.Done()
		if err := sub.Close(); err != nil {
			b.logger(ctx).Debug("failed to close subscription", "error", err)
		}
		return
	}
}

// signalReconnect records the reconnect and nudges the run loop to re-list, since events published while the
// subscription was down were never delivered. It is the reconnect callback, so it must not block.
func (b *BusInformer) signalReconnect() {
	b.metrics.observeReconnect()
	b.signalRelist()
}

// signalRelist asks the run loop for an out-of-band re-list. The send is non-blocking and a pending signal
// coalesces further requests into the next re-list, so callers on a delivery path never wait.
func (b *BusInformer) signalRelist() {
	select {
	case b.relistSignal <- struct{}{}:
	default:
	}
}

// onEvent is the subscriber callback. The contract requires it not to block, so it only hands the event to the run
// loop: hydration reads the API and watchers run arbitrary code, neither of which may sit on the bus's delivery path.
//
// A full queue means the informer is not keeping up. The event is dropped and a re-list forced, which is the same
// recovery the informer already runs for events published while the subscription was down — dropping is preferable
// to blocking the bus or growing the queue without bound.
func (b *BusInformer) onEvent(_ context.Context, evt resourcebus.Event) {
	select {
	case b.events <- evt:
	default:
		b.metrics.observeDroppedEvent()
		b.signalOverflowRelist()
	}
}

// signalOverflowRelist asks for a recovery re-list after a dropped event, but no more often than
// OverflowRelistInterval. A sustained backlog drops many events, and one list recovers all of them, so scheduling
// one per drop only spends list requests on work already in hand. A drop whose signal is suppressed is still
// recovered — by the next re-list, whichever triggers it — within the resync interval.
func (b *BusInformer) signalOverflowRelist() {
	now := time.Now().UnixNano()
	last := b.lastOverflowRelist.Load()
	if now-last < int64(b.opts.OverflowRelistInterval) {
		return
	}
	// A lost race means another drop just scheduled the re-list this one wanted.
	if !b.lastOverflowRelist.CompareAndSwap(last, now) {
		return
	}
	b.signalRelist()
}

// handleEvent applies one live event and dispatches it. It runs on the run loop, so a live event and a re-list diff
// are applied and delivered in one order: without that, a live delete could evict a key between a re-list computing
// its diff and delivering it, leaving watchers with an add for an object the store no longer holds and no
// corrective delete to follow.
func (b *BusInformer) handleEvent(ctx context.Context, evt resourcebus.Event) {
	ctx = ContextWithDelivery(ctx, DeliveryLive)
	rv := parseResourceVersion(evt.ResourceVersion)
	// An event waits in the queue, and a re-list can complete while it waits. A list read after the event was
	// published already accounts for it — including by listing a re-creation, or by confirming an absence — so
	// applying it now would roll the snapshot back to a state storage has already moved past. The store catches
	// this for a key it still holds; only the completed list's version catches it for a key it does not.
	if listRV := b.lastListRV.Load(); rv > 0 && listRV > 0 && rv < listRV {
		b.metrics.observeSupersededEvent()
		b.logger(ctx).Debug("dropping event superseded by a newer list",
			"namespace", evt.Namespace, "name", evt.Name, "resourceVersion", evt.ResourceVersion)
		return
	}
	switch evt.Verb {
	case resourcebus.VerbDeleted:
		// The object is already gone from storage, so there is nothing to read: dispatch the last-known object the
		// eviction returns, and the identity when the store held none. Evicting keeps the next re-list from diffing
		// a delivered delete as a change the live stream missed.
		obj, applied := b.store.DeleteAt(evt.Namespace, evt.Name, evt.ResourceVersion)
		if !applied {
			b.handleError(ctx, fmt.Errorf("dropping delete for %s/%s: superseded by a newer write", evt.Namespace, evt.Name))
			return
		}
		if obj == nil {
			obj = b.identityObject(evt)
		}
		b.metrics.observeLiveEvent(busVerbDelete, rv)
		b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Delete(ctx, obj) })
	case resourcebus.VerbAdded, resourcebus.VerbModified:
		obj, err := b.payloadObject(ctx, evt)
		if err != nil {
			b.handleError(ctx, err)
			return
		}
		b.handleWrite(ctx, evt, obj, rv)
	default:
		b.handleError(ctx, fmt.Errorf("dropping event with unknown verb '%s' for %s/%s", evt.Verb, evt.Namespace, evt.Name))
	}
}

// handleWrite applies an added or modified event. A modification of an object the informer has not seen is delivered
// as an add: there is no previous state to pair it with, and a first-seen key is what the re-list diff would call an
// add too.
func (b *BusInformer) handleWrite(ctx context.Context, evt resourcebus.Event, obj resource.Object, rv int64) {
	// Reading before writing is safe because every store mutation runs on the run loop: handleEvent and relist are
	// both called from it, so nothing can land between the two calls below.
	previous, held := b.store.Get(evt.Namespace, evt.Name)

	// An identity object carries no deletion timestamp, so storing a modification of a known object would overwrite
	// the stored object's real one and mislead the next re-list's diff. Its version still has to advance, or a later
	// out-of-order delete at a version between the two would read as newer than the key and evict it. Everything
	// else is stored outright: without that the object is absent until the next re-list, which would then re-report
	// a delivered event as a change the live stream missed.
	//
	// Either way the store can refuse — a newer delete may have been applied while the payload was being read — and
	// dispatching regardless would leave watchers holding an event for a state the snapshot does not have, with no
	// later re-list able to correct it.
	if held && evt.Verb == resourcebus.VerbModified && b.opts.Payload == PayloadIdentity {
		if !b.store.Touch(evt.Namespace, evt.Name, evt.ResourceVersion) {
			b.handleError(ctx, fmt.Errorf("dropping event for %s/%s: superseded by a newer write", evt.Namespace, evt.Name))
			return
		}
	} else {
		var taken bool
		if previous, taken = b.store.Update(obj); !taken {
			b.handleError(ctx, fmt.Errorf("dropping event for %s/%s: superseded by a newer write", evt.Namespace, evt.Name))
			return
		}
	}

	if evt.Verb == resourcebus.VerbAdded || previous == nil {
		b.metrics.observeLiveEvent(busVerbAdd, rv)
		b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Add(ctx, obj) })
		return
	}
	b.metrics.observeLiveEvent(busVerbUpdate, rv)
	b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Update(ctx, previous, obj) })
}

// payloadObject builds the object to dispatch for a live event, per the configured payload mode.
func (b *BusInformer) payloadObject(ctx context.Context, evt resourcebus.Event) (resource.Object, error) {
	if b.opts.Payload == PayloadIdentity {
		return b.identityObject(evt), nil
	}
	obj, err := b.opts.Getter.Get(ctx, resource.Identifier{Namespace: evt.Namespace, Name: evt.Name})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("dropping event for %s/%s: object no longer exists", evt.Namespace, evt.Name)
		}
		return nil, fmt.Errorf("dropping event for %s/%s: %w", evt.Namespace, evt.Name, err)
	}
	return obj, nil
}

// identityObject is the kind's zero value carrying the event's identity, which is all a consumer needs to read the
// object for itself.
func (b *BusInformer) identityObject(evt resourcebus.Event) resource.Object {
	return NewIdentityObject(b.kind, evt.Namespace, evt.Name, evt.ResourceVersion)
}

// relist reads the full set and dispatches the diff against the previous snapshot. On the initial list the snapshot
// is empty, so every object is an add and there is nothing to delete.
func (b *BusInformer) relist(ctx context.Context, pass relistPass) error {
	objs, listRV, err := b.lister.List(ctx)
	if err != nil {
		b.handleError(ctx, fmt.Errorf("re-list failed: %w", err))
		return err
	}

	ctx = ContextWithDelivery(ctx, DeliveryRelist)
	added, updated, removed := b.store.Replace(objs, listRV)
	// Recorded after Replace, so a queued event is only judged against a snapshot already installed. An
	// unparseable version leaves it at zero, which disables the check — the same fallback the store's arbitration
	// makes.
	if parsed := parseResourceVersion(listRV); parsed > b.lastListRV.Load() {
		b.lastListRV.Store(parsed)
	}
	b.logger(ctx).Debug("re-listed", "initial", pass == initialRelist, "count", len(objs),
		"added", len(added), "updated", len(updated), "removed", len(removed))

	for _, obj := range added {
		rv := int64(0)
		if pass == periodicRelist {
			rv = parseResourceVersion(obj.GetResourceVersion())
		}
		b.metrics.observeRelistEvent(busVerbAdd, rv)
		b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Add(ctx, obj) })
	}
	for _, upd := range updated {
		b.metrics.observeRelistEvent(busVerbUpdate, 0)
		b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error {
			return w.Update(ctx, upd.previous, upd.current)
		})
	}
	for _, obj := range removed {
		// The last-known version predates the delete, so it cannot date it.
		b.metrics.observeRelistEvent(busVerbDelete, 0)
		b.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Delete(ctx, obj) })
	}
	return nil
}

func (b *BusInformer) dispatch(ctx context.Context, fn func(context.Context, ResourceWatcher) error) {
	b.mu.Lock()
	handlers := b.handlers
	b.mu.Unlock()
	for _, handler := range handlers {
		if err := fn(ctx, handler); err != nil {
			b.handleError(ctx, err)
		}
	}
}

func (b *BusInformer) handleError(ctx context.Context, err error) {
	if b.opts.ErrorHandler != nil {
		b.opts.ErrorHandler(ctx, err)
		return
	}
	logging.FromContext(ctx).Error(err.Error(), "component", "BusInformer",
		"kind", b.kind.GroupVersionKind().String(), "error", err)
}
