package operator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusInformerOptions configures notification delivery with LIST recovery.
type BusInformerOptions struct {
	Namespace            string
	Group                resourcebus.Group
	ResyncInterval       time.Duration
	AllowDegradedStart   bool
	EventTimeout         time.Duration
	MaxConcurrentWorkers uint64
	ErrorHandler         func(context.Context, error)
}

// BusMechanismOptions supplies deployment defaults for a bus with LIST recovery.
type BusMechanismOptions struct {
	Group              resourcebus.Group
	ResyncInterval     time.Duration
	PageSize           int64
	AllowDegradedStart bool
	KeyListerFactory   KeyListerFactory
}

// NewBusMechanism uses an identity snapshot to recover missed notifications.
// A nil subscriber disables the mechanism.
func NewBusMechanism(subscriber resourcebus.Subscriber, defaults BusMechanismOptions) InformerMechanism {
	if subscriber == nil {
		return nil
	}
	return newBusMechanism(recoveryWatchOptions(defaults.Group), func(
		kind resource.Kind, clients resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		if err := validateBusOptions(defaults.Group, opts.ListWatchOptions); err != nil {
			return nil, err
		}
		full, err := periodicClientLister(kind, clients, opts, defaults.PageSize)
		if err != nil {
			return nil, err
		}
		lister, err := keysListerFor(kind, defaults.KeyListerFactory, full, ClientListerOptions{
			ListWatchOptions: opts.ListWatchOptions, PageSize: listPageSize(opts, defaults.PageSize),
		})
		if err != nil {
			return nil, err
		}
		workers := opts.MaxConcurrentWorkers
		if workers == 0 {
			workers = defaultConcurrentInformerWorkers
		}
		return NewBusInformer(kind, subscriber, lister, BusInformerOptions{
			Namespace: opts.ListWatchOptions.Namespace, Group: defaults.Group,
			ResyncInterval:     periodicOptions(opts, PeriodicMechanismOptions{ResyncInterval: defaults.ResyncInterval}).ResyncInterval,
			AllowDegradedStart: defaults.AllowDegradedStart, EventTimeout: opts.EventTimeout,
			MaxConcurrentWorkers: workers, ErrorHandler: opts.ErrorHandler,
		})
	})
}

func recoveryWatchOptions(group resourcebus.Group) WatchOptions {
	provides := busWatchOptions(group)
	provides.Resync = periodicWatchOptions(WatchDetailIdentity).Resync
	return provides
}

// NewBusInformer subscribes to changes and periodically reconciles an identity
// snapshot. The lister must cover the subscription's namespace. The LIST and
// notifications must use the same increasing numeric resource versions.
// An empty initial LIST can have snapshot version 0.
func NewBusInformer(kind resource.Kind, subscriber resourcebus.Subscriber, lister ResourceLister, opts BusInformerOptions) (*BusInformer, error) {
	if lister == nil {
		return nil, errors.New("bus recovery requires a lister")
	}
	b, err := newBusInformer(kind, subscriber, opts.Group, InformerOptions{
		ListWatchOptions: ListWatchOptions{Namespace: opts.Namespace},
		EventTimeout:     opts.EventTimeout, ErrorHandler: opts.ErrorHandler,
	})
	if err != nil {
		return nil, err
	}
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = defaultPeriodicInterval
	}
	b.recovery = &busRecovery{lister: lister, opts: opts, relist: make(chan struct{}, 1), overflowed: make(chan struct{}, 1),
		items: make(map[resource.Identifier]resource.Object), deleted: make(map[resource.Identifier]int64)}
	return b, nil
}

type busRecovery struct {
	lister             ResourceLister
	opts               BusInformerOptions
	relist             chan struct{}
	overflowed         chan struct{}
	nextOverflowRelist time.Time

	// The run loop alone mutates the snapshot and dispatches changes.
	items       map[resource.Identifier]resource.Object
	deleted     map[resource.Identifier]int64
	listVersion int64
}

