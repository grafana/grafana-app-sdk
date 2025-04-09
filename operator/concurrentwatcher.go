package operator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/grafana/grafana-app-sdk/resource"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/buffer"
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

type worker struct {
	toProcess chan eventInfo
}

// ConcurrentWatcher is a struct that implements ResourceWatcher, but takes no action on its own.
// For each method in (Add, Update, Delete) the event is added in a buffered queue and the corresponding
// methods of the underlying ResourceWatcher are called concurrently.
type ConcurrentWatcher struct {
	watcher ResourceWatcher
	size    uint64
	workers map[uint64]*bufferedQueue
}

func NewConcurrentWatcher(watcher ResourceWatcher, initialPoolSize uint64) *ConcurrentWatcher {
	cw := &ConcurrentWatcher{
		watcher: watcher,
		size:    initialPoolSize,
		workers: make(map[uint64]*bufferedQueue),
	}

	var i uint64 = 0
	for i < initialPoolSize {
		cw.workers[i] = newBufferedQueue(initialBufferSize)
	}

	// Start the workers in background as part of the watcher initialisation itself.
	go func() {
		cw.run(context.TODO())
	}()

	return cw
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
	for _, v := range w.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			events := v.events()

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

type bufferedQueue struct {
	incomingEvents chan any
	toProcess      chan any
	buf            buffer.RingGrowing
}

func newBufferedQueue(bufferSize int) *bufferedQueue {
	return &bufferedQueue{
		incomingEvents: make(chan any),
		toProcess:      make(chan any),
		buf:            *buffer.NewRingGrowing(bufferSize),
	}
}

func (l *bufferedQueue) events() chan any {
	return l.toProcess
}

func (l *bufferedQueue) push(event any) {
	l.incomingEvents <- event
}

// run will continuously read messages from the events channel, and write them to a buffer.
// while any contents exist in the buffer, it will also attempt to write them out to the toProcess channel.
// This allows writes to the events channel to not be blocked by processing of the events, which instead consumes
// from the toProcess channel.
func (l *bufferedQueue) run() {
	defer close(l.toProcess)

	var nextCh chan<- any
	var event any
	var ok bool
	for {
		select {
		case nextCh <- event:
			event, ok = l.buf.ReadOne()
			if !ok {
				nextCh = nil
			}
		case eventToAdd, ok := <-l.incomingEvents:
			if !ok {
				return
			}
			if event == nil {
				event = eventToAdd
				nextCh = l.toProcess
			} else { // There is already a notification waiting to be dispatched
				l.buf.WriteOne(eventToAdd)
			}
		}
	}
}

// stop stops the run processes. Because the channels are closed, the listener cannot be re-used.
func (l *bufferedQueue) stop() {
	close(l.incomingEvents)
}
