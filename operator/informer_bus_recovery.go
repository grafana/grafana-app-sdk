package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusRecoveryOptions configures LIST recovery for an existing bus subscription.
type BusRecoveryOptions struct {
	ResyncInterval       time.Duration
	AllowDegradedStart   bool
	MaxConcurrentWorkers uint64
}

// BusRecoveryDefaults supplies deployment defaults for LIST recovery. The
// kind's WatchNeeds can set the resync interval and page size.
type BusRecoveryDefaults struct {
	// Detail controls what handlers receive. Zero means WatchDetailIdentity.
	Detail             WatchDetail
	ResyncInterval     time.Duration
	PageSize           int64
	AllowDegradedStart bool
	KeyListerFactory   KeyListerFactory
}

// NewBusRecovery offers live events and scheduled LIST recovery.
// Full-object delivery uses the same kind-specific client for GET and LIST.
// Use resource.NamespaceAll in ListWatchOptions for a cluster-wide subscription
// and LIST.
// A nil subscriber disables this option in the selector.
func NewBusRecovery(subscriber resourcebus.Subscriber, group resourcebus.Group, defaults BusRecoveryDefaults) InformerMechanism {
	if subscriber == nil {
		return nil
	}
	fullObjects := defaults.Detail == WatchDetailFullObject
	return newBusMechanism(recoveryWatchOptions(group, defaults.Detail), func(
		kind resource.Kind, clients resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		switch defaults.Detail {
		case "", WatchDetailIdentity, WatchDetailFullObject:
		default:
			return nil, fmt.Errorf("unsupported bus recovery detail %q", defaults.Detail)
		}
		bus, err := newBusInformer(kind, subscriber, busOptionsFor(group, opts))
		if err != nil {
			return nil, err
		}
		client, err := clientForList(kind, clients)
		if err != nil {
			return nil, err
		}
		listOpts := listOptionsFor(opts, defaults.PageSize)
		lister, err := NewClientLister(client, kind, listOpts)
		if err != nil {
			return nil, err
		}
		var getObject func(context.Context, resource.Identifier) (resource.Object, error)
		if fullObjects {
			getObject = client.Get
		} else {
			lister, err = keysListerFor(kind, defaults.KeyListerFactory, lister, listOpts)
			if err != nil {
				return nil, err
			}
		}
		workers := opts.MaxConcurrentWorkers
		if workers == 0 {
			workers = defaultConcurrentInformerWorkers
		}
		return wrapBusWithRecovery(bus, lister, BusRecoveryOptions{
			ResyncInterval:       resyncIntervalOr(opts, defaults.ResyncInterval),
			AllowDegradedStart:   defaults.AllowDegradedStart,
			MaxConcurrentWorkers: workers,
		}, getObject), nil
	})
}

func recoveryWatchOptions(group resourcebus.Group, detail WatchDetail) WatchOptions {
	provides := busWatchOptions(group)
	if detail == WatchDetailFullObject {
		provides.Live.Detail = WatchDetailFullObject
	}
	provides.Resync = periodicWatchOptions(provides.Live.Detail).Resync
	return provides
}

// NewBusInformerWithRecovery delivers resource identities from notifications
// and a caller-supplied recovery lister. The lister must cover the
// subscription's namespace. Items outside that namespace are ignored.
// The LIST and notifications must use the same increasing numeric resource
// versions. An empty initial LIST can have
// snapshot version 0. Use NewBusRecovery for full-object delivery.
func NewBusInformerWithRecovery(kind resource.Kind, subscriber resourcebus.Subscriber, lister ResourceLister, busOpts BusInformerOptions, recoveryOpts BusRecoveryOptions) (*BusInformerWithRecovery, error) {
	if lister == nil {
		return nil, errors.New("bus recovery requires a lister")
	}
	bus, err := newBusInformer(kind, subscriber, busOpts)
	if err != nil {
		return nil, err
	}
	return wrapBusWithRecovery(bus, lister, recoveryOpts, nil), nil
}

