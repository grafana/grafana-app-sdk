package operator

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/types"

	"github.com/grafana/grafana-app-sdk/resource"
)

// objectOperations serializes finalizer transitions for one owner. Only
// objects with an active operation have an entry.
type objectOperations struct {
	active sync.Map
}

type interruptedDelivery struct{ error }

func (e interruptedDelivery) Unwrap() error { return e.error }

func (o *objectOperations) acquire(ctx context.Context, id resource.Identifier) (release func(), waited bool, err error) {
	done := make(chan struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, waited, interruptedDelivery{err}
		}
		other, loaded := o.active.LoadOrStore(id, done)
		if !loaded {
			return func() {
				o.active.Delete(id)
				close(done)
			}, waited, nil
		}
		waited = true
		select {
		case <-other.(chan struct{}):
		case <-ctx.Done():
			return nil, waited, interruptedDelivery{ctx.Err()}
		}
	}
}

type syncFailureKey struct{}

// A generation-zero resource may have no comparable spec (for example, a
// wrapped Kubernetes object). Keep the latest notification while a Sync is due.
type watcherSyncRetry struct {
	mu        sync.Mutex
	target    resource.Object
	finalizer string
}

func (s *watcherSyncRetry) observe(action ResourceAction, obj resource.Object) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if action == ResourceActionDelete || obj.GetUID() != s.target.GetUID() ||
		obj.GetDeletionTimestamp() != nil || !slices.Contains(obj.GetFinalizers(), s.finalizer) {
		return false
	}
	s.target = obj.Copy()
	return true
}

func (s *watcherSyncRetry) snapshot() resource.Object {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.target.Copy()
}

// Registrations have stable retry keys even when an earlier handler is removed.
type handlerRegistration[T any] struct {
	handler  T
	id       int
	lifetime context.Context
	cancel   context.CancelFunc
}

type handlerList[T comparable] struct {
	*ListMap[string, *handlerRegistration[T]]
	addMu sync.Mutex
	next  int
}

func newHandlerList[T comparable]() *handlerList[T] {
	return &handlerList[T]{ListMap: NewListMap[*handlerRegistration[T]]()}
}

func (l *handlerList[T]) add(kind string, handler T) {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // Removal cancels this registration.
	// Deferred deliveries use the ID to preserve registration order.
	l.addMu.Lock()
	defer l.addMu.Unlock()
	l.AddItem(kind, &handlerRegistration[T]{handler: handler, id: l.next, lifetime: ctx, cancel: cancel})
	l.next++
}

func (l *handlerList[T]) remove(kind string, handler T) {
	l.RemoveItem(kind, func(reg *handlerRegistration[T]) bool {
		if reg.handler != handler {
			return false
		}
		reg.cancel()
		return true
	})
}

func (l *handlerList[T]) removeAll(kind string) {
	for _, reg := range l.takeKey(kind) {
		reg.cancel()
	}
}

// Deferred work belongs to both the informer run that received the event and
// the handler registration. Share that context with any retries it schedules.
type registeredInformer struct {
	Informer
	run atomic.Pointer[informerRun]
}

type informerRun struct {
	ctx      context.Context
	mu       sync.Mutex
	handlers map[context.Context]context.Context
	cleanups map[*deliveryOrder]*cleanupQueue
}

func (i *registeredInformer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	run := &informerRun{
		ctx:      ctx,
		handlers: make(map[context.Context]context.Context),
		cleanups: make(map[*deliveryOrder]*cleanupQueue),
	}
	i.run.Store(run)
	defer i.run.CompareAndSwap(run, nil)
	defer cancel()
	return i.Informer.Run(ctx)
}

func (r *informerRun) workContext(owner context.Context) context.Context {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil || owner.Err() != nil {
		return nil
	}
	if existing := r.handlers[owner]; existing != nil {
		return existing
	}
	ctx, cancel := context.WithCancel(r.ctx)
	stop := context.AfterFunc(owner, cancel)
	r.handlers[owner] = ctx
	context.AfterFunc(ctx, func() {
		stop()
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.handlers, owner)
		if r.ctx.Err() != nil {
			clear(r.cleanups)
			return
		}
		for _, queue := range r.cleanups {
			queue.pending = slices.DeleteFunc(queue.pending, func(d deferredDelivery) bool { return d.owner == owner })
		}
	})
	return ctx
}

const (
	watcherCleanup = iota
	reconcilerCleanup
)

type cleanupOrder struct{ group, id int }

func (a cleanupOrder) compare(b cleanupOrder) int {
	return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.id, b.id))
}

type deferredDelivery struct {
	order    cleanupOrder
	cleanup  bool
	owner    context.Context
	live     context.Context
	running  []<-chan struct{}
	dispatch func(context.Context)
}

func (d deferredDelivery) resume() {
	if d.live.Err() == nil && d.owner.Err() == nil {
		d.dispatch(d.live)
	}
}

// pending stays in registration order. A redelivery for an owner whose
// callback has started gets one slot on the next pass.
type cleanupQueue struct {
	pending []deferredDelivery
	last    cleanupOrder
	changed chan struct{}
}

