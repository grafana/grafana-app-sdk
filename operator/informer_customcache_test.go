package operator

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var (
	untypedKind = resource.Kind{
		Schema: resource.NewSimpleSchema("foo", "bar", &resource.UntypedObject{}, &resource.UntypedList{}, resource.WithKind("test")),
	}
)

func TestCustomCacheInformer_Run(t *testing.T) {
	t.Run("Test stop", func(t *testing.T) {
		inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return nil, fmt.Errorf("I AM ERROR")
			},
		}, untypedKind, CustomCacheInformerOptions{})
		ctx, cancel := context.WithCancel(context.Background())
		stopped := false
		go func() {
			inf.Run(ctx)
			stopped = true
		}()
		cancel()
		time.Sleep(time.Second)
		assert.True(t, stopped, "informer did not stop when stopCh was closed")
	})
}

func TestCustomCacheInformer_Run_DistributeEvents(t *testing.T) {
	events := make(chan watch.Event)
	defer close(events)
	inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &resource.UntypedList{}, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return &mockWatch{
				events: events,
			}, nil
		},
	}, untypedKind, CustomCacheInformerOptions{})
	addObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            "foo",
			ResourceVersion: "1",
		},
	}
	updateObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            "foo",
			ResourceVersion: "2",
		},
	}
	numHandlers := 100
	wg := sync.WaitGroup{}
	for i := 0; i < numHandlers; i++ {
		inf.AddEventHandler(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				assert.Equal(t, addObj, object)
				wg.Done()
				return nil
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				assert.Equal(t, addObj, object)
				assert.Equal(t, updateObj, object2)
				wg.Done()
				return nil
			},
			DeleteFunc: func(ctx context.Context, object resource.Object) error {
				assert.Equal(t, addObj, object)
				wg.Done()
				return nil
			},
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inf.Run(ctx)

	// Add
	wg.Add(numHandlers)
	events <- watch.Event{
		Type:   watch.Added,
		Object: addObj,
	}
	assert.True(t, waitOrTimeout(&wg, time.Second*10), "event was not distributed to all handlers within 10 seconds")
	// Update
	wg = sync.WaitGroup{} // reset WG
	wg.Add(numHandlers)
	events <- watch.Event{
		Type:   watch.Modified,
		Object: updateObj,
	}
	assert.True(t, waitOrTimeout(&wg, time.Second*10), "event was not distributed to all handlers within 10 seconds")
	// Delete
	wg = sync.WaitGroup{} // reset WG
	wg.Add(numHandlers)
	events <- watch.Event{
		Type:   watch.Deleted,
		Object: addObj,
	}
	assert.True(t, waitOrTimeout(&wg, time.Second*10), "event was not distributed to all handlers within 10 seconds")
}

func TestCustomCacheInformer_Run_ManyEvents(t *testing.T) {
	numEvents := 1000
	events := make(chan watch.Event, numEvents)
	defer close(events)
	inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &resource.UntypedList{}, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return &mockWatch{
				events: events,
			}, nil
		},
	}, untypedKind, CustomCacheInformerOptions{})
	numHandlers := 100
	addWG := sync.WaitGroup{}
	updateWG := sync.WaitGroup{}
	deleteWG := sync.WaitGroup{}
	for i := 0; i < numHandlers; i++ {
		inf.AddEventHandler(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addWG.Done()
				return nil
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateWG.Done()
				return nil
			},
			DeleteFunc: func(ctx context.Context, object resource.Object) error {
				deleteWG.Done()
				return nil
			},
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inf.Run(ctx)
	for i := 0; i < numEvents; i++ {
		etype := watch.Added
		switch i % 3 {
		case 0:
			addWG.Add(numHandlers)
		case 1:
			etype = watch.Modified
			updateWG.Add(numHandlers)
		case 2:
			etype = watch.Deleted
			deleteWG.Add(numHandlers)
		}
		events <- watch.Event{
			Type: etype,
			Object: &resource.UntypedObject{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       "default",
					Name:            fmt.Sprintf("object-%d", int(i/3)),
					ResourceVersion: strconv.Itoa(i),
				},
			},
		}
	}
	assert.True(t, waitOrTimeout(&addWG, time.Second*10), "all add events were not distributed within 10 seconds")
	assert.True(t, waitOrTimeout(&updateWG, time.Second*10), "all add events were not distributed within 10 seconds")
	assert.True(t, waitOrTimeout(&deleteWG, time.Second*10), "all add events were not distributed within 10 seconds")
}