func wrapBusWithRecovery(
	bus *busInformer, lister ResourceLister, opts BusRecoveryOptions,
	getObject func(context.Context, resource.Identifier) (resource.Object, error),
) *BusInformerWithRecovery {
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = defaultPeriodicInterval
	}
	return &BusInformerWithRecovery{
		busInformer: bus, lister: lister, opts: opts, getObject: getObject,
		relistRequests: make(chan struct{}, 1), overflows: make(chan struct{}, 1),
		items: make(map[resource.Identifier]resource.Object), deleted: make(map[resource.Identifier]int64),
	}
}

// BusInformerWithRecovery combines bus notifications with LIST recovery.
type BusInformerWithRecovery struct {
	*busInformer
	lister    ResourceLister
	opts      BusRecoveryOptions
	getObject func(context.Context, resource.Identifier) (resource.Object, error)

	relistRequests chan struct{}
	overflows      chan struct{}
	deliveries     []chan busDelivery

	// The run loop alone updates the snapshot and queues changes for delivery.
	items       map[resource.Identifier]resource.Object
	deleted     map[resource.Identifier]int64
	listVersion int64
}

// WatchOptions describes the live and scheduled deliveries provided by the informer.
func (b *BusInformerWithRecovery) WatchOptions() WatchOptions {
	detail := WatchDetailIdentity
	if b.getObject != nil {
		detail = WatchDetailFullObject
	}
	return recoveryWatchOptions(b.group, detail)
}

// Schema returns the kind served by the informer.
func (b *BusInformerWithRecovery) Schema() resource.Schema { return b.kind.Schema }

// HealthCheck reports whether initial sync has completed and the informer is running.
func (b *BusInformerWithRecovery) HealthCheck(context.Context) error {
	select {
	case <-b.stopped:
		if b.runErr != nil {
			return b.runErr
		}
		return errors.New("bus informer has stopped")
	default:
	}
	select {
	case <-b.synced:
		return nil
	default:
		return errors.New("bus informer has not synced")
	}
}

// Run starts the subscription and keeps the snapshot current until ctx is canceled.
func (b *BusInformerWithRecovery) Run(ctx context.Context) error {
	return b.runOnce(ctx, b.run)
}

func (b *BusInformerWithRecovery) run(parent context.Context) error {
	ctx, cancel := context.WithCancelCause(parent)
	var workers sync.WaitGroup
	b.startDeliveryWorkers(ctx, &workers)
	sub, err := b.startSubscription(ctx, cancel, &workers)
	defer func() {
		cancel(nil)
		if sub != nil {
			b.closeSubscription(ctx, sub)
		}
		workers.Wait()
	}()

	if err == nil {
		err = b.initialList(ctx)
	}
	if err == nil {
		err = b.watch(ctx)
	}
	if parent.Err() != nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return err
}

func (b *BusInformerWithRecovery) startSubscription(ctx context.Context, cancel context.CancelCauseFunc, workers *sync.WaitGroup) (resourcebus.Subscription, error) {
	hooks := busSubscriptionHooks{onReconnect: b.requestRelist, onOverflow: b.requestRateLimitedRelist}
	if !b.opts.AllowDegradedStart {
		return b.subscribe(ctx, hooks)
	}
	sub, err := b.openSubscription(ctx, hooks)
	if err == nil || ctx.Err() != nil || subscriptionRejected(err) {
		return sub, err
	}
	b.reportError(ctx, err)
	workers.Go(func() { b.retrySubscription(ctx, cancel, hooks) })
	return nil, nil
}

func (b *BusInformerWithRecovery) retrySubscription(ctx context.Context, cancel context.CancelCauseFunc, hooks busSubscriptionHooks) {
	if !waitInformerInterval(ctx, busRetryInterval) {
		return
	}
	sub, err := b.subscribe(ctx, hooks)
	if err != nil {
		if ctx.Err() == nil {
			cancel(err)
		}
		return
	}
	b.requestRelist()
	<-ctx.Done()
	b.closeSubscription(ctx, sub)
}

