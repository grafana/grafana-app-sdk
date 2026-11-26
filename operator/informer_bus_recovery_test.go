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
		initialList := make(chan struct{})
		var publish func(context.Context, resourcebus.Event)
		var reconnect func()
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, fn func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = fn
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{ready: func(context.Context) error { <-ready; return nil }}, nil
		})
		lists := 0
		lister := listerFunc(func(ctx context.Context) ([]resource.Object, string, error) {
			lists++
			if lists == 1 {
				select {
				case <-initialList:
				case <-ctx.Done():
					return nil, "", ctx.Err()
				}
			}
			obj := listerObj("item")
			obj.SetResourceVersion(strconv.Itoa(lists * 100))
			return []resource.Object{obj}, obj.GetResourceVersion(), nil
		})
		b, err := NewBusInformerWithRecovery(listerTestKind, subscriber, lister, BusInformerOptions{}, BusRecoveryOptions{ResyncInterval: time.Hour})
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
		require.Error(t, b.HealthCheck(ctx))
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbDeleted, Namespace: listerTestNamespace, Name: "item", ResourceVersion: "50"})
		close(ready)
		synctest.Wait()
		require.Equal(t, 1, lists)
		require.Error(t, b.HealthCheck(ctx), "subscription readiness alone does not make recovery healthy")
		close(initialList)
		require.NoError(t, b.WaitForSync(ctx))
		require.NoError(t, b.HealthCheck(ctx))
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
		require.Error(t, b.HealthCheck(ctx))
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
		b, err := NewBusInformerWithRecovery(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls++
			if calls == 1 {
				<-unblock
			}
			return nil, "5000", nil
		}), BusInformerOptions{}, BusRecoveryOptions{ResyncInterval: time.Hour})
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
		informer, err := NewBusInformerWithRecovery(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "100", nil
		}), BusInformerOptions{ErrorHandler: func(context.Context, error) {}}, BusRecoveryOptions{
			AllowDegradedStart: true,
			ResyncInterval:     time.Hour,
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

func TestBusRecoveryRejectsOpaqueListAndKeepsNumericWatermark(t *testing.T) {
	subscriber := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	version := "100"
	informer, err := NewBusInformerWithRecovery(listerTestKind, subscriber, listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return nil, version, nil
	}), BusInformerOptions{}, BusRecoveryOptions{})
	require.NoError(t, err)
	require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(context.Context, resource.Object) error {
		t.Error("a stale add resurrected a missing key")
		return nil
	}}))
	require.NoError(t, informer.relist(t.Context(), busRelistScheduled))
	version = "opaque"
	require.ErrorContains(t, informer.relist(t.Context(), busRelistScheduled), "snapshot resource version")
	informer.applyLive(t.Context(), resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item", ResourceVersion: "90"})
}

func TestBusRecoveryArbitratesStaleSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name          string
		live          resourcebus.Verb
		listRV        string
		listItemRV    string
		wantError     string
		wantNewerItem bool
	}{
		{name: "live write omitted", live: resourcebus.VerbModified, listRV: "150"},
		{name: "live write with older listed item", live: resourcebus.VerbModified, listRV: "150", listItemRV: "120"},
		{name: "live delete with older listed item", live: resourcebus.VerbDeleted, listRV: "150", listItemRV: "120"},
		{name: "live write followed by a newer first-page item", live: resourcebus.VerbModified, listRV: "150", listItemRV: "250", wantNewerItem: true},
		{name: "live delete followed by a recreated first-page item", live: resourcebus.VerbDeleted, listRV: "150", listItemRV: "250", wantNewerItem: true},
		{name: "live write omitted from opaque list", live: resourcebus.VerbModified, listRV: "opaque", wantError: "snapshot resource version"},
		{name: "live write with older item in opaque list", live: resourcebus.VerbModified, listRV: "opaque", listItemRV: "120", wantError: "snapshot resource version"},
		{name: "live delete followed by opaque list", live: resourcebus.VerbDeleted, listRV: "opaque", wantError: "snapshot resource version"},
		{name: "live delete followed by opaque item", live: resourcebus.VerbDeleted, listRV: "300", listItemRV: "opaque", wantError: "item resource version"},
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
			informer, err := NewBusInformerWithRecovery(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
				return objects, listRV, nil
			}), BusInformerOptions{}, BusRecoveryOptions{})
			require.NoError(t, err)
			seen := 0
			require.NoError(t, informer.AddEventHandler(&SimpleWatcher{
				AddFunc:    func(context.Context, resource.Object) error { seen++; return nil },
				UpdateFunc: func(context.Context, resource.Object, resource.Object) error { seen++; return nil },
				DeleteFunc: func(context.Context, resource.Object) error { seen++; return nil },
			}))
			require.NoError(t, informer.relist(t.Context(), busRelistScheduled))
			informer.applyLive(t.Context(), resourcebus.Event{Verb: tc.live, Namespace: id.Namespace, Name: id.Name, ResourceVersion: "200"})
			require.Equal(t, 2, seen)

			listRV, objects = tc.listRV, nil
			if tc.listItemRV != "" {
				item := listerObj(id.Name)
				item.SetResourceVersion(tc.listItemRV)
				objects = []resource.Object{item}
			}
			err = informer.relist(t.Context(), busRelistScheduled)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			informer.applyLive(t.Context(), resourcebus.Event{Verb: resourcebus.VerbAdded, Namespace: id.Namespace, Name: id.Name, ResourceVersion: "160"})
			informer.applyLive(t.Context(), resourcebus.Event{Verb: resourcebus.VerbAdded, Namespace: id.Namespace, Name: id.Name, ResourceVersion: "opaque"})
			if tc.wantNewerItem {
				require.Equal(t, 3, seen)
				require.Equal(t, tc.listItemRV, informer.items[id].GetResourceVersion())
				require.Equal(t, int64(150), informer.listVersion, "an item version is not the collection version")
				return
			}
			require.Equal(t, 2, seen)
			if tc.live == resourcebus.VerbDeleted {
				require.NotContains(t, informer.items, id)
				recreated := listerObj(id.Name)
				recreated.SetResourceVersion("250")
				listRV, objects = "300", []resource.Object{recreated}
				require.NoError(t, informer.relist(t.Context(), busRelistScheduled))
				require.Equal(t, 3, seen)
				require.Equal(t, "250", informer.items[id].GetResourceVersion())
			} else {
				require.Equal(t, "200", informer.items[id].GetResourceVersion())
			}
		})
	}
}

