package operator

import (
	"context"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/grafana/grafana-app-sdk/resource"
)

var (
	defaultBuffer = 1000
)

type eventInfo struct {
	action ResourceAction
	target resource.Object
	old    resource.Object
}

type worker struct {
	toProcess chan eventInfo
}

// ConcurrentWatcher is a struct that implements ResourceWatcher, but takes no action on its own.
// For each method in (Add, Update, Delete) the event is added in a queue and the corresponding
// methods of the underlying ResourceWatcher are called concurrently.
type ConcurrentWatcher struct {
	watcher ResourceWatcher
	size    uint64
	workers map[uint64]*worker
}

func NewConcurrentWatcher(watcher ResourceWatcher, initialPoolSize uint64) *ConcurrentWatcher {
	cw := &ConcurrentWatcher{
		watcher: watcher,
		size:    initialPoolSize,
		workers: make(map[uint64]*worker),
	}

	var i uint64 = 0
	for i < initialPoolSize {
		cw.workers[i] = &worker{
			toProcess: make(chan eventInfo, defaultBuffer),
		}
	}

	return cw
}

func (w *ConcurrentWatcher) Add(ctx context.Context, object resource.Object) error {
	worker := w.workers[w.hashMod(object)]
	worker.toProcess <- eventInfo{
		action: ResourceActionCreate,
		target: object,
	}
	return nil
}

func (w *ConcurrentWatcher) Update(ctx context.Context, src resource.Object, tgt resource.Object) error {
	worker := w.workers[w.hashMod(src)]
	worker.toProcess <- eventInfo{
		action: ResourceActionUpdate,
		target: tgt,
		old:    src,
	}
	return nil
}

func (w *ConcurrentWatcher) Delete(ctx context.Context, object resource.Object) error {
	worker := w.workers[w.hashMod(object)]
	worker.toProcess <- eventInfo{
		action: ResourceActionDelete,
		target: object,
	}
	return nil
}

func (w *ConcurrentWatcher) run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, v := range w.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case item, ok := <-v.toProcess:
					if !ok {
						return
					}
					switch item.action {
					case ResourceActionCreate:
						_ = w.watcher.Add(ctx, item.target)
					case ResourceActionUpdate:
						_ = w.watcher.Update(ctx, item.old, item.target)
					case ResourceActionDelete:
						_ = w.watcher.Delete(ctx, item.target)
					}
				}
			}
		}()
	}

	wg.Wait()
}

func (w *ConcurrentWatcher) hashMod(obj resource.Object) uint64 {
	id := obj.GroupVersionKind().String() + "/" + obj.GetNamespace() + "/" + obj.GetName()
	digest := xxhash.Sum64([]byte(id))

	return digest % w.size
}
