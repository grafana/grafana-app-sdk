package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"go.opentelemetry.io/otel/attribute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var _ Informer = &CustomCacheInformer{}

const processorBufferSize = 1024

type CustomCacheInformer struct {
	// CacheResyncInterval is the interval at which the informer will emit CacheResync events for all resources in the cache.
	// This is distinct from a full resync, as no information is fetched from the API server.
	// Changes to this value after run() is called will not take effect.
	CacheResyncInterval time.Duration

	started           bool
	startedLock       sync.Mutex
	store             cache.Store
	controller        cache.Controller
	listerWatcher     cache.ListerWatcher
	objectType        resource.Object
	objectDescription string
	processor         *informerProcessor
}

// NewMemcachedInformer creates a new CustomCacheInformer which uses memcached as its custom cache.
// This is analogous to calling NewCustomCacheInformer with a MemcachedStore as the store.
func NewMemcachedInformer(kind resource.Kind, client ListWatchClient, namespace string, addrs ...string) *CustomCacheInformer {
	c := NewMemcachedStore(kind, MemcachedStoreConfig{
		Addrs:     addrs,
		TrackKeys: true,
	})
	return NewCustomCacheInformer(c, ListerWatcher(client, kind, namespace), kind.ZeroValue())
}

// NewMemcachedInformerWithLabelFilters creates a new CustomCacheInformer which uses memcached as its custom cache.
// This is analogous to calling NewCustomCacheInformer with a MemcachedStore as the store.
func NewMemcachedInformerWithLabelFilters(kind resource.Kind, client ListWatchClient, namespace string, labelFilters []string, addrs ...string) *CustomCacheInformer {
	c := NewMemcachedStore(kind, MemcachedStoreConfig{
		Addrs:     addrs,
		TrackKeys: true,
	})
	return NewCustomCacheInformer(c, ListerWatcher(client, kind, namespace, labelFilters...), kind.ZeroValue())
}

func NewCustomCacheInformer(store cache.Store, lw cache.ListerWatcher, exampleObject resource.Object) *CustomCacheInformer {
	return &CustomCacheInformer{
		store:         store,
		listerWatcher: lw,
		objectType:    exampleObject,
		processor:     newInformerProcessor(),
	}
}

func (c *CustomCacheInformer) AddEventHandler(handler ResourceWatcher) error {
	c.processor.addListener(newInformerProcessorListener(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cast, ok := obj.(resource.Object)
				if !ok {
					// Hmm
				}
				handler.Add(context.TODO(), cast)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				ocast, ok := oldObj.(resource.Object)
				if !ok {
					// Hmm
				}
				ncast, ok := newObj.(resource.Object)
				handler.Update(context.TODO(), ocast, ncast)
			},
			DeleteFunc: func(obj interface{}) {
				cast, ok := obj.(resource.Object)
				if !ok {
					// Hmm
				}
				handler.Delete(context.TODO(), cast)
			},
		}, processorBufferSize))
	return nil
}

func (c *CustomCacheInformer) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if c.HasStarted() {
		return fmt.Errorf("informer is already started")
	}

	func() {
		c.startedLock.Lock()
		defer c.startedLock.Unlock()

		c.controller = newInformer(c.listerWatcher, c.objectType, c.CacheResyncInterval, c, c.store, nil)
		c.started = true
	}()

	// Separate stop channel because Processor should be stopped strictly after controller
	processorStopCh := make(chan struct{})
	var wg wait.Group
	defer wg.Wait()              // Wait for Processor to stop
	defer close(processorStopCh) // Tell Processor to stop
	//wg.StartWithChannel(processorStopCh, c.cacheMutationDetector.Run)
	wg.StartWithChannel(processorStopCh, c.processor.run)

	defer func() {
		c.startedLock.Lock()
		defer c.startedLock.Unlock()
		c.started = false
	}()
	c.controller.Run(stopCh)
	return nil
}

func (c *CustomCacheInformer) HasStarted() bool {
	c.startedLock.Lock()
	defer c.startedLock.Unlock()
	return c.started
}

func (c *CustomCacheInformer) HasSynced() bool {
	c.startedLock.Lock()
	defer c.startedLock.Unlock()

	if c.controller == nil {
		return false
	}
	return c.controller.HasSynced()
}

func (c *CustomCacheInformer) LastSyncResourceVersion() string {
	c.startedLock.Lock()
	defer c.startedLock.Unlock()

	if c.controller == nil {
		return ""
	}
	return c.controller.LastSyncResourceVersion()
}

func (c *CustomCacheInformer) OnAdd(obj interface{}, isInInitialList bool) {
	c.processor.distribute(informerEventAdd{
		obj:             obj,
		isInInitialList: isInInitialList,
	})
}

func (c *CustomCacheInformer) OnUpdate(oldObj interface{}, newObj interface{}) {
	c.processor.distribute(informerEventUpdate{
		obj: newObj,
		old: oldObj,
	})
}

func (c *CustomCacheInformer) OnDelete(obj interface{}) {
	c.processor.distribute(informerEventDelete{
		obj: obj,
	})
}