func (b *BusInformer) runRecovery(parent context.Context) error {
	ctx, cancel := context.WithCancelCause(parent)
	var workers sync.WaitGroup
	b.startDeliveryWorkers(ctx, &workers)
	defer func() {
		cancel(nil)
		workers.Wait()
	}()
	r := b.recovery

	if r.opts.AllowDegradedStart {
		sub, err := b.openSubscription(ctx)
		switch {
		case err == nil:
			defer b.closeSubscription(ctx, sub)
		case subscriptionRejected(err):
			return err
		default:
			if ctx.Err() == nil {
				b.reportError(ctx, err)
				workers.Go(func() { b.recoverSubscription(ctx, cancel) })
			}
		}
	} else {
		sub, err := b.subscribe(ctx)
		if err != nil {
			if parent.Err() != nil {
				return nil
			}
			return err
		}
		defer b.closeSubscription(ctx, sub)
	}

	trigger := busRelistScheduled
	var resyncAt, retryAt time.Time
	for ctx.Err() == nil {
		err := b.relist(ctx, trigger != busRelistScheduled)
		now := time.Now()
		if trigger == busRelistOverflow {
			r.nextOverflowRelist = now.Add(overflowRelistInterval)
		}
		delay := r.opts.ResyncInterval
		if err != nil {
			if subscriptionRejected(err) {
				return err
			}
			if ctx.Err() != nil {
				break
			}
			b.reportError(ctx, err)
			delay = busRetryInterval
			if !b.hasSynced() {
				if !waitInformerInterval(ctx, delay) {
					break
				}
				continue
			}
		} else {
			retryAt = time.Time{}
			if !b.hasSynced() {
				close(b.synced)
			}
		}

		if trigger == busRelistScheduled {
			resyncAt = now.Add(wait.Jitter(delay, informerRetryJitter))
		} else if err != nil {
			retryAt = now.Add(wait.Jitter(delay, informerRetryJitter))
		}
		trigger = b.serveUntilRelist(ctx, resyncAt, retryAt)
		if trigger == busRelistStopped {
			break
		}
	}
	return recoveryRunError(parent, ctx)
}

func recoveryRunError(parent, run context.Context) error {
	if parent.Err() != nil {
		return nil
	}
	return context.Cause(run)
}

func (b *BusInformer) recoverSubscription(ctx context.Context, cancel context.CancelCauseFunc) {
	if !waitInformerInterval(ctx, busRetryInterval) {
		return
	}
	sub, err := b.subscribe(ctx)
	if err != nil {
		if ctx.Err() == nil {
			cancel(err)
		}
		return
	}
	defer b.closeSubscription(ctx, sub)
	b.recovery.signalRelist()
	<-ctx.Done()
}

const overflowRelistInterval = 30 * time.Second

type busRelistTrigger uint8

const (
	busRelistStopped busRelistTrigger = iota
	busRelistScheduled
	busRelistCatchUp
	busRelistOverflow
)

