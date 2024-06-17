package operator

import (
	"sync"

	"k8s.io/client-go/tools/cache"
)

type informerEventType string

const (
	informerEventAdd    informerEventType = informerEventType(ResourceActionCreate)
	informerEventUpdate informerEventType = informerEventType(ResourceActionUpdate)
	informerEventDelete informerEventType = informerEventType(ResourceActionDelete)
)

// informerProcessor is a very rough processor based loosely on the kubernetes cache.sharedProcessor.
// It is used by the CustomCacheInformer to distribute events to registered ResourceWatcher implementations
// TODO: this should be improved--how much do we want to pull from sharedProcessor?
type informerProcessor struct {
	listeners    map[*informerProcessorListener]bool
	listenersMux sync.RWMutex
}

func (p *informerProcessor) addListener(l *informerProcessorListener) {
	p.listenersMux.Lock()
	defer p.listenersMux.Unlock()
	p.listeners[l] = true
}

func (p *informerProcessor) distribute(event informerProcessorListenerEvent) {
	p.listenersMux.RLock()
	defer p.listenersMux.RUnlock()
	for listener, _ := range p.listeners {
		listener.events <- event
	}
}

func (p *informerProcessor) run(stopCh <-chan struct{}) {
	for listener, _ := range p.listeners {
		go func(l *informerProcessorListener) {
			l.run()
		}(listener)
	}
	for range stopCh {
		return
	}
}

type informerProcessorListenerEvent struct {
	eventType informerEventType
	obj       any
	old       any
	isList    bool
}

type informerProcessorListener struct {
	events  chan informerProcessorListenerEvent
	handler cache.ResourceEventHandler
}

func (l *informerProcessorListener) run() {
	for event := range l.events {
		switch event.eventType {
		case informerEventAdd:
			l.handler.OnAdd(event.obj, event.isList)
		case informerEventUpdate:
			l.handler.OnUpdate(event.old, event.obj)
		case informerEventDelete:
			l.handler.OnDelete(event.obj)
		}
	}
}
