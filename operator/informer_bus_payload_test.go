package operator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

type hydrationTestClient struct {
	resource.Client
	list listerFunc
	get  func(context.Context, resource.Identifier) (resource.Object, error)
}

func (c hydrationTestClient) Get(ctx context.Context, id resource.Identifier) (resource.Object, error) {
	return c.get(ctx, id)
}

func (c hydrationTestClient) ListInto(ctx context.Context, _ string, _ resource.ListOptions, into resource.ListObject) error {
	items, version, err := c.list(ctx)
	if err != nil {
		return err
	}
	into.SetItems(items)
	into.SetResourceVersion(version)
	return nil
}

func newHydrationTestInformer(t *testing.T, sub resourcebus.Subscriber, opts InformerOptions, client hydrationTestClient) *BusInformerWithRecovery {
	t.Helper()
	mechanism := NewBusRecovery(sub, resourcebus.Group{}, BusRecoveryDefaults{
		Detail: WatchDetailFullObject, ResyncInterval: time.Hour,
	})
	inf, err := mechanism.NewInformer(listerTestKind, keyListerClientGenerator{Client: client}, opts)
	require.NoError(t, err)
	return inf.(*BusInformerWithRecovery)
}

func TestHydrationRecognizesRecreatedObject(t *testing.T) {
	existing := listerObj("item")
	existing.SetGroupVersionKind(schema.GroupVersionKind{Group: listerTestKind.Group()})
	existing.SetUID("old")
	existing.SetResourceVersion("10")
	existing.Spec = map[string]any{"value": "old"}
	recreated := listerObj("item")
	recreated.SetGroupVersionKind(schema.GroupVersionKind{})
	recreated.SetUID("new")
	recreated.SetResourceVersion("13")
	recreated.Spec = map[string]any{"value": "new"}
	sub := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	b := newHydrationTestInformer(t, sub, InformerOptions{}, hydrationTestClient{
		list: func(context.Context) ([]resource.Object, string, error) {
			return []resource.Object{existing}, existing.GetResourceVersion(), nil
		},
		get: func(_ context.Context, id resource.Identifier) (resource.Object, error) {
			assert.Equal(t, existing.GetStaticMetadata().Identifier(), id)
			return recreated, nil
		},
	})
	require.Equal(t, WatchDetailFullObject, b.WatchOptions().Live.Detail)
	require.Equal(t, WatchDetailFullObject, b.WatchOptions().Resync.Detail)
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
	for _, delivered := range objects {
		require.Equal(t, listerTestKind.GroupVersionKind(), delivered.GroupVersionKind())
	}
	require.Empty(t, existing.GroupVersionKind().Kind)
	require.Empty(t, recreated.GroupVersionKind())
	require.Equal(t, existing.GetSpec(), objects[0].GetSpec())
	require.Equal(t, existing.GetUID(), objects[1].GetUID())
	require.Equal(t, existing.GetSpec(), objects[1].GetSpec())
	require.Equal(t, recreated.GetUID(), objects[2].GetUID())
	require.Equal(t, recreated.GetSpec(), objects[2].GetSpec())
}

func TestHydrationTimeoutAllowsRecoveryToProceed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured time.Duration
		deadline   time.Duration
	}{
		{name: "configured timeout", configured: time.Second, deadline: time.Second},
		{name: "default timeout", deadline: 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var publish func(context.Context, resourcebus.Event)
				sub := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
					publish = fn
					return &testBusSubscription{}, nil
				})
				var lists atomic.Int64
				item := listerObj("stalled")
				item.SetResourceVersion("101")
				item.Spec = map[string]any{"value": "recovered"}
				errorsSeen := make(chan error, 1)
				informer := newHydrationTestInformer(t, sub, InformerOptions{
					EventTimeout: tc.configured,
					ErrorHandler: func(_ context.Context, err error) { errorsSeen <- err },
				}, hydrationTestClient{
					list: func(context.Context) ([]resource.Object, string, error) {
						if lists.Add(1) == 1 {
							return nil, "100", nil
						}
						return []resource.Object{item}, "101", nil
					},
					get: func(ctx context.Context, _ resource.Identifier) (resource.Object, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
				})
				deliveries := make(chan resource.Object, 1)
				require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(ctx context.Context, obj resource.Object) error {
					if DeliveryFromContext(ctx) == DeliveryRelist {
						deliveries <- obj
					}
					return nil
				}}))
				done := make(chan error, 1)
				go func() { done <- informer.Run(ctx) }()
				require.NoError(t, informer.WaitForSync(ctx))
				publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Namespace: item.GetNamespace(), Name: item.GetName(), ResourceVersion: "101"})
				time.Sleep(tc.deadline - time.Millisecond)
				synctest.Wait()
				require.EqualValues(t, 1, lists.Load())
				require.Empty(t, errorsSeen)
				time.Sleep(time.Millisecond)
				synctest.Wait()
				require.Len(t, errorsSeen, 1)
				require.ErrorIs(t, <-errorsSeen, context.DeadlineExceeded)
				require.EqualValues(t, 2, lists.Load())
				require.Len(t, deliveries, 1)
				require.Equal(t, item.GetSpec(), (<-deliveries).GetSpec())
				cancel()
				require.NoError(t, <-done)
			})
		})
	}
}

func TestHydrationFailuresShareRecoveryCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		sub := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = fn
			return &testBusSubscription{}, nil
		})
		var lists, failures atomic.Int64
		informer := newHydrationTestInformer(t, sub, InformerOptions{
			ErrorHandler: func(context.Context, error) { failures.Add(1) },
		}, hydrationTestClient{
			list: func(context.Context) ([]resource.Object, string, error) {
				lists.Add(1)
				return nil, "100", nil
			},
			get: func(context.Context, resource.Identifier) (resource.Object, error) {
				return nil, errors.New("transient GET failure")
			},
		})
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		event := resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item", ResourceVersion: "101"}
		publish(ctx, event)
		synctest.Wait()
		require.EqualValues(t, 2, lists.Load())
		for range 3 {
			publish(ctx, event)
		}
		synctest.Wait()
		require.EqualValues(t, 4, failures.Load())
		require.EqualValues(t, 2, lists.Load())
		time.Sleep(overflowRelistInterval)
		synctest.Wait()
		require.EqualValues(t, 3, lists.Load())
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoveryDeliveryDetail(t *testing.T) {
	subscriber := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return nil, "0", nil
	})

	t.Run("identity by default", func(t *testing.T) {
		b, err := NewBusInformerWithRecovery(listerTestKind, subscriber, lister, BusInformerOptions{}, BusRecoveryOptions{})
		require.NoError(t, err)
		require.Equal(t, WatchDetailIdentity, b.WatchOptions().Live.Detail)
		require.Equal(t, WatchDetailIdentity, b.WatchOptions().Resync.Detail)
	})
	t.Run("unsupported metadata", func(t *testing.T) {
		mechanism := NewBusRecovery(subscriber, resourcebus.Group{}, BusRecoveryDefaults{Detail: WatchDetailMetadata})
		_, err := mechanism.NewInformer(listerTestKind, nil, InformerOptions{})
		require.ErrorContains(t, err, "unsupported bus recovery detail")
	})
}
