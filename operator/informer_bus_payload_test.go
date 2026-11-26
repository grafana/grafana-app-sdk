package operator

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

type busGetFunc func(context.Context, resource.Identifier) (resource.Object, error)

func (f busGetFunc) Get(ctx context.Context, id resource.Identifier) (resource.Object, error) {
	return f(ctx, id)
}

func TestHydrationRecognizesRecreatedObject(t *testing.T) {
	existing := listerObj("item")
	existing.SetUID("old")
	existing.SetResourceVersion("10")
	existing.Spec = map[string]any{"value": "old"}
	recreated := listerObj("item")
	recreated.SetUID("new")
	recreated.SetResourceVersion("13")
	recreated.Spec = map[string]any{"value": "new"}
	sub := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return []resource.Object{existing}, existing.GetResourceVersion(), nil
	})
	b, err := NewBusInformerWithRecovery(listerTestKind, sub, lister, BusInformerOptions{}, BusRecoveryOptions{
		Payload: PayloadHydrated,
		Getter: busGetFunc(func(context.Context, resource.Identifier) (resource.Object, error) {
			return recreated, nil
		}),
	})
	require.NoError(t, err)
	var kinds []string
	var objects []resource.Object
	require.NoError(t, b.AddEventHandler(&SimpleWatcher{
		AddFunc: func(_ context.Context, obj resource.Object) error {
			kinds = append(kinds, "add")
			objects = append(objects, obj)
			return nil
		},
		DeleteFunc: func(_ context.Context, obj resource.Object) error {
			kinds = append(kinds, "delete")
			objects = append(objects, obj)
			return nil
		},
	}))
	require.NoError(t, b.relist(t.Context(), busRelistScheduled))
	// A delayed update for the old object reads the already-recreated object.
	for _, event := range []resourcebus.Event{
		{Verb: resourcebus.VerbModified, ResourceVersion: "11"},
		{Verb: resourcebus.VerbDeleted, ResourceVersion: "12"},
		{Verb: resourcebus.VerbAdded, ResourceVersion: "13"},
	} {
		event.Name, event.Namespace = existing.GetName(), existing.GetNamespace()
		b.applyLive(t.Context(), event)
	}
	require.Equal(t, []string{"add", "delete", "add"}, kinds)
	require.Equal(t, existing.GetSpec(), objects[0].GetSpec())
	require.Equal(t, existing.GetUID(), objects[1].GetUID())
	require.Equal(t, existing.GetSpec(), objects[1].GetSpec())
	require.Equal(t, recreated.GetUID(), objects[2].GetUID())
	require.Equal(t, recreated.GetSpec(), objects[2].GetSpec())
}

func TestHydrationTimeoutAllowsRecoveryToProceed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		var reconnect func()
		sub := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = fn
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{}, nil
		})
		var lists atomic.Int64
		errorsSeen := make(chan error, 1)
		informer, err := NewBusInformerWithRecovery(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "100", nil
		}), BusInformerOptions{
			EventTimeout: time.Second,
			ErrorHandler: func(_ context.Context, err error) { errorsSeen <- err },
		}, BusRecoveryOptions{
			Payload: PayloadHydrated, ResyncInterval: time.Hour,
			Getter: busGetFunc(func(ctx context.Context, _ resource.Identifier) (resource.Object, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}),
		})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "stalled", ResourceVersion: "101"})
		synctest.Wait()
		reconnect()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(2), lists.Load())
		require.ErrorIs(t, <-errorsSeen, context.DeadlineExceeded)
		cancel()
		require.NoError(t, <-done)
	})
}
