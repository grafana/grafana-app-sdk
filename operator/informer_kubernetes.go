package operator

import (
	"context"
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/resource"
)

// KubernetesBasedInformer is a k8s apimachinery-based informer. It wraps a k8s cache.SharedIndexInformer,
// and works most optimally with a client that has a Watch response that implements KubernetesCompatibleWatch.
type KubernetesBasedInformer struct {
	ErrorHandler        func(error)
	SharedIndexInformer cache.SharedIndexInformer
	schema              resource.Schema
}

var EmptyLabelFilters []string

// NewKubernetesBasedInformer creates a new KubernetesBasedInformer for the provided schema and namespace,
// using the ListWatchClient provided to do its List and Watch requests.
func NewKubernetesBasedInformer(sch resource.Schema, client ListWatchClient, namespace string) (
	*KubernetesBasedInformer, error) {
	return NewKubernetesBasedInformerWithFilters(sch, client, namespace, EmptyLabelFilters)
}

// NewKubernetesBasedInformerWithFilters creates a new KubernetesBasedInformer for the provided schema and namespace,
// using the ListWatchClient provided to do its List and Watch requests applying provided labelFilters if it is not empty.
func NewKubernetesBasedInformerWithFilters(sch resource.Schema, client ListWatchClient, namespace string, labelFilters []string) (
	*KubernetesBasedInformer, error) {
	if sch == nil {
		return nil, fmt.Errorf("resource cannot be nil")
	}
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	return &KubernetesBasedInformer{
		schema: sch,
		ErrorHandler: func(err error) {
			// Do nothing
		},
		SharedIndexInformer: cache.NewSharedIndexInformer(
			&cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					resp := listObjectWrapper{}
					err := client.ListInto(context.TODO(), namespace, resource.ListOptions{
						LabelFilters: labelFilters,
						Continue:     options.Continue,
						Limit:        int(options.Limit),
					}, &resp)
					if err != nil {
						return nil, err
					}
					return &resp, nil
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					ctx := context.TODO()
					opts := resource.WatchOptions{
						ResourceVersion:      options.ResourceVersion,
						ResourceVersionMatch: string(options.ResourceVersionMatch),
						LabelFilters:         labelFilters,
					}
					// TODO: can't defer the cancel call for the context, because it should only be canceled if the
					// _caller_ of WatchFunc finishes with the WatchResponse before the timeout elapses...
					// Seems to be a limitation of the kubernetes implementation here
					/*if options.TimeoutSeconds != nil {
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
			},
			nil,
			time.Second*30,
			cache.Indexers{
				cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			}),
	}, nil
}

// AddEventHandler adds a ResourceWatcher as an event handler for watch events from the informer.
// Event handlers are not guaranteed to be executed in parallel or in any particular order by the underlying
// kubernetes apimachinery code. If you want to coordinate ResourceWatchers, use am InformerController.
func (k *KubernetesBasedInformer) AddEventHandler(handler ResourceWatcher) error {
	// TODO: AddEventHandler returns the registration handle which should be supplied to RemoveEventHandler
	// but we don't currently call the latter. We should add RemoveEventHandler to the informer API
	// and let controller call it when appropriate.
	_, err := k.SharedIndexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cast, err := k.toResourceObject(obj)
			if err != nil {
				k.ErrorHandler(err)
				return
			}
			err = handler.Add(context.TODO(), cast)
			if err != nil {
				k.ErrorHandler(err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cOld, err := k.toResourceObject(oldObj)
			if err != nil {
				k.ErrorHandler(err)
				return
			}
			cNew, err := k.toResourceObject(newObj)
			if err != nil {
				k.ErrorHandler(err)
				return
			}
			err = handler.Update(context.TODO(), cOld, cNew)
			if err != nil {
				k.ErrorHandler(err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			cast, err := k.toResourceObject(obj)
			if err != nil {
				k.ErrorHandler(err)
				return
			}
			err = handler.Delete(context.TODO(), cast)
			if err != nil {
				k.ErrorHandler(err)
			}
		},
	})

	return err
}

// Run starts the informer and blocks until stopCh receives a message
func (k *KubernetesBasedInformer) Run(stopCh <-chan struct{}) error {
	k.SharedIndexInformer.Run(stopCh)
	return nil
}

// Schema returns the resource.Schema this informer is set up for
func (k *KubernetesBasedInformer) Schema() resource.Schema {
	return k.schema
}

func (k *KubernetesBasedInformer) toResourceObject(obj interface{}) (resource.Object, error) {
	// First, check if it's already a resource.Object
	if cast, ok := obj.(resource.Object); ok {
		return cast, nil
	}

	// Is this an instance of ResourceObjectWrapper? Unwrap it if so
	if cast, ok := obj.(ResourceObjectWrapper); ok {
		return cast.ResourceObject(), nil
	}

	// Next, see if it has an `Into` method for casting to a resource.Object
	if cast, ok := obj.(ConvertableIntoResourceObject); ok {
		newObj := k.schema.ZeroValue()
		err := cast.Into(newObj)
		return newObj, err
	}
	// TODO: other methods...?

	return nil, fmt.Errorf("unable to cast %v into resource.Object", reflect.TypeOf(obj))
}

// ConvertableIntoResourceObject describes any object which can be marshaled into a resource.Object.
// This is specifically useful for objects which may wrap underlying data which can be marshaled into a resource.Object,
// but need the exact implementation provided to them (by `into`).
type ConvertableIntoResourceObject interface {
	Into(object resource.Object) error
}

// ResourceObjectWrapper describes anything which wraps a resource.Object, such that it can be extracted.
type ResourceObjectWrapper interface {
	ResourceObject() resource.Object
}

// KubernetesCompatibleWatch describes a watch response that either is wrapping a kubernetes watch.Interface,
// or can return a compatibility layer that implements watch.Interface.
type KubernetesCompatibleWatch interface {
	KubernetesWatch() watch.Interface
}

// ListWatchClient describes a client which can do ListInto and Watch requests.
type ListWatchClient interface {
	ListInto(ctx context.Context, namespace string, options resource.ListOptions, into resource.ListObject) error
	Watch(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error)
}

type listObjectWrapper struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []runtime.Object
}

func (l *listObjectWrapper) DeepCopyObject() runtime.Object {
	val := reflect.ValueOf(l).Elem()

	cpy := reflect.New(val.Type())
	cpy.Elem().Set(val)

	// Using the <obj>, <ok> for the type conversion ensures that it doesn't panic if it can't be converted
	if obj, ok := cpy.Interface().(runtime.Object); ok {
		return obj
	}

	// TODO: better return than nil?
	return nil
}

func (*listObjectWrapper) ListMetadata() resource.ListMetadata {
	return resource.ListMetadata{}
}

func (l *listObjectWrapper) SetListMetadata(md resource.ListMetadata) {
	l.ListMeta = metav1.ListMeta{
		ResourceVersion: md.ResourceVersion,
		Continue:        md.Continue,
	}
}

func (*listObjectWrapper) ListItems() []resource.Object {
	return nil
}

func (l *listObjectWrapper) SetItems(items []resource.Object) {
	list := make([]runtime.Object, 0)
	for _, i := range items {
		// If the Object already implements runtime.Object, we don't have to wrap it
		if ro, ok := i.(runtime.Object); ok {
			list = append(list, ro)
		} else {
			list = append(list, &objectWrapper{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       i.StaticMetadata().Namespace,
					Name:            i.StaticMetadata().Name,
					ResourceVersion: i.CommonMetadata().ResourceVersion,
				},
				Object: i,
			})
		}
	}
	l.Items = list
}

type objectWrapper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Object            resource.Object
}

func (o *objectWrapper) DeepCopyObject() runtime.Object {
	val := reflect.ValueOf(o).Elem()

	cpy := reflect.New(val.Type())
	cpy.Elem().Set(val)

	// Using the <obj>, <ok> for the type conversion ensures that it doesn't panic if it can't be converted
	if obj, ok := cpy.Interface().(runtime.Object); ok {
		return obj
	}

	// TODO: better return than nil?
	return nil
}

func (o *objectWrapper) ResourceObject() resource.Object {
	return o.Object
}

type watchWrapper struct {
	watch resource.WatchResponse
	ch    chan watch.Event
}

func (w *watchWrapper) start() {
	for e := range w.watch.WatchEvents() {
		w.ch <- watch.Event{
			Type: watch.EventType(e.EventType),
			Object: &objectWrapper{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       e.Object.StaticMetadata().Namespace,
					Name:            e.Object.StaticMetadata().Name,
					ResourceVersion: e.Object.CommonMetadata().ResourceVersion,
				},
				Object: e.Object,
			},
		}
	}
}

func (w *watchWrapper) Stop() {
	w.watch.Stop()
	close(w.ch)
}

func (w *watchWrapper) ResultChan() <-chan watch.Event {
	return w.ch
}
