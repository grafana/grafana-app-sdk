package operator

import (
	"context"
	"sync"

	"github.com/grafana/grafana-app-sdk/resource"
)

var _ Informer = &ConcurrentInformer{}

// ConcurrentInformer implements the Informer interface, wrapping another Informer implementation
// to provide concurrent handling of events.
// Events will still be emitted sequentially, but the event handler methods on added ResourceWatchers
// (ie the business logic) will be processed by concurrent workers. Events for an object will be assigned
// to the same worker to preserve the per-object in-order guarantee provided by K8s client tooling.
type ConcurrentInformer struct {
	ErrorHandler func(context.Context, error)

	informer             Informer
	watchers             []*concurrentWatcher
	maxConcurrentWorkers uint64

	mtx sync.RWMutex
}

// NewConcurrentInformer creates a new ConcurrentInformer wrapping the provided Informer.
func NewConcurrentInformer(inf Informer, maxConcurrentWorkers uint64) (
	*ConcurrentInformer, error) {
	return &ConcurrentInformer{
		ErrorHandler:         DefaultErrorHandler,
		informer:             inf,
		watchers:             make([]*concurrentWatcher, 0),
		maxConcurrentWorkers: maxConcurrentWorkers,
	}, nil
}

// AddEventHandler adds a ResourceWatcher as an event handler for watch events from the informer.
// The ResourceWatcher is wrapped before adding it to the underlying Informer, to allow concurrent
// handling of the events.
// Event handlers are not guaranteed to be executed in parallel or in any particular order by the underlying
// Informer. If you want to coordinate between ResourceWatchers, use an InformerController.
// nolint:dupl
func (k *ConcurrentInformer) AddEventHandler(handler ResourceWatcher) error {
	cw, err := newConcurrentWatcher(handler, k.maxConcurrentWorkers, k.ErrorHandler)
	if err != nil {
		return err
	}

	{
		k.mtx.Lock()
		k.watchers = append(k.watchers, cw)
		k.mtx.Unlock()
	}

	return k.informer.AddEventHandler(cw)
}

func (k *ConcurrentInformer) Kind() resource.Kind {
	return k.Kind()
}

// Run starts the informer and blocks until stopCh receives a message
func (k *ConcurrentInformer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	k.mtx.RLock()
	for _, cw := range k.watchers {
		go cw.Run(ctx)
	}
	k.mtx.RUnlock()

	return k.informer.Run(ctx)
}