type deliveryKey struct {
	kind string
	id   resource.Identifier
}

type deliveryOrder struct {
	key         deliveryKey
	uid         types.UID
	terminating bool
	previous    chan struct{}
	done        chan struct{}
	active      int          // admissions and workers holding this position, under deliveryMu
	deferredRun *informerRun // set once this run has queued its interrupted fan-out
}

// Consecutive terminating snapshots of one UID can share cleanup. A hard
// delete or a replacement takes its own position behind that cleanup.
func (c *InformerController) beginDelivery(key deliveryKey, action ResourceAction, obj resource.Object, run *informerRun) (*deliveryOrder, bool) {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	last := c.deliveries[key]
	terminating := action != ResourceActionDelete && obj.GetDeletionTimestamp() != nil
	if terminating && last != nil && run != nil && last.deferredRun == run && last.uid == obj.GetUID() {
		last.active++
		return last, true
	}
	order := &deliveryOrder{key: key, uid: obj.GetUID(), terminating: terminating, done: make(chan struct{}), active: 1}
	if last != nil {
		order.previous = last.done
	}
	c.deliveries[key] = order
	return order, false
}

func (c *InformerController) releaseDelivery(order *deliveryOrder) {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	order.active--
	if order.active != 0 {
		return
	}
	// A stopped source may leave no callbacks. Keep its predecessor in
	// order for a successor delivered through another informer.
	if order.previous != nil && !retryFinished(order.previous) {
		order.active++
		go func() {
			<-order.previous
			c.releaseDelivery(order)
		}()
		return
	}
	if c.deliveries[order.key] == order {
		delete(c.deliveries, order.key)
	}
	close(order.done)
}

func (c *InformerController) finishDelivery(order *deliveryOrder, r *informerRun, batch []deferredDelivery) {
	if len(batch) == 0 {
		c.releaseDelivery(order)
		return
	}
	if order.terminating {
		if r.queueCleanup(order, batch) {
			c.deliveryMu.Lock()
			order.deferredRun = r
			c.deliveryMu.Unlock()
			go func() {
				defer c.releaseDelivery(order)
				r.drainCleanup(order)
			}()
		} else {
			c.releaseDelivery(order)
		}
		return
	}
	go func() {
		defer c.releaseDelivery(order)
		for _, d := range batch {
			if waitForRetries(d.live, d.running) == nil {
				d.resume()
			}
		}
	}()
}

// queueCleanup reports whether the caller must start the object's worker.
func (r *informerRun) queueCleanup(key *deliveryOrder, batch []deferredDelivery) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return false
	}
	q := r.cleanups[key]
	start := q == nil
	for _, d := range batch {
		if d.owner.Err() != nil {
			continue
		}
		if q == nil {
			q = &cleanupQueue{last: cleanupOrder{id: -1}, changed: make(chan struct{}, 1)}
			r.cleanups[key] = q
		}
		idx, found := slices.BinarySearchFunc(q.pending, d.order, func(existing deferredDelivery, order cleanupOrder) int {
			return existing.order.compare(order)
		})
		// Only finalizer cleanup reads current state and can stand in for a
		// redelivery. Other handlers need each original action and payload.
		for found && idx < len(q.pending) && q.pending[idx].order == d.order {
			idx++
		}
		if found && d.cleanup && q.pending[idx-1].cleanup {
			existing := &q.pending[idx-1]
			existing.running = slices.DeleteFunc(existing.running, retryFinished)
			for _, done := range d.running {
				if !retryFinished(done) && !slices.Contains(existing.running, done) {
					existing.running = append(existing.running, done)
				}
			}
			continue
		}
		d.running = slices.DeleteFunc(d.running, retryFinished)
		q.pending = slices.Insert(q.pending, idx, d)
		// A newly admitted watcher may precede the owner we are waiting for.
		select {
		case q.changed <- struct{}{}:
		default:
		}
	}
	return start && q != nil
}

func retryFinished(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (r *informerRun) drainCleanup(key *deliveryOrder) {
	for {
		r.mu.Lock()
		q := r.cleanups[key]
		if q == nil || len(q.pending) == 0 || r.ctx.Err() != nil {
			delete(r.cleanups, key)
			r.mu.Unlock()
			return
		}
		idx := slices.IndexFunc(q.pending, func(d deferredDelivery) bool { return d.order.compare(q.last) > 0 })
		idx = max(idx, 0)
		d := &q.pending[idx]
		d.running = slices.DeleteFunc(d.running, retryFinished)
		if len(d.running) > 0 && d.owner.Err() == nil {
			done, live, changed := d.running[0], d.live, q.changed
			r.mu.Unlock()
			select {
			case <-done:
			case <-live.Done():
			case <-changed:
			}
			// Redeliveries can discover another running retry while we wait.
			continue
		}
		work := *d
		q.last = d.order
		q.pending = slices.Delete(q.pending, idx, idx+1)
		r.mu.Unlock()
		work.resume()
	}
}

func waitForRetries(ctx context.Context, running []<-chan struct{}) error {
	for _, done := range running {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}
