package operator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
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
		return NewBusInformer(kind, subscriber, lister, BusInformerOptions{
			Namespace: opts.ListWatchOptions.Namespace, Group: defaults.Group,
			ResyncInterval:     periodicOptions(opts, PeriodicMechanismOptions{ResyncInterval: defaults.ResyncInterval}).ResyncInterval,
			AllowDegradedStart: defaults.AllowDegradedStart, EventTimeout: opts.EventTimeout,
			MaxConcurrentWorkers: opts.MaxConcurrentWorkers, ErrorHandler: opts.ErrorHandler,
		})
	})
}

func recoveryWatchOptions(group resourcebus.Group) WatchOptions {
	provides := busWatchOptions(group)
	provides.Resync = periodicWatchOptions(WatchDetailIdentity).Resync
	return provides
}

// NewBusInformer subscribes to changes and periodically reconciles an identity
// snapshot. The lister must cover the subscription's namespace. Numeric
// positive resource versions must be monotonic across the subscription and LIST.
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
	b.recovery = &busRecovery{lister: lister, opts: opts, relist: make(chan struct{}, 1),
		items: make(map[resource.Identifier]resource.Object), deleted: make(map[resource.Identifier]int64)}
	return b, nil
}

type busRecovery struct {
	lister       ResourceLister
	opts         BusInformerOptions
	relist       chan struct{}
	lastOverflow atomic.Int64

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

	delay := time.Duration(0)
	for ctx.Err() == nil {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return recoveryRunError(parent, ctx)
		case <-timer.C:
		}
		delay = r.opts.ResyncInterval
		if err := b.relist(ctx); err != nil {
			if subscriptionRejected(err) {
				return err
			}
			if ctx.Err() != nil {
				break
			}
			b.reportError(ctx, err)
			delay = busRetryInterval
			if !b.hasSynced() {
				delay = wait.Jitter(delay, informerRetryJitter)
				continue
			}
		} else if !b.hasSynced() {
			close(b.synced)
		}

		if !b.serveUntilRelist(ctx, delay) {
			break
		}
		delay = 0
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

func (b *BusInformer) serveUntilRelist(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(wait.Jitter(delay, informerRetryJitter))
	defer timer.Stop()
	for ctx.Err() == nil {
		// Prioritize recovery even when events arrive continuously.
		select {
		case <-timer.C:
			return true
		case <-b.recovery.relist:
			return true
		default:
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		case <-b.recovery.relist:
			return true
		case event := <-b.events:
			b.applyLive(ctx, event)
		}
	}
	return false
}

func (r *busRecovery) signalRelist() {
	select {
	case r.relist <- struct{}{}:
	default:
	}
}

func (r *busRecovery) overflow() {
	now := time.Now().UnixNano()
	last := r.lastOverflow.Load()
	if now-last >= int64(30*time.Second) && r.lastOverflow.CompareAndSwap(last, now) {
		r.signalRelist()
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
	if version > 0 && (version <= r.listVersion || version <= r.deleted[id] ||
		(old != nil && version <= numericVersion(old.GetResourceVersion()))) {
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
		if version > 0 {
			r.deleted[id] = version
		}
		if old == nil {
			old = newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion)
		}
		b.deliverChange(ctx, old, nil)
	default:
		b.reportError(ctx, fmt.Errorf("unknown bus verb %q", event.Verb))
	}
}

func (b *BusInformer) relist(ctx context.Context) error {
	r := b.recovery
	items, rv, err := r.lister.List(ctx)
	if err != nil {
		return fmt.Errorf("bus recovery LIST: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	version := numericVersion(rv)
	if version > 0 && version < r.listVersion {
		return fmt.Errorf("bus recovery LIST version %q predates installed snapshot %d", rv, r.listVersion)
	}
	next := make(map[resource.Identifier]resource.Object, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			return fmt.Errorf("bus recovery LIST at %q contains an item without identity", rv)
		}
		id := resource.Identifier{Namespace: item.GetNamespace(), Name: item.GetName()}
		if _, duplicate := next[id]; duplicate {
			return fmt.Errorf("bus recovery LIST at %q contains duplicate key %v", rv, id)
		}
		next[id] = newIdentityObject(b.kind, id.Namespace, id.Name, item.GetResourceVersion())
	}
	for id, obj := range next {
		old := r.items[id]
		if version > 0 && r.deleted[id] > version {
			delete(next, id)
			continue
		}
		if version > 0 && old != nil && numericVersion(old.GetResourceVersion()) > version {
			next[id] = old
			continue
		}
		b.deliverChange(ctx, old, obj)
	}
	for id, old := range r.items {
		if next[id] != nil {
			continue
		}
		if version > 0 && numericVersion(old.GetResourceVersion()) > version {
			next[id] = old
			continue
		}
		b.deliverChange(ctx, old, nil)
	}
	for id, deleted := range r.deleted {
		if version == 0 || deleted <= version {
			delete(r.deleted, id)
		}
	}
	r.items, r.listVersion = next, max(r.listVersion, version)
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