func TestBusRecoveryBuildUsesKindWatchNeeds(t *testing.T) {
	subscriber := busSubscribeFunc(func(context.Context, schema.GroupVersionResource, string, func(context.Context, resourcebus.Event), ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
		return &testBusSubscription{}, nil
	})
	group := resourcebus.Group{Name: "workers"}
	var keyListOptions ClientListerOptions
	recovery := NewBusRecovery(subscriber, group, BusRecoveryDefaults{
		PageSize:       10,
		ResyncInterval: time.Hour,
		KeyListerFactory: func(_ resource.Kind, opts ClientListerOptions) (ResourceKeyLister, error) {
			keyListOptions = opts
			return nil, ErrProjectionUnsupported
		},
	})
	supply := NewSelectingInformerSupplier(nil,
		NewPeriodicList(PeriodicListOptions{}),
		NewBusNotifications(subscriber, group),
		recovery,
	)
	clients := keyListerClientGenerator{Client: keyListerFallbackClient{}}
	live := &WatchLiveOptions{Detail: WatchDetailIdentity, Distribution: WatchDistributionShared}
	resync := &WatchResyncOptions{Detail: WatchDetailIdentity, Interval: time.Minute, ListPageSize: 3}
	liveOnly, err := supply(listerTestKind, clients, InformerOptions{WatchNeeds: WatchOptions{Live: live}})
	require.NoError(t, err)
	require.IsType(t, &busInformer{}, liveOnly)
	resyncOnly, err := supply(listerTestKind, clients, InformerOptions{WatchNeeds: WatchOptions{Resync: resync}})
	require.NoError(t, err)
	require.IsType(t, &PeriodicInformer{}, resyncOnly)

	for _, tc := range []struct {
		configured uint64
		want       uint64
	}{{0, 10}, {1, 1}, {3, 3}} {
		informer, err := supply(listerTestKind, clients, InformerOptions{
			WatchNeeds:           WatchOptions{Live: live, Resync: resync},
			ListWatchOptions:     ListWatchOptions{Namespace: listerTestNamespace},
			MaxConcurrentWorkers: tc.configured,
		})
		require.NoError(t, err)
		require.IsType(t, &BusInformerWithRecovery{}, informer)
		bus := informer.(*BusInformerWithRecovery)
		require.Equal(t, tc.want, bus.opts.MaxConcurrentWorkers)
		require.Equal(t, time.Minute, bus.opts.ResyncInterval)
		require.Equal(t, group, bus.group)
		require.Equal(t, listerTestNamespace, bus.namespace)
		require.Equal(t, ClientListerOptions{
			ListWatchOptions: ListWatchOptions{Namespace: listerTestNamespace},
			PageSize:         3,
		}, keyListOptions)
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
		informer, err := NewBusInformerWithRecovery(listerTestKind, sub, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if lists.Add(1) == 2 {
				time.Sleep(2 * overflowRelistInterval)
			}
			return nil, "100", nil
		}), BusInformerOptions{}, BusRecoveryOptions{ResyncInterval: time.Hour})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))

		informer.recordOverflow()
		synctest.Wait()
		require.Equal(t, int64(2), lists.Load())
		informer.recordOverflow()
		time.Sleep(2 * overflowRelistInterval)
		synctest.Wait()
		require.Equal(t, int64(2), lists.Load(), "cooldown starts after the slow LIST")
		time.Sleep(overflowRelistInterval)
		synctest.Wait()
		require.Equal(t, int64(3), lists.Load(), "recover the second drop before the normal resync")
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoveryReconnectsDoNotPostponeScheduledResync(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var reconnect func()
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, _ func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{}, nil
		})
		version := "100"
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			obj := listerObj("item")
			obj.SetResourceVersion(version)
			return []resource.Object{obj}, version, nil
		})
		informer, err := NewBusInformerWithRecovery(listerTestKind, subscriber, lister, BusInformerOptions{}, BusRecoveryOptions{ResyncInterval: time.Minute})
		require.NoError(t, err)
		updates := make(chan string, 16)
		require.NoError(t, informer.AddEventHandler(&SimpleWatcher{
			UpdateFunc: func(_ context.Context, old, current resource.Object) error {
				updates <- old.GetResourceVersion() + "->" + current.GetResourceVersion()
				return nil
			},
		}))
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		synctest.Wait()
		version = "110"
		for range 11 {
			time.Sleep(5 * time.Second)
			reconnect()
			synctest.Wait()
		}
		informer.recordOverflow()
		synctest.Wait()
		require.Len(t, updates, 1, "reconnects and overflow only deliver changes")
		require.Equal(t, "100->110", <-updates)
		// The initial scheduled interval, including jitter, has now passed.
		time.Sleep(12 * time.Second)
		synctest.Wait()
		require.Len(t, updates, 1, "scheduled resync still delivers unchanged objects")
		require.Equal(t, "110->110", <-updates)
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoveryWorkersDeliverSameKeyInOrderWithFreshTimeouts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		var reconnect func()
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, handler func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = handler
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{}, nil
		})
		var items []resource.Object
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			return items, "100", nil
		})
		const timeout = time.Second
		informer, err := NewBusInformerWithRecovery(listerTestKind, subscriber, lister, BusInformerOptions{EventTimeout: timeout}, BusRecoveryOptions{
			ResyncInterval: time.Hour, MaxConcurrentWorkers: 2,
		})
		require.NoError(t, err)
		type delivery struct {
			action    string
			err       error
			remaining time.Duration
		}
		received := make(chan delivery, 3)
		record := func(ctx context.Context, action string) {
			deadline, _ := ctx.Deadline()
			received <- delivery{action, ctx.Err(), time.Until(deadline)}
		}
		release := make(chan struct{})
		require.NoError(t, informer.AddEventHandler(&SimpleWatcher{
			AddFunc: func(eventCtx context.Context, obj resource.Object) error {
				record(eventCtx, "add:"+obj.GetResourceVersion())
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			},
			UpdateFunc: func(eventCtx context.Context, old, current resource.Object) error {
				record(eventCtx, "update:"+old.GetResourceVersion()+"->"+current.GetResourceVersion())
				return nil
			},
			DeleteFunc: func(eventCtx context.Context, obj resource.Object) error {
				record(eventCtx, "delete:"+obj.GetResourceVersion())
				return nil
			},
		}))
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		synctest.Wait()
		item := listerObj("item")
		item.SetResourceVersion("100")
		items = []resource.Object{item}
		reconnect()
		require.Equal(t, delivery{"add:100", nil, timeout}, <-received)

		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbModified, Namespace: item.GetNamespace(), Name: item.GetName(), ResourceVersion: "110"})
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbDeleted, Namespace: item.GetNamespace(), Name: item.GetName(), ResourceVersion: "120"})
		synctest.Wait()
		time.Sleep(2 * timeout)
		close(release)
		require.Equal(t, delivery{"update:100->110", nil, timeout}, <-received)
		require.Equal(t, delivery{"delete:110", nil, timeout}, <-received)
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoveryCancelsBlockedWorkerEnqueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var publish func(context.Context, resourcebus.Event)
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, handler func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = handler
			return &testBusSubscription{}, nil
		})
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			return nil, "100", nil
		})
		informer, err := NewBusInformerWithRecovery(listerTestKind, subscriber, lister, BusInformerOptions{}, BusRecoveryOptions{
			ResyncInterval: time.Hour, MaxConcurrentWorkers: 2,
		})
		require.NoError(t, err)
		blocked := make(chan struct{})
		require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(ctx context.Context, _ resource.Object) error {
			close(blocked)
			<-ctx.Done()
			return nil
		}}))
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item", ResourceVersion: "110"})
		<-blocked
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbModified, Name: "item", ResourceVersion: "120"})
		publish(ctx, resourcebus.Event{Verb: resourcebus.VerbDeleted, Name: "item", ResourceVersion: "130"})
		synctest.Wait()
		queued := 0
		for _, queue := range informer.deliveries {
			queued += len(queue)
		}
		require.Equal(t, 1, queued, "update is queued behind the blocked add")
		require.Empty(t, informer.events, "delete has left the notification queue and is blocked on the worker queue")
		cancel()
		require.NoError(t, <-done)
	})
}