func (b *BusInformer) serveUntilRelist(ctx context.Context, resyncAt, retryAt time.Time) busRelistTrigger {
	r := b.recovery
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
	overflowEvents := (<-chan struct{})(r.overflowed)
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
		case <-r.relist:
			return busRelistCatchUp
		case <-overflowReady:
			return busRelistOverflow
		case <-overflowEvents:
			if remaining := time.Until(r.nextOverflowRelist); remaining > 0 {
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

func (r *busRecovery) signalRelist() {
	select {
	case r.relist <- struct{}{}:
	default:
	}
}

func (r *busRecovery) overflow() {
	select {
	case r.overflowed <- struct{}{}:
	default:
	}
}

func (b *BusInformer) applyLive(ctx context.Context, event resourcebus.Event) {
	if !b.acceptEvent(event) {
		return
	}
	r := b.recovery
	id := resource.Identifier{Namespace: event.Namespace, Name: event.Name}
	old := r.items[id]
	version := numericVersion(event.ResourceVersion)
	if version == 0 {
		b.reportError(ctx, fmt.Errorf("bus recovery notification for %v has an invalid resource version %q", id, event.ResourceVersion))
		return
	}
	if version <= r.listVersion || version <= r.deleted[id] ||
		(old != nil && version <= numericVersion(old.GetResourceVersion())) {
		return
	}

	switch event.Verb {
	case resourcebus.VerbAdded, resourcebus.VerbModified:
		obj := newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion)
		r.items[id] = obj
		delete(r.deleted, id)
		if event.Verb == resourcebus.VerbAdded {
			// A recreate can reach this group member even if its delete did not.
			old = nil
		}
		b.deliverChange(ctx, old, obj)
	case resourcebus.VerbDeleted:
		delete(r.items, id)
		r.deleted[id] = version
		if old == nil {
			old = newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion)
		}
		b.deliverChange(ctx, old, nil)
	default:
		b.reportError(ctx, fmt.Errorf("unknown bus verb %q", event.Verb))
	}
}

func (b *BusInformer) relist(ctx context.Context, changesOnly bool) error {
	r := b.recovery
	items, rv, err := r.lister.List(ctx)
	if err != nil {
		return fmt.Errorf("bus recovery LIST: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	version := numericVersion(rv)
	if version == 0 && (rv != "0" || len(items) != 0) {
		return fmt.Errorf("bus recovery LIST requires a numeric snapshot resource version, got %q", rv)
	}
	if version < r.listVersion {
		return fmt.Errorf("bus recovery LIST version %q predates installed snapshot %d", rv, r.listVersion)
	}
	next := make(map[resource.Identifier]resource.Object, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			return fmt.Errorf("bus recovery LIST at %q contains an item without identity", rv)
		}
		id := resource.Identifier{Namespace: item.GetNamespace(), Name: item.GetName()}
		if numericVersion(item.GetResourceVersion()) == 0 {
			return fmt.Errorf("bus recovery LIST at %q has an invalid item resource version %q for %v", rv, item.GetResourceVersion(), id)
		}
		if _, duplicate := next[id]; duplicate {
			return fmt.Errorf("bus recovery LIST at %q contains duplicate key %v", rv, id)
		}
		next[id] = newIdentityObject(b.kind, id.Namespace, id.Name, item.GetResourceVersion())
	}
	for id, obj := range next {
		old := r.items[id]
		// Grafana's first LIST page can see item writes committed after it
		// reads the collection version. Use the item's own version when present.
		itemVersion := numericVersion(obj.GetResourceVersion())
		if r.deleted[id] >= itemVersion {
			delete(next, id)
			continue
		}
		if old != nil && numericVersion(old.GetResourceVersion()) > itemVersion {
			next[id] = old
			continue
		}
		if changesOnly && old != nil && old.GetResourceVersion() == obj.GetResourceVersion() {
			continue
		}
		b.deliverChange(ctx, old, obj)
	}
	for id, old := range r.items {
		if next[id] != nil {
			continue
		}
		if numericVersion(old.GetResourceVersion()) > version {
			next[id] = old
			continue
		}
		b.deliverChange(ctx, old, nil)
	}
	for id, deleted := range r.deleted {
		if deleted <= version {
			delete(r.deleted, id)
		}
	}
	r.items, r.listVersion = next, version
	return nil
}

func numericVersion(version string) int64 {
	n, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return 0
	}
	return max(0, n)
}

type busDelivery struct {
	ctx     context.Context
	old     resource.Object
	current resource.Object
}

func (b *BusInformer) startDeliveryWorkers(ctx context.Context, workers *sync.WaitGroup) {
	count := b.recovery.opts.MaxConcurrentWorkers
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

func (b *BusInformer) deliverChange(ctx context.Context, old, current resource.Object) {
	d := busDelivery{ctx: ctx, old: old, current: current}
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

func (b *BusInformer) dispatchChange(d busDelivery) {
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
