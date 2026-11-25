package operator

import (
	"context"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

func TestBusRecoveryAfterSubscriptionGap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ready := make(chan struct{})
		var publish func(context.Context, resourcebus.Event)
		var reconnect func()
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = fn
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{ready: func(context.Context) error { <-ready; return nil }}, nil
		})
		lists := 0
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists++
			obj := listerObj("item")
			obj.SetResourceVersion(strconv.Itoa(lists * 100))
			return []resource.Object{obj}, obj.GetResourceVersion(), nil
		})
		b, err := NewBusInformer(listerTestKind, subscriber, lister, BusInformerOptions{ResyncInterval: time.Hour})
		require.NoError(t, err)
		var deliveries []string
		require.NoError(t, b.AddEventHandler(&SimpleWatcher{
			AddFunc: func(_ context.Context, obj resource.Object) error {
				deliveries = append(deliveries, "add:"+obj.GetResourceVersion())
				return nil
			},
			UpdateFunc: func(_ context.Context, old, obj resource.Object) error {
				deliveries = append(deliveries, old.GetResourceVersion()+"->"+obj.GetResourceVersion())
				return nil
			},
			DeleteFunc: func(_ context.Context, obj resource.Object) error {
				deliveries = append(deliveries, "delete:"+obj.GetResourceVersion())
				return nil
			},
		}))
		done := make(chan error, 1)
		go func() { done <- b.Run(ctx) }()
		synctest.Wait()
		require.Zero(t, lists)
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbDeleted, Namespace: listerTestNamespace, Name: "item", ResourceVersion: "50"})
		close(ready)
		require.NoError(t, b.WaitForSync(ctx))
		synctest.Wait()
		require.Equal(t, []string{"add:100"}, deliveries)
		// The next snapshot catches a change delivered to a different group member.
		reconnect()
		synctest.Wait()
		require.Equal(t, []string{"add:100", "100->200"}, deliveries)
		for _, e := range []resourcebus.Event{
			{Verb: resourcebus.VerbDeleted, ResourceVersion: "250"},
			{Verb: resourcebus.VerbAdded, ResourceVersion: "220"},
			{Verb: resourcebus.VerbAdded, ResourceVersion: "260"},
			{Verb: resourcebus.VerbDeleted, ResourceVersion: "255"},
		} {
			e.Namespace, e.Name = listerTestNamespace, "item"
			publish(ctx, e)
		}
		synctest.Wait()
		require.Equal(t, []string{"add:100", "100->200", "delete:200", "add:260"}, deliveries)
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoverySchedulesRelistOnOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = fn
			return &testBusSubscription{}, nil
		})
		calls := 0
		unblock := make(chan struct{})
		b, err := NewBusInformer(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls++
			if calls == 1 {
				<-unblock
			}
			return nil, "5000", nil
		}), BusInformerOptions{ResyncInterval: time.Hour})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- b.Run(ctx) }()
		synctest.Wait()
		for i := range busQueueSize + 10 {
			publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item", ResourceVersion: strconv.Itoa(i + 1)})
		}
		close(unblock)
		require.NoError(t, b.WaitForSync(ctx))
		synctest.Wait()
		require.Equal(t, 2, calls)
		cancel()
		require.NoError(t, <-done)
	})
}