func TestBusRecoverySnapshotReportsInstalledIdentities(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		item := listerObj("item")
		item.SetResourceVersion("10")
		id := resource.Identifier{Namespace: item.GetNamespace(), Name: item.GetName()}
		var reconnect func()
		sub := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, _ func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			reconnect = resourcebus.NewSubscribeConfig(opts...).OnReconnect
			return &testBusSubscription{}, nil
		})
		calls := 0
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls++
			if calls == 1 {
				return []resource.Object{item}, "10", nil
			}
			return nil, "20", nil
		})
		informer, err := NewBusInformerWithRecovery(listerTestKind, sub, lister, BusInformerOptions{}, BusRecoveryOptions{ResyncInterval: time.Hour})
		require.NoError(t, err)
		require.Empty(t, informer.Snapshot())

		seen := make(chan []resource.Identifier, 2)
		require.NoError(t, informer.AddEventHandler(&SimpleWatcher{
			AddFunc: func(context.Context, resource.Object) error {
				seen <- informer.Snapshot()
				return nil
			},
			DeleteFunc: func(context.Context, resource.Object) error {
				seen <- informer.Snapshot()
				return nil
			},
		}))
		done := make(chan error, 1)
		go func() { done <- informer.Run(ctx) }()
		require.NoError(t, informer.WaitForSync(ctx))
		keys := <-seen
		require.Equal(t, []resource.Identifier{id}, keys)
		keys[0].Name = "changed by caller"
		require.Equal(t, []resource.Identifier{id}, informer.Snapshot())

		reconnect()
		require.Empty(t, <-seen)
		require.Empty(t, informer.Snapshot())
		cancel()
		require.NoError(t, <-done)
	})
}