func TestCustomCacheInformer_Run_CacheState(t *testing.T) {
	events := make(chan watch.Event)
	defer close(events)
	store := newUnsafeCache()
	inf := NewCustomCacheInformer(store, &mockListWatcher{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &resource.UntypedList{}, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return &mockWatch{
				events: events,
			}, nil
		},
	}, untypedKind, CustomCacheInformerOptions{})
	wg := sync.WaitGroup{}
	inf.AddEventHandler(&SimpleWatcher{
		AddFunc: func(ctx context.Context, object resource.Object) error {
			wg.Done()
			return nil
		},
		UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			wg.Done()
			return nil
		},
		DeleteFunc: func(ctx context.Context, object resource.Object) error {
			wg.Done()
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	go inf.Run(ctx)
	defer cancel()

	obj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            "foo",
			ResourceVersion: "1",
		},
	}
	wg.Add(1)
	events <- watch.Event{
		Type:   watch.Added,
		Object: obj,
	}
	require.True(t, waitOrTimeout(&wg, time.Second), "timed out waiting for event")
	key, _ := store.keyFunc(obj)
	assert.Equal(t, obj, store.items[key])
	updated := obj.Copy()
	updated.SetResourceVersion("2")
	wg.Add(1)
	events <- watch.Event{
		Type:   watch.Modified,
		Object: updated,
	}
	require.True(t, waitOrTimeout(&wg, time.Second), "timed out waiting for event")
	assert.Equal(t, updated, store.items[key])
	wg.Add(1)
	events <- watch.Event{
		Type:   watch.Deleted,
		Object: updated,
	}
	require.True(t, waitOrTimeout(&wg, time.Second), "timed out waiting for event")
	_, ok := store.items[key]
	assert.False(t, ok)
}

func waitOrTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

type mockListWatcher struct {
	ListFunc  func(options metav1.ListOptions) (runtime.Object, error)
	WatchFunc func(options metav1.ListOptions) (watch.Interface, error)
}

func (lw *mockListWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	if lw.ListFunc != nil {
		return lw.ListFunc(options)
	}
	return nil, nil
}

func (lw *mockListWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	if lw.WatchFunc != nil {
		return lw.WatchFunc(options)
	}
	return nil, nil
}

type mockWatch struct {
	events chan watch.Event
}

func (w *mockWatch) ResultChan() <-chan watch.Event {
	return w.events
}

func (*mockWatch) Stop() {}

func newUnsafeCache() *unsafeCache {
	return &unsafeCache{
		keyFunc: cache.DeletionHandlingMetaNamespaceKeyFunc,
		items:   make(map[string]any),
	}
}

type unsafeCache struct {
	items   map[string]any
	keyFunc func(any) (string, error)
}

func (u *unsafeCache) Add(obj any) error {
	key, err := u.keyFunc(obj)
	if err != nil {
		return err
	}
	u.items[key] = obj
	return nil
}

func (u *unsafeCache) Update(obj any) error {
	key, err := u.keyFunc(obj)
	if err != nil {
		return err
	}
	u.items[key] = obj
	return nil
}

func (u *unsafeCache) Delete(obj any) error {
	key, err := u.keyFunc(obj)
	if err != nil {
		return err
	}
	delete(u.items, key)
	return nil
}

func (u *unsafeCache) List() []any {
	return nil
}

func (u *unsafeCache) ListKeys() []string {
	return nil
}

func (u *unsafeCache) Get(obj any) (item any, exists bool, err error) {
	key, err := u.keyFunc(obj)
	if err != nil {
		return nil, false, err
	}
	item, ok := u.items[key]
	return item, ok, nil
}

func (u *unsafeCache) GetByKey(key string) (item any, exists bool, err error) {
	item, ok := u.items[key]
	return item, ok, nil
}

func (u *unsafeCache) Replace([]any, string) error {
	return nil
}

func (u *unsafeCache) Resync() error {
	return nil
}
