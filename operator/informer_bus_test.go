package operator

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

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
	close  func() error
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
	if s.close != nil {
		return s.close()
	}
	return nil
}

func TestNotificationMechanismDeliversWithoutClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		var subscribedResource schema.GroupVersionResource
		var subscribedNamespace, subscribedGroup string
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
			subscribedResource = gvr
			subscribedNamespace = namespace
			subscribedGroup = resourcebus.NewSubscribeConfig(opts...).Group.Name
			publish = handler
			return sub, nil
		})
		needs := WatchOptions{Live: &WatchLiveOptions{
			Detail: WatchDetailIdentity, Distribution: WatchDistributionShared, NotificationOnly: true,
		}}
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
		require.Equal(t, notificationKind.GroupVersionResource(), subscribedResource)
		require.Equal(t, "default", subscribedNamespace)
		require.Equal(t, "workers", subscribedGroup)
		require.NotNil(t, publish)
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

func TestNotificationMechanismRequiresOptIn(t *testing.T) {
	bus := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	mechanism := NewNotificationMechanism(bus, resourcebus.Group{})
	options := InformerOptions{WatchNeeds: WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}}}
	t.Run("selection", func(t *testing.T) {
		fallbackErr := errors.New("API watch selected")
		supplier := NewSelectingInformerSupplier(func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return nil, fallbackErr
		}, mechanism)
		_, err := BuildInformer(InformerRequest{Kind: notificationKind, Supplier: supplier, Options: options})
		require.ErrorIs(t, err, fallbackErr)
	})
	t.Run("direct construction", func(t *testing.T) {
		_, err := mechanism.NewInformer(notificationKind, nil, options)
		require.ErrorContains(t, err, "notification-only")
	})
}

func TestNotificationInformerHealthChecks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ready := make(chan struct{})
		bus := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			return &testBusSubscription{ready: func(ctx context.Context) error {
				select {
				case <-ready:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}}, nil
		})
		inf, err := newBusInformer(notificationKind, bus, resourcebus.Group{}, InformerOptions{})
		require.NoError(t, err)
		controller := NewInformerController(InformerControllerConfig{})
		require.NoError(t, controller.AddInformer(inf, notificationKind.Kind()))
		checks := controller.HealthChecks()
		require.Len(t, checks, 1)
		require.Error(t, checks[0].HealthCheck(ctx))
		done := make(chan error, 1)
		go func() { done <- inf.Run(ctx) }()
		synctest.Wait()
		require.Error(t, checks[0].HealthCheck(ctx))

		close(ready)
		require.NoError(t, inf.WaitForSync(ctx))
		require.NoError(t, checks[0].HealthCheck(ctx))

		cancel()
		require.NoError(t, <-done)
		require.Error(t, checks[0].HealthCheck(t.Context()))
	})
}

func TestNotificationInformerCancelsFailedSubscriptionBeforeRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var failedCtx, liveCtx context.Context
		var failedPublish, livePublish func(context.Context, resourcebus.Event)
		var canceledOnClose bool
		failed := &testBusSubscription{
			ready: func(context.Context) error { return errors.New("subscription unavailable") },
			close: func() error {
				canceledOnClose = failedCtx.Err() == context.Canceled
				return errors.New("close failed")
			},
		}
		live := &testBusSubscription{}
		bus := busSubscribeFunc(func(ctx context.Context, _ schema.GroupVersionResource, _ string, handler func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			if failedCtx == nil {
				failedCtx, failedPublish = ctx, handler
				return failed, nil
			}
			liveCtx, livePublish = ctx, handler
			return live, nil
		})
		inf, err := newBusInformer(notificationKind, bus, resourcebus.Group{}, InformerOptions{
			ErrorHandler: func(context.Context, error) {},
		})
		require.NoError(t, err)
		var received []string
		require.NoError(t, inf.AddEventHandler(&SimpleWatcher{AddFunc: func(_ context.Context, obj resource.Object) error {
			received = append(received, obj.GetName())
			return nil
		}}))
		done := make(chan error, 1)
		go func() { done <- inf.Run(ctx) }()
		require.NoError(t, inf.WaitForSync(ctx))
		require.True(t, canceledOnClose, "failed subscription must be canceled before Close")
		require.NoError(t, liveCtx.Err())

		failedPublish(ctx, resourcebus.Event{Name: "failed", Verb: resourcebus.VerbAdded})
		livePublish(ctx, resourcebus.Event{Name: "live", Verb: resourcebus.VerbAdded})
		synctest.Wait()
		require.Equal(t, []string{"live"}, received)

		cancel()
		require.NoError(t, <-done)
		require.ErrorIs(t, liveCtx.Err(), context.Canceled)
		require.True(t, live.closed)
	})
}

func TestConcurrentNotificationInformerStartsTimeoutInWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		bus := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, handler func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = handler
			return &testBusSubscription{}, nil
		})
		const timeout = time.Second
		opts := InformerOptions{EventTimeout: timeout, MaxConcurrentWorkers: 1}
		base, err := newBusInformer(notificationKind, bus, resourcebus.Group{}, opts)
		require.NoError(t, err)
		inf, err := NewConcurrentInformerFromOptions(base, opts)
		require.NoError(t, err)
		type handlerAttempt struct {
			ctx       context.Context
			err       error
			remaining time.Duration
		}
		attempts := make(chan handlerAttempt, 2)
		releaseFirst := make(chan struct{})
		require.NoError(t, inf.AddEventHandler(&SimpleWatcher{AddFunc: func(eventCtx context.Context, obj resource.Object) error {
			deadline, _ := eventCtx.Deadline()
			attempts <- handlerAttempt{eventCtx, eventCtx.Err(), time.Until(deadline)}
			if obj.GetName() == "first" {
				select {
				case <-releaseFirst:
				case <-ctx.Done():
				}
			}
			return nil
		}}))
		done := make(chan error, 1)
		go func() { done <- inf.Run(ctx) }()
		require.NoError(t, inf.WaitForSync(ctx))

		publish(ctx, resourcebus.Event{Name: "first", Verb: resourcebus.VerbAdded})
		first := <-attempts
		synctest.Wait()
		require.NoError(t, first.ctx.Err())
		require.Equal(t, timeout, first.remaining)

		publish(ctx, resourcebus.Event{Name: "second", Verb: resourcebus.VerbAdded})
		synctest.Wait()
		time.Sleep(2 * timeout)
		require.ErrorIs(t, first.ctx.Err(), context.DeadlineExceeded)
		close(releaseFirst)
		second := <-attempts
		require.NoError(t, second.err)
		require.Equal(t, timeout, second.remaining)

		cancel()
		require.NoError(t, <-done)
	})
}

func TestNotificationMechanismFallbackAndSelectionErrors(t *testing.T) {
	fallbackCalled := false
	fallback := func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
		fallbackCalled = true
		return nil, errors.New("fallback reached")
	}
	needs := WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity, NotificationOnly: true}}
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
