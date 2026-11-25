package operator

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
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
			// Another replica received the delete, then this replica received the recreation.
			{Verb: resourcebus.VerbAdded, ResourceVersion: "280"},
		} {
			e.Namespace, e.Name = listerTestNamespace, "item"
			publish(ctx, e)
		}
		synctest.Wait()
		require.Equal(t, []string{"add:100", "100->200", "delete:200", "add:260", "add:280"}, deliveries)
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

func TestBusRecoveryDegradedStartAndReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var attempts atomic.Int64
		recovered := &testBusSubscription{}
		subscriber := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("bus unavailable")
			}
			return recovered, nil
		})
		var lists atomic.Int64
		informer, err := NewBusInformer(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "", nil
		}), BusInformerOptions{
			AllowDegradedStart: true,
			ResyncInterval:     time.Hour,
			ErrorHandler:       func(context.Context, error) {},
		})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		synctest.Wait()
		require.Equal(t, int64(1), lists.Load())
		time.Sleep(busRetryInterval * 2)
		synctest.Wait()
		require.Equal(t, int64(2), attempts.Load())
		require.Equal(t, int64(2), lists.Load())
		cancel()
		require.NoError(t, <-done)
		require.True(t, recovered.closed)
	})
}

func TestBusRecoveryKeepsNumericWatermarkAfterOpaqueList(t *testing.T) {
	subscriber := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	version := "100"
	informer, err := NewBusInformer(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return nil, version, nil
	}), BusInformerOptions{})
	require.NoError(t, err)
	require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(context.Context, resource.Object) error {
		t.Error("a stale add resurrected a missing key")
		return nil
	}}))
	require.NoError(t, informer.relist(t.Context()))
	version = "opaque"
	require.NoError(t, informer.relist(t.Context()))
	informer.applyLive(t.Context(), resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item", ResourceVersion: "90"})
}

func TestBusRecoveryArbitratesStaleSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name       string
		live       resourcebus.Verb
		listRV     string
		listItemRV string
	}{
		{name: "live write omitted", live: resourcebus.VerbModified, listRV: "150"},
		{name: "live write with older listed item", live: resourcebus.VerbModified, listRV: "150", listItemRV: "120"},
		{name: "live delete with older listed item", live: resourcebus.VerbDeleted, listRV: "150", listItemRV: "120"},
		{name: "live delete followed by opaque list", live: resourcebus.VerbDeleted, listRV: "opaque"},
		{name: "live delete with older item in opaque list", live: resourcebus.VerbDeleted, listRV: "opaque", listItemRV: "120"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
				return &testBusSubscription{}, nil
			})
			id := resource.Identifier{Namespace: listerTestNamespace, Name: "item"}
			listRV := "100"
			initial := listerObj(id.Name)
			initial.SetResourceVersion("100")
			objects := []resource.Object{initial}
			informer, err := NewBusInformer(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
				return objects, listRV, nil
			}), BusInformerOptions{})
			require.NoError(t, err)
			seen := 0
			require.NoError(t, informer.AddEventHandler(&SimpleWatcher{
				AddFunc:    func(context.Context, resource.Object) error { seen++; return nil },
				UpdateFunc: func(context.Context, resource.Object, resource.Object) error { seen++; return nil },
				DeleteFunc: func(context.Context, resource.Object) error { seen++; return nil },
			}))
			require.NoError(t, informer.relist(t.Context()))
			informer.applyLive(t.Context(), resourcebus.Event{Verb: tc.live, Namespace: id.Namespace, Name: id.Name, ResourceVersion: "200"})
			require.Equal(t, 2, seen)

			listRV, objects = tc.listRV, nil
			if tc.listItemRV != "" {
				item := listerObj(id.Name)
				item.SetResourceVersion(tc.listItemRV)
				objects = []resource.Object{item}
			}
			require.NoError(t, informer.relist(t.Context()))
			informer.applyLive(t.Context(), resourcebus.Event{Verb: resourcebus.VerbAdded, Namespace: id.Namespace, Name: id.Name, ResourceVersion: "160"})
			require.Equal(t, 2, seen)
			if tc.live == resourcebus.VerbDeleted {
				require.NotContains(t, informer.recovery.items, id)
			} else {
				require.Equal(t, "200", informer.recovery.items[id].GetResourceVersion())
			}
		})
	}
}

func TestBusRecoveryRetainsOverflowDuringCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		sub := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			return &testBusSubscription{}, nil
		})
		var lists atomic.Int64
		informer, err := NewBusInformer(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "100", nil
		}), BusInformerOptions{ResyncInterval: time.Hour})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))

		informer.recovery.overflow()
		synctest.Wait()
		require.Equal(t, int64(2), lists.Load())
		time.Sleep(time.Second)
		informer.recovery.overflow()
		synctest.Wait()
		require.Equal(t, int64(2), lists.Load(), "respect cooldown")
		time.Sleep(30 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(3), lists.Load(), "recover the second drop before the normal resync")
		cancel()
		require.NoError(t, <-done)
	})
}