// Multiplexes updates in the form of a list of Deltas into a Store, and informs
// a given handler of events OnUpdate, OnAdd, OnDelete
func processDeltas(
	// Object which receives event notifications from the given deltas
	handler cache.ResourceEventHandler,
	clientState cache.Store,
	deltas cache.Deltas,
	isInInitialList bool,
) error {
	// from oldest to newest
	for _, d := range deltas {
		obj := d.Object
		switch d.Type {
		case cache.Sync, cache.Replaced, cache.Added, cache.Updated:
			// TODO: it would be nice to treat cache.Sync events differently here,
			// so we could tell the difference between a cache sync (period re-emission of all items in the cache)
			// from an update sourced from the API server watch request.
			if old, exists, err := clientState.Get(obj); err == nil && exists {
				if err := clientState.Update(obj); err != nil {
					return err
				}
				handler.OnUpdate(old, obj)
			} else {
				if err := clientState.Add(obj); err != nil {
					return err
				}
				handler.OnAdd(obj, isInInitialList)
			}
		case cache.Deleted:
			if err := clientState.Delete(obj); err != nil {
				return err
			}
			handler.OnDelete(obj)
		}
	}
	return nil
}

// newInformer returns a controller for populating the store while also
// providing event notifications.
//
// Parameters
//   - lw is list and watch functions for the source of the resource you want to
//     be informed of.
//   - objType is an object of the type that you expect to receive.
//   - resyncPeriod: if non-zero, will re-list this often (you will get OnUpdate
//     calls, even if nothing changed). Otherwise, re-list will be delayed as
//     long as possible (until the upstream source closes the watch or times out,
//     or you stop the controller).
//   - h is the object you want notifications sent to.
//   - clientState is the store you want to populate
func newInformer(
	lw cache.ListerWatcher,
	objType runtime.Object,
	resyncPeriod time.Duration,
	h cache.ResourceEventHandler,
	clientState cache.Store,
	transformer cache.TransformFunc,
) cache.Controller {
	// This will hold incoming changes. Note how we pass clientState in as a
	// KeyLister, that way resync operations will result in the correct set
	// of update/delete deltas.
	fifo := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{
		KnownObjects:          clientState,
		EmitDeltaTypeReplaced: true,
		Transformer:           transformer,
	})

	cfg := &cache.Config{
		Queue:            fifo,
		ListerWatcher:    lw,
		ObjectType:       objType,
		FullResyncPeriod: resyncPeriod,
		RetryOnError:     false,

		Process: func(obj interface{}, isInInitialList bool) error {
			if deltas, ok := obj.(cache.Deltas); ok {
				return processDeltas(h, clientState, deltas, isInInitialList)
			}
			return errors.New("object given as Process argument is not Deltas")
		},
	}
	return cache.New(cfg)
}

func ListerWatcher(client ListWatchClient, sch resource.Schema, namespace string, labelFilters ...string) cache.ListerWatcher {
	return &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			ctx, span := GetTracer().Start(context.Background(), "informer-list")
			defer span.End()
			span.SetAttributes(
				attribute.String("kind.name", sch.Kind()),
				attribute.String("kind.group", sch.Group()),
				attribute.String("kind.version", sch.Version()),
				attribute.String("namespace", namespace),
			)
			resp := resource.UntypedList{}
			err := client.ListInto(ctx, namespace, resource.ListOptions{
				LabelFilters:    labelFilters,
				Continue:        options.Continue,
				Limit:           int(options.Limit),
				ResourceVersion: options.ResourceVersion,
			}, &resp)
			if err != nil {
				return nil, err
			}
			return &resp, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			ctx, span := GetTracer().Start(context.Background(), "informer-watch")
			defer span.End()
			span.SetAttributes(
				attribute.String("kind.name", sch.Kind()),
				attribute.String("kind.group", sch.Group()),
				attribute.String("kind.version", sch.Version()),
				attribute.String("namespace", namespace),
			)
			opts := resource.WatchOptions{
				ResourceVersion:      options.ResourceVersion,
				ResourceVersionMatch: string(options.ResourceVersionMatch),
				LabelFilters:         labelFilters,
			}
			// TODO: can't defer the cancel call for the context, because it should only be canceled if the
			// _caller_ of WatchFunc finishes with the WatchResponse before the timeout elapses...
			// Seems to be a limitation of the kubernetes implementation here
			/* if options.TimeoutSeconds != nil {
				timeout := time.Duration(*options.TimeoutSeconds) * time.Second
				ctx, cancel = context.WithTimeout(ctx, timeout)
			}*/
			watchResp, err := client.Watch(ctx, namespace, opts)
			if err != nil {
				return nil, err
			}
			if cast, ok := watchResp.(KubernetesCompatibleWatch); ok {
				return cast.KubernetesWatch(), nil
			}
			// If we can't extract a pure watch.Interface from the watch response, we have to make one
			w := &watchWrapper{
				watch: watchResp,
				ch:    make(chan watch.Event),
			}
			go w.start()
			return w, nil
		},
	}
}
