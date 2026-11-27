package operator

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

func TestDeltaSourceInitialListAndTombstone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var onReconnect func()
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, _ func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			onReconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{}, nil
		})
		calls := 0
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls++
			if calls == 1 {
				item := listerObj("item")
				item.SetResourceVersion("100")
				return []resource.Object{item}, "100", nil
			}
			return nil, "200", nil
		})
		kind := resource.Kind{Schema: resource.NewSimpleSchema("example.grafana.app", "v1",
			resource.NewWrappedObject(&metav1.PartialObjectMetadata{}), &resource.UntypedList{},
			resource.WithKind("Widget"), resource.WithPlural("widgets"))}
		informer, err := NewBusInformerWithRecovery(kind, subscriber, lister, BusInformerOptions{}, BusRecoveryOptions{
			MaxConcurrentWorkers: 3,
			ResyncInterval:       time.Hour,
		})
		require.NoError(t, err)
		source := NewDeltaSource(informer)
		initial := make(chan bool, 1)
		added := make(chan any, 1)
		deleted := make(chan any, 1)
		reg, err := source.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
			AddFunc:    func(obj any, initialList bool) { added <- obj; initial <- initialList },
			DeleteFunc: func(obj any) { deleted <- obj },
		})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- source.RunWithContext(ctx) }()
		<-reg.HasSyncedChecker().Done()
		require.True(t, <-initial)
		require.IsType(t, &metav1.PartialObjectMetadata{}, <-added)
		onReconnect()
		tombstone, ok := (<-deleted).(cache.DeletedFinalStateUnknown)
		require.True(t, ok)
		require.Equal(t, listerTestNamespace+"/item", tombstone.Key)
		require.IsType(t, &metav1.PartialObjectMetadata{}, tombstone.Obj)
		cancel()
		require.NoError(t, <-done)
	})
}

func TestDeltaSourceSerializesEachRegistration(t *testing.T) {
	informer := &testInformer{}
	source := NewDeltaSource(informer)
	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})
	deleted := make(chan struct{})
	_, err := source.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { close(firstEntered); <-firstRelease },
		DeleteFunc: func(any) { close(deleted) },
	})
	require.NoError(t, err)
	go informer.FireAdd(t.Context(), listerObj("first"))
	<-firstEntered
	attempted := make(chan struct{})
	go func() {
		close(attempted)
		_ = informer.handlers[0].Delete(t.Context(), listerObj("second"))
	}()
	<-attempted
	select {
	case <-deleted:
		t.Error("delete ran while add was still in progress")
	case <-time.After(20 * time.Millisecond):
	}
	close(firstRelease)
	<-deleted
}

func TestDeltaSourceRejectsQueuedSync(t *testing.T) {
	wrapped := InformerWithWatchOptions(&ConcurrentInformer{informer: &mockInformer{}}, WatchOptions{})
	source := NewDeltaSource(wrapped)
	_, err := source.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	require.ErrorContains(t, err, "initial callbacks")
	require.ErrorContains(t, source.RunWithContext(t.Context()), "initial callbacks")
	require.False(t, source.HasSynced())
}

func TestBusRecoveryParallelInitialListWaitsForCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		sub := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			return &testBusSubscription{}, nil
		})
		objects := make([]resource.Object, 2)
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			obj := listerObj(name)
			obj.SetResourceVersion("100")
			obj.SetCreationTimestamp(metav1.NewTime(time.Unix(123, 0)))
			objects[xxhash.Sum64String(obj.GetNamespace()+"/"+name)%2] = obj
		}
		require.NotNil(t, objects[0])
		require.NotNil(t, objects[1])
		b, err := NewBusInformerWithRecovery(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			return objects, "100", nil
		}), BusInformerOptions{}, BusRecoveryOptions{MaxConcurrentWorkers: 2})
		require.NoError(t, err)
		entered := make(chan resource.Object, 2)
		release := make(chan struct{})
		require.NoError(t, b.AddEventHandler(&SimpleWatcher{AddFunc: func(ctx context.Context, obj resource.Object) error {
			entered <- obj
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		}}))
		done := make(chan error, 1)
		go func() { done <- b.Run(ctx) }()
		synctest.Wait()
		require.Len(t, entered, 2)
		for range 2 {
			require.Equal(t, objects[0].GetCreationTimestamp(), (<-entered).GetCreationTimestamp())
		}
		require.Error(t, b.HealthCheck(ctx))
		close(release)
		require.NoError(t, b.WaitForSync(ctx))
		cancel()
		require.NoError(t, <-done)
	})
}
