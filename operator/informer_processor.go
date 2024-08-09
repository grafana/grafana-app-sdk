package operator

import (
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/logging"
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
	listeners    map[*informerProcessorListener]bool
	listenersMux sync.RWMutex
	wg           wait.Group
	started      bool
	startedCh    chan struct{}
}

func newInformerProcessor() *informerProcessor {
	return &informerProcessor{
		listeners: make(map[*informerProcessorListener]bool),
	}
}

func (p *informerProcessor) addListener(l *informerProcessorListener) {
	p.listenersMux.Lock()
	defer p.listenersMux.Unlock()
	p.listeners[l] = true
}

func (p *informerProcessor) distribute(event any) {
	if !p.started {
		// Drop events if we're not started to prevent us from not being able to start if listener.push() blocks
		if logging.DefaultLogger != nil {
			logging.DefaultLogger.Warn("Received event for informer distribution while processor is not started, dropping event")
		}
		return
	}
	p.listenersMux.RLock()
	defer p.listenersMux.RUnlock()
	for listener := range p.listeners {
		listener.push(event)
	}
}

func (p *informerProcessor) run(stopCh <-chan struct{}) {
	p.listenersMux.Lock()
	for listener := range p.listeners {
		go func(l *informerProcessorListener) {
			p.wg.Start(l.run)
		}(listener)
	}
	p.started = true
	p.listenersMux.Unlock()
	if p.startedCh != nil {
		// Run in a goroutine to prevent blocking if the channel buffer is full
		go func() {
			p.startedCh <- struct{}{}
		}()
	}
	<-stopCh
	p.listenersMux.Lock()
	p.started = false
	for listener := range p.listeners {
		listener.stop()
	}
	p.listenersMux.Unlock()

	p.wg.Wait()
}

type informerProcessorListener struct {
	events    chan any
	handler   cache.ResourceEventHandler
	buf       buffer.RingGrowing
	toProcess chan any
}

func newInformerProcessorListener(handler cache.ResourceEventHandler, bufferSize int) *informerProcessorListener {
	return &informerProcessorListener{
		events:    make(chan any),
		toProcess: make(chan any),
		buf:       *buffer.NewRingGrowing(bufferSize),
		handler:   handler,
	}
}

func (l *informerProcessorListener) push(event any) {
	l.events <- event
}

// pop will continuously read messages from the events channel, and write them to a buffer.
// while any contents exist in the buffer, it will also attempt to write them out to the toProcess channel.
// This allows writes to the events channel to not be blocked by processing of the handler, which instead consumes
// from the toProcess channel.
func (l *informerProcessorListener) pop() {
	// TODO: should this whole thing be a goroutine in run(), rather than a separate method?
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
		case eventToAdd, ok := <-l.events:
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

// run starts pop to move events from the events channel to the toProcess channel,
// then reads events from the toProcess channel and dispatches them to the handler based on event type
func (l *informerProcessorListener) run() {
	go l.pop()

	stopCh := make(chan struct{})
	wait.Until(func() {
		for next := range l.toProcess {
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

// stop stops the run and pop processes. Because the channels are closed, the listener cannot be re-used.
func (l *informerProcessorListener) stop() {
	close(l.events)
}
