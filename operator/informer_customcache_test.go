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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func TestCustomCacheInformer_Run(t *testing.T) {
	t.Run("Test stop", func(t *testing.T) {
		inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return nil, fmt.Errorf("I AM ERROR")
			},
		}, &resource.UntypedObject{})
		stopCh := make(chan struct{})
		stopped := false
		go func() {
			inf.Run(stopCh)
			stopped = true
		}()
		close(stopCh)
		time.Sleep(time.Second)
		assert.True(t, stopped, "informer did not stop when stopCh was closed")
	})
}

func TestCustomCacheInformer_Run_DistributeEvents(t *testing.T) {
	events := make(chan watch.Event)
	inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &resource.UntypedList{}, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return &mockWatch{
				events: events,
			}, nil
		},
	}, &resource.UntypedObject{})
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

	stopCh := make(chan struct{})
	go inf.Run(stopCh)

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
	close(stopCh)
}

func TestCustomCacheInformer_Run_ManyEvents(t *testing.T) {
	numEvents := 1000
	events := make(chan watch.Event, numEvents)
	inf := NewCustomCacheInformer(newUnsafeCache(), &mockListWatcher{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &resource.UntypedList{}, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return &mockWatch{
				events: events,
			}, nil
		},
	}, &resource.UntypedObject{})
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
	stopCh := make(chan struct{})
	go inf.Run(stopCh)
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
	close(stopCh)
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