func (b *BusInformerWithRecovery) initialList(ctx context.Context) error {
	listCtx := ContextWithDelivery(ctx, DeliveryRelist)
	for ctx.Err() == nil {
		err := b.relist(listCtx, busRelistScheduled)
		switch {
		case err == nil:
			close(b.synced)
			return nil
		case subscriptionRejected(err):
			return err
		case ctx.Err() != nil:
			return ctx.Err()
		}
		b.reportError(ctx, err)
		if !waitInformerInterval(ctx, busRetryInterval) {
			break
		}
	}
	return ctx.Err()
}

func (b *BusInformerWithRecovery) watch(ctx context.Context) error {
	resyncAt := time.Now().Add(wait.Jitter(b.opts.ResyncInterval, informerRetryJitter))
	var retryAt, nextOverflowRelist time.Time
	for {
		trigger := b.serveUntilRelist(ctx, resyncAt, retryAt, nextOverflowRelist)
		if trigger == busRelistStopped {
			return ctx.Err()
		}
		err := b.relist(ContextWithDelivery(ctx, DeliveryRelist), trigger)
		now := time.Now()
		if trigger == busRelistOverflow {
			nextOverflowRelist = now.Add(overflowRelistInterval)
		}
		delay := b.opts.ResyncInterval
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case subscriptionRejected(err):
			return err
		case err != nil:
			b.reportError(ctx, err)
			delay = busRetryInterval
		default:
			retryAt = time.Time{}
		}
		if trigger == busRelistScheduled {
			resyncAt = now.Add(wait.Jitter(delay, informerRetryJitter))
		} else if err != nil {
			retryAt = now.Add(wait.Jitter(delay, informerRetryJitter))
		}
	}
}

const overflowRelistInterval = 30 * time.Second

type busRelistTrigger uint8

const (
	busRelistStopped busRelistTrigger = iota
	busRelistScheduled
	busRelistCatchUp
	busRelistOverflow
)

func (b *BusInformerWithRecovery) serveUntilRelist(ctx context.Context, resyncAt, retryAt, nextOverflowRelist time.Time) busRelistTrigger {
	resync := time.NewTimer(time.Until(resyncAt))
	defer resync.Stop()
	var retry <-chan time.Time
	if !retryAt.IsZero() {
		timer := time.NewTimer(time.Until(retryAt))
		defer timer.Stop()
		retry = timer.C
	}
	var overflowTimer *time.Timer
	defer func() {
		if overflowTimer != nil {
			overflowTimer.Stop()
		}
	}()
	overflowEvents := (<-chan struct{})(b.overflows)
	var overflowReady <-chan time.Time

	for ctx.Err() == nil {
		// Once due, a full resync takes priority over other requests.
		select {
		case <-resync.C:
			return busRelistScheduled
		default:
		}
		select {
		case <-ctx.Done():
			return busRelistStopped
		case <-resync.C:
			return busRelistScheduled
		case <-retry:
			return busRelistCatchUp
		case <-b.relistRequests:
			return busRelistCatchUp
		case <-overflowReady:
			return busRelistOverflow
		case <-overflowEvents:
			if remaining := time.Until(nextOverflowRelist); remaining > 0 {
				// A LIST already queued by the normal schedule or reconnect
				// can cover this drop before the cooldown ends.
				overflowTimer = time.NewTimer(remaining)
				overflowReady = overflowTimer.C
				overflowEvents = nil
				continue
			}
			return busRelistOverflow
		case event := <-b.events:
			b.applyLive(ctx, event)
		}
	}
	return busRelistStopped
}

func (b *BusInformerWithRecovery) requestRelist() {
	select {
	case b.relistRequests <- struct{}{}:
	default:
	}
}

func (b *BusInformerWithRecovery) requestRateLimitedRelist() {
	select {
	case b.overflows <- struct{}{}:
	default:
	}
}

const defaultHydrationTimeout = 30 * time.Second

