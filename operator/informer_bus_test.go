package operator

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

var notificationKind = resource.Kind{Schema: resource.NewSimpleSchema(
	"example.grafana.app", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
	resource.WithKind("Widget"), resource.WithPlural("widgets"),
)}

type busSubscribeFunc func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error)

func (f busSubscribeFunc) Subscribe(ctx context.Context, gvr schema.GroupVersionResource, namespace string, handler func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
	return f(ctx, gvr, namespace, handler, opts...)
}

type testBusSubscription struct {
	ready  func(context.Context) error
	closed bool
}

func (s *testBusSubscription) WaitReady(ctx context.Context) error {
	if s.ready != nil {
		return s.ready(ctx)
	}
	return nil
}
func (s *testBusSubscription) Close() error {
	s.closed = true
	return nil
}

func TestNotificationMechanismDeliversWithoutClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		ready := make(chan struct{})
		sub := &testBusSubscription{ready: func(ctx context.Context) error {
			select {
			case <-ready:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		bus := busSubscribeFunc(func(_ context.Context, gvr schema.GroupVersionResource, namespace string, handler func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			require.Equal(t, notificationKind.GroupVersionResource(), gvr)
			require.Equal(t, "default", namespace)
			require.Equal(t, "workers", resourcebus.NewSubscribeConfig(opts...).Group.Name)
			publish = handler
			return sub, nil
		})
		needs := WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity, Distribution: WatchDistributionShared}}
		inf, err := BuildInformer(InformerRequest{
			Kind:     notificationKind,
			Supplier: NewSelectingInformerSupplier(nil, NewNotificationMechanism(bus, resourcebus.Group{Name: "workers"})),
			Options:  InformerOptions{WatchNeeds: needs, ListWatchOptions: ListWatchOptions{Namespace: "default"}},
		})
		require.NoError(t, err)
		var calls []string
		require.NoError(t, inf.AddEventHandler(&SimpleWatcher{
			AddFunc: func(_ context.Context, obj resource.Object) error {
				calls = append(calls, "add:"+obj.GetName())
				return nil
			},
			DeleteFunc: func(_ context.Context, obj resource.Object) error {
				calls = append(calls, "delete:"+obj.GetName())
				return nil
			},
		}))
		done := make(chan error, 1)
		go func() { done <- inf.Run(ctx) }()
		synctest.Wait()
		publish(ctx, resourcebus.Event{Namespace: "default", Name: "one", Verb: resourcebus.VerbModified})
		require.Empty(t, calls)
		close(ready)
		require.NoError(t, inf.WaitForSync(ctx))
		publish(ctx, resourcebus.Event{Namespace: "default", Name: "one", Verb: resourcebus.VerbDeleted})
		publish(ctx, resourcebus.Event{Namespace: "unrelated", Name: "two", Verb: resourcebus.VerbAdded})
		synctest.Wait()
		require.Equal(t, []string{"add:one", "delete:one"}, calls)
		cancel()
		require.NoError(t, <-done)
		require.True(t, sub.closed)
	})
}

func TestNotificationMechanismFallbackAndSelectionErrors(t *testing.T) {
	fallbackCalled := false
	fallback := func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
		fallbackCalled = true
		return nil, errors.New("fallback reached")
	}
	needs := WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}}
	supplier := NewSelectingInformerSupplier(fallback, NewNotificationMechanism(nil, resourcebus.Group{}))
	_, err := supplier(notificationKind, nil, InformerOptions{WatchNeeds: needs})
	require.ErrorContains(t, err, "fallback reached")
	require.True(t, fallbackCalled)

	fallbackCalled = false
	bus := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return nil, resourcebus.ErrSubscriptionRejected
	})
	mechanism := NewNotificationMechanism(bus, resourcebus.Group{})
	supplier = NewSelectingInformerSupplier(fallback, mechanism)
	filtered := InformerOptions{WatchNeeds: needs, ListWatchOptions: ListWatchOptions{LabelFilters: []string{"a=b"}}}
	_, err = supplier(notificationKind, nil, filtered)
	require.ErrorContains(t, err, "fallback reached")
	require.True(t, fallbackCalled)
	_, err = mechanism.NewInformer(notificationKind, nil, filtered)
	require.ErrorContains(t, err, "selectors")
	inf, err := supplier(notificationKind, nil, InformerOptions{WatchNeeds: needs})
	require.NoError(t, err)
	require.ErrorIs(t, inf.Run(t.Context()), resourcebus.ErrSubscriptionRejected)
}
