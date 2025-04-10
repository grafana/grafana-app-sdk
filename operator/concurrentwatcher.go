package operator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/cespare/xxhash/v2"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	initialBufferSize = 1024
)

type eventInfo struct {
	ctx    context.Context
	action ResourceAction
	target resource.Object
	source resource.Object
}

// ConcurrentWatcher is a struct that implements ResourceWatcher, but takes no action on its own.
// For each method in (Add, Update, Delete) the event is added in a buffered queue and the corresponding
// methods of the underlying ResourceWatcher are called concurrently.
type ConcurrentWatcher struct {
	watcher ResourceWatcher
	size    uint64
	workers map[uint64]*bufferedQueue
}

func NewConcurrentWatcher(watcher ResourceWatcher, initialPoolSize uint64) (*ConcurrentWatcher, context.CancelFunc) {
	cw := &ConcurrentWatcher{
		watcher: watcher,
		size:    initialPoolSize,
		workers: make(map[uint64]*bufferedQueue),
	}

	var i uint64
	for i < initialPoolSize {
		cw.workers[i] = newBufferedQueue(initialBufferSize)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Start the workers in background as part of the watcher initialisation itself.
	go func(ctx context.Context) {
		cw.run(ctx)
	}(ctx)

	return cw, cancel
}

func (w *ConcurrentWatcher) Add(ctx context.Context, object resource.Object) error {
	worker := w.workers[w.hashMod(object)]
	worker.push(eventInfo{
		ctx:    ctx,
		action: ResourceActionCreate,
		target: object,
	})
	return nil
}

func (w *ConcurrentWatcher) Update(ctx context.Context, src resource.Object, tgt resource.Object) error {
	worker := w.workers[w.hashMod(src)]
	worker.push(eventInfo{
		ctx:    ctx,
		action: ResourceActionUpdate,
		target: tgt,
		source: src,
	})
	return nil
}

func (w *ConcurrentWatcher) Delete(ctx context.Context, object resource.Object) error {
	worker := w.workers[w.hashMod(object)]
	worker.push(eventInfo{
		ctx:    ctx,
		action: ResourceActionDelete,
		target: object,
	})
	return nil
}

func (w *ConcurrentWatcher) run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, queue := range w.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Start the background process to emit the events from queue.
			go queue.run()
			defer queue.stop()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			events := queue.events()
			wait.Until(func() {
				for next := range events {
					event, ok := next.(eventInfo)
					if !ok {
						utilruntime.HandleError(fmt.Errorf("unrecognized notification: %T", next))
					}

					switch event.action {
					case ResourceActionCreate:
						_ = w.watcher.Add(event.ctx, event.target)
					case ResourceActionUpdate:
						_ = w.watcher.Update(event.ctx, event.source, event.target)
					case ResourceActionDelete:
						_ = w.watcher.Delete(event.ctx, event.target)
					default:
						utilruntime.HandleError(fmt.Errorf("invalid event type: %T", event.action))
					}
				}
				// the only way to get here is if the l.toProcess is empty and closed
				cancel()
			}, 1*time.Second, ctx.Done())
		}()
	}

	wg.Wait()
}

func (w *ConcurrentWatcher) hashMod(obj resource.Object) uint64 {
	id := obj.GroupVersionKind().String() + "/" + obj.GetNamespace() + "/" + obj.GetName()
	digest := xxhash.Sum64([]byte(id))

	return digest % w.size
}
