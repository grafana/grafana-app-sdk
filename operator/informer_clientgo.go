package operator

import (
	"context"
	"errors"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

// DeltaSource is the subset of client-go's cache.SharedIndexInformer a controller uses to receive events: register
// a handler, whose registration reports when the initial list has been delivered, and run until stopped.
//
// A SharedIndexInformer satisfies it, and so does NewDeltaSource over any Informer, so a controller written against
// cache.ResourceEventHandler can take its events from either without knowing which.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type DeltaSource interface {
	AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	Run(stopCh <-chan struct{})
}

// InformerDeltaSource presents an Informer as a DeltaSource. See NewDeltaSource.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type InformerDeltaSource struct {
	informer Informer
	syncErr  error

	once     sync.Once
	syncedCh chan struct{}
}

var _ DeltaSource = (*InformerDeltaSource)(nil)

// NewDeltaSource presents an Informer as a client-go delta source, so that a controller written against
// cache.ResourceEventHandler takes its events from a bus or periodic informer unchanged.
//
// The informer must finish its initial callbacks before WaitForSync returns.
// Registration and RunWithContext reject ConcurrentInformer, which reports
// its underlying informer's sync before its own workers have finished.
//
// Two points of the mapping are worth knowing, both following from an SDK informer dispatching to handlers rather
// than handing deltas to a shared processor:
//
//   - isInInitialList follows the dispatch context of the first successful LIST.
//   - a delete the informer's re-list detected is wrapped in cache.DeletedFinalStateUnknown, which is what a
//     client-go handler reads as "the delete was missed, and the object carried with it may be stale". A delete the
//     live stream delivered is passed through as the object.
//
// Both read the delivery off the dispatch context, so an informer which reports DeliveryUnknown — a watch-backed one
// — delivers everything as live and tombstones nothing. There is no reason to wrap one: a SharedIndexInformer is
// already a DeltaSource.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewDeltaSource(informer Informer) *InformerDeltaSource {
	d := &InformerDeltaSource{informer: informer, syncedCh: make(chan struct{})}
	for inf := informer; inf != nil; {
		if _, queued := inf.(*ConcurrentInformer); queued {
			d.syncErr = errors.New("client-go adapter requires initial callbacks to complete before sync")
			break
		}
		wrapper, ok := inf.(interface{ Unwrap() Informer })
		if !ok {
			break
		}
		inf = wrapper.Unwrap()
	}
	return d
}

// AddEventHandler registers a client-go event handler on the informer. Add every handler before Run, as the
// informers this wraps require.
func (d *InformerDeltaSource) AddEventHandler(
	handler cache.ResourceEventHandler,
) (cache.ResourceEventHandlerRegistration, error) {
	if d.syncErr != nil {
		return nil, d.syncErr
	}
	if handler == nil {
		return nil, errors.New("handler cannot be nil")
	}
	if err := d.informer.AddEventHandler(&clientGoWatcher{handler: handler}); err != nil {
		return nil, err
	}
	return handlerRegistration{source: d}, nil
}

// Run runs the informer until stopCh is closed. client-go's signature has nowhere to return an error, so one which
// stopped the informer is logged; RunWithContext returns it instead.
func (d *InformerDeltaSource) Run(stopCh <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	if err := d.RunWithContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logging.DefaultLogger.Error("informer stopped with an error", "name", d.name(), "error", err)
	}
}

// RunWithContext runs the informer until ctx is done, returning what stopped it. It is Run for a caller which has a
// context and wants the error.
func (d *InformerDeltaSource) RunWithContext(ctx context.Context) error {
	if d.syncErr != nil {
		return d.syncErr
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The informers this wraps have no HasSynced of their own, so the sync state a client-go registration reports
	// is latched here. The goroutine ends with the run, since WaitForSync returns when the context is done.
	go func() {
		if err := d.informer.WaitForSync(ctx); err == nil {
			d.markSynced()
		}
	}()
	return d.informer.Run(ctx)
}

// HasSynced reports whether the informer has finished its initial LIST callbacks.
func (d *InformerDeltaSource) HasSynced() bool {
	select {
	case <-d.syncedCh:
		return true
	default:
		return false
	}
}

func (d *InformerDeltaSource) markSynced() {
	d.once.Do(func() { close(d.syncedCh) })
}

func (d *InformerDeltaSource) name() string {
	if schemaed, ok := d.informer.(interface{ Schema() resource.Schema }); ok {
		sch := schemaed.Schema()
		return schema.GroupVersionKind{Group: sch.Group(), Version: sch.Version(), Kind: sch.Kind()}.String()
	}
	return "informer"
}

// handlerRegistration is what AddEventHandler hands back: the sync state of the informer the handler was registered
// on. It is not an unregistration handle — the informers this wraps cannot remove a handler — so passing it to
// anything expecting to deregister with it will not do so.
type handlerRegistration struct {
	source *InformerDeltaSource
}

var _ cache.ResourceEventHandlerRegistration = handlerRegistration{}

func (r handlerRegistration) HasSynced() bool { return r.source.HasSynced() }

func (r handlerRegistration) HasSyncedChecker() cache.DoneChecker { return r }

func (r handlerRegistration) Name() string { return r.source.name() }

func (r handlerRegistration) Done() <-chan struct{} { return r.source.syncedCh }

// clientGoWatcher calls a client-go event handler for each of the informer's dispatches. The handler cannot return
// an error — client-go's events are informational — so neither does this.
type clientGoWatcher struct {
	handler cache.ResourceEventHandler
	mu      sync.Mutex
}

var _ ResourceWatcher = (*clientGoWatcher)(nil)

func (w *clientGoWatcher) Add(ctx context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handler.OnAdd(clientGoObject(obj), isInitialList(ctx))
	return nil
}

func (w *clientGoWatcher) Update(_ context.Context, src, tgt resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handler.OnUpdate(clientGoObject(src), clientGoObject(tgt))
	return nil
}

func (w *clientGoWatcher) Delete(ctx context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if DeliveryFromContext(ctx) == DeliveryRelist {
		// The informer inferred the delete from a list, so the object it carries predates it — which is exactly what
		// a tombstone tells a client-go handler.
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			key = obj.GetName()
		}
		w.handler.OnDelete(cache.DeletedFinalStateUnknown{Key: key, Obj: clientGoObject(obj)})
		return nil
	}
	w.handler.OnDelete(clientGoObject(obj))
	return nil
}

func clientGoObject(obj resource.Object) runtime.Object {
	if wrapped, ok := obj.(interface{ UnwrapObject() runtime.Object }); ok {
		return wrapped.UnwrapObject()
	}
	return obj
}