func (b *BusInformerWithRecovery) liveObject(ctx context.Context, event resourcebus.Event, id resource.Identifier) (resource.Object, error) {
	if b.getObject == nil {
		return b.kind.NewIdentityObject(id, event.ResourceVersion), nil
	}
	timeout := b.timeout
	if timeout <= 0 {
		timeout = defaultHydrationTimeout
	}
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	obj, err := b.getObject(readCtx, id)
	if err != nil {
		return nil, err
	}
	if obj == nil || obj.GetStaticMetadata().Identifier() != id {
		return nil, errors.New("object response does not match the notification identity")
	}
	observed, announced := numericVersion(obj.GetResourceVersion()), numericVersion(event.ResourceVersion)
	if observed == 0 {
		return nil, fmt.Errorf("object response has an invalid resource version %q", obj.GetResourceVersion())
	}
	if observed < announced {
		return nil, fmt.Errorf("object version %d predates announced version %d", observed, announced)
	}
	return b.snapshotObject(obj), nil
}

func (b *BusInformerWithRecovery) snapshotObject(obj resource.Object) resource.Object {
	if b.getObject != nil {
		owned := obj.Copy()
		gvk := obj.GroupVersionKind()
		if filled := withFallbackGVK(gvk, b.kind.GroupVersionKind()); filled != gvk {
			owned.SetGroupVersionKind(filled)
		}
		return owned
	}
	return b.kind.NewIdentityObject(obj.GetStaticMetadata().Identifier(), obj.GetResourceVersion())
}

func withFallbackGVK(gvk, fallback schema.GroupVersionKind) schema.GroupVersionKind {
	if gvk.Group == "" {
		gvk.Group = fallback.Group
	}
	if gvk.Version == "" {
		gvk.Version = fallback.Version
	}
	if gvk.Kind == "" {
		gvk.Kind = fallback.Kind
	}
	return gvk
}

type busDelivery struct {
	ctx     context.Context
	old     resource.Object
	current resource.Object
}

func (b *BusInformerWithRecovery) startDeliveryWorkers(ctx context.Context, workers *sync.WaitGroup) {
	count := b.opts.MaxConcurrentWorkers
	if count <= 1 {
		return
	}
	b.deliveries = make([]chan busDelivery, count)
	for i := range b.deliveries {
		queue := make(chan busDelivery, 1)
		b.deliveries[i] = queue
		workers.Go(func() {
			for ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return
				case d := <-queue:
					b.dispatchChange(d)
				}
			}
		})
	}
}

func (b *BusInformerWithRecovery) deliverChange(ctx context.Context, old, current resource.Object) {
	if busObjectReplaced(old, current) {
		b.queueDelivery(busDelivery{ctx: ctx, old: old})
		old = nil
	}
	b.queueDelivery(busDelivery{ctx: ctx, old: old, current: current})
}

func busObjectReplaced(old, current resource.Object) bool {
	return old != nil && current != nil && old.GetUID() != "" && current.GetUID() != "" && old.GetUID() != current.GetUID()
}

func (b *BusInformerWithRecovery) queueDelivery(d busDelivery) {
	ctx, old, current := d.ctx, d.old, d.current
	if len(b.deliveries) == 0 {
		b.dispatchChange(d)
		return
	}
	obj := current
	if obj == nil {
		obj = old
	}
	key := obj.GetNamespace() + "/" + obj.GetName()
	queue := b.deliveries[xxhash.Sum64String(key)%uint64(len(b.deliveries))]
	select {
	case <-ctx.Done():
	case queue <- d:
	}
}

func (b *BusInformerWithRecovery) dispatchChange(d busDelivery) {
	for _, handler := range b.handlers {
		if d.ctx.Err() != nil {
			return
		}
		err := withInformerEventTimeout(d.ctx, b.timeout, handler, func(ctx context.Context) error {
			// The snapshot and queued deliveries own their objects. A watcher may
			// mutate the copies it receives, including finalizer updates.
			switch {
			case d.current == nil:
				return handler.Delete(ctx, d.old.Copy())
			case d.old == nil:
				return handler.Add(ctx, d.current.Copy())
			default:
				return handler.Update(ctx, d.old.Copy(), d.current.Copy())
			}
		})
		if err != nil {
			b.reportError(d.ctx, err)
		}
	}
}
