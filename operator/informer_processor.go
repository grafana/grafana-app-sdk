package operator

import (
	"errors"
	"fmt"
	"sync"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/buffer"
)

/*
informer_processor.go is based heavily on the processor and listener (sharedProcessor and processorListener) in
kubernetes' tools/cache/shared_informer.go. Since these types are unexported, they could not be re-used
by the CustomCacheInformer and require re-implementation.
*/

type informerEventAdd struct {
	obj             any
	isInInitialList bool
}

type informerEventUpdate struct {
	obj any
	old any
}
type informerEventDelete struct {
	obj any
}
type informerEventCacheSync struct {
	obj any
}

type informerProcessor struct {
	listeners map[*informerProcessorListener]struct{}
	wg        wait.Group
	startMux  sync.RWMutex
	started   bool
	startedCh chan struct{}
}

func newInformerProcessor() *informerProcessor {
	return &informerProcessor{
		listeners: make(map[*informerProcessorListener]struct{}),
	}
}

func (p *informerProcessor) addListener(l *informerProcessorListener) error {
	p.startMux.Lock()
	defer p.startMux.Unlock()

	if p.started {
		return errors.New("error adding listener: not allowed after processor already started")
	}

	p.listeners[l] = struct{}{}
	return nil
}

func (p *informerProcessor) distribute(event any) {
	// Block until the processor is started.
	<-p.startedCh

	// Distribute the event to all listeners.
	for listener := range p.listeners {
		listener.push(event)
	}
}

func (p *informerProcessor) run(stopCh <-chan struct{}) {
	// Start everything, use a mutex to prevent race conditions.
	p.startMux.Lock()
	for listener := range p.listeners {
		p.wg.Start(listener.run)
	}
	p.started = true
	p.startMux.Unlock()

	// Signal that the processor has started.
	// Run in a goroutine to prevent blocking if the channel buffer is full
	if p.startedCh != nil {
		go func() {
			close(p.startedCh)
		}()
	}

	// Wait for the processor to be stopped
	<-stopCh

	// Stop everything, use a mutex to prevent race conditions.
	p.startMux.Lock()
	p.started = false
	for listener := range p.listeners {
		listener.stop()
	}
	p.startMux.Unlock()

	// Wait for all listeners to stop.
	p.wg.Wait()
}

type informerProcessorListener struct {
	handler cache.ResourceEventHandler
	queue   *bufferedQueue
}

func newInformerProcessorListener(handler cache.ResourceEventHandler, bufferSize int) *informerProcessorListener {
	return &informerProcessorListener{
		queue:   newBufferedQueue(bufferSize),
		handler: handler,
	}
}

func (l *informerProcessorListener) push(event any) {
	l.queue.push(event)
}

// run starts the queue (to start receiving events) in the background, and
// then reads events from the queue's output channel and dispatches them to
// the handler based on event type.
func (l *informerProcessorListener) run() {
	go l.queue.run()

	stopCh := make(chan struct{})
	toProcess := l.queue.events()
	wait.Until(func() {
		for next := range toProcess {
			switch event := next.(type) {
			case informerEventAdd:
				l.handler.OnAdd(event.obj, event.isInInitialList)
			case informerEventUpdate:
				l.handler.OnUpdate(event.old, event.obj)
			case informerEventDelete:
				l.handler.OnDelete(event.obj)
			case informerEventCacheSync:
				l.handler.OnUpdate(event.obj, event.obj)
			default:
				utilruntime.HandleError(fmt.Errorf("unrecognized notification: %T", next))
			}
		}
		// the only way to get here is if the l.toProcess is empty and closed
		close(stopCh)
	}, 1*time.Second, stopCh)
}

// stop stops the run process. Because the underlying queue is closed, the listener cannot be re-used.
func (l *informerProcessorListener) stop() {
	l.queue.stop()
}

// bufferedQueue is a FIFO queue that allows concurrent listeners by streaming
// events to a channel. The queue uses a growing ring buffer to avoid blocking
// event push.
type bufferedQueue struct {
	incomingEvents chan any
	toProcess      chan any
	buf            *buffer.RingGrowing //nolint:staticcheck
	stopMux        sync.RWMutex
	stopped        bool
}

// newBufferedQueue returns a properly initialized bufferedQueue. The consumer
// need to run `bufferedQueue.run()` method to start receiving the events.
func newBufferedQueue(bufferSize int) *bufferedQueue {
	return &bufferedQueue{
		incomingEvents: make(chan any),
		toProcess:      make(chan any),
		buf:            buffer.NewRingGrowing(bufferSize), //nolint:staticcheck
	}
}

// events returns the output channel of the queue where the events will
// be streamed for consumption.
func (l *bufferedQueue) events() chan any {
	return l.toProcess
}

// push inserts an event in the queue.
func (l *bufferedQueue) push(event any) {
	l.stopMux.RLock()
	defer l.stopMux.RUnlock()

	if l.stopped {
		return
	}

	l.incomingEvents <- event
}

// run will continuously read messages from the events channel, and write them to a buffer.
// while any contents exist in the buffer, it will also attempt to write them out to the toProcess channel.
// This allows writes to the events channel to not be blocked by processing of the events, which instead
// consumes from the toProcess channel.
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
	l.stopMux.Lock()
	defer l.stopMux.Unlock()

	l.stopped = true
	close(l.incomingEvents)
}
