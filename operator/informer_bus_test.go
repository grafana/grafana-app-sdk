package operator

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

var busTestKind = resource.Kind{
	Schema: resource.NewSimpleSchema("example.grafana.app", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind("Widget"), resource.WithPlural("widgets")),
}

// recordingWatcher captures every dispatch a BusInformer makes.
type recordingWatcher struct {
	mu            sync.Mutex
	adds          []resource.Object
	updates       []resource.Object
	updateSources []resource.Object
	deletes       []resource.Object
	addErr        error
}

func (w *recordingWatcher) Add(_ context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.adds = append(w.adds, obj)
	return w.addErr
}

func (w *recordingWatcher) Update(_ context.Context, src, tgt resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.updateSources = append(w.updateSources, src)
	w.updates = append(w.updates, tgt)
	return nil
}

func (w *recordingWatcher) Delete(_ context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deletes = append(w.deletes, obj)
	return nil
}

// The slices must be read under the lock: passing w.adds as an argument would read the slice header before the
// callee could acquire it, racing a concurrent append.
func (w *recordingWatcher) addedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return busNames(w.adds)
}

func (w *recordingWatcher) updatedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return busNames(w.updates)
}

func (w *recordingWatcher) deletedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return busNames(w.deletes)
}

func (w *recordingWatcher) added() []resource.Object {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]resource.Object(nil), w.adds...)
}

var _ ResourceWatcher = (*recordingWatcher)(nil)

func busEvent(verb resourcebus.Verb, name, rv string) resourcebus.Event {
	return resourcebus.Event{Verb: verb, Namespace: busTestNamespace, Name: name, ResourceVersion: rv}
}

func busTestGVR() schema.GroupVersionResource { return busTestKind.GroupVersionResource() }

// startBusInformer runs an informer inside the current synctest bubble and lets the run loop quiesce, so it has
// subscribed and finished its initial list. The returned stop func must be deferred: the bubble requires every
// goroutine it spawned to have exited before the test body returns.
func startBusInformer(t *testing.T, inf *BusInformer) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = inf.Run(ctx)
	}()
	// Wait blocks until the run goroutine is durably blocked, i.e. parked in the main select after subscribing and
	// the initial list. None of that needs the fake clock to advance.
	synctest.Wait()
	require.True(t, inf.HasSynced(), "informer never synced")
	return func() {
		cancel()
		<-done
		synctest.Wait()
	}
}

func newBusInformer(t *testing.T, sub resourcebus.Subscriber, lister ResourceLister, watcher ResourceWatcher, opts BusInformerOptions) *BusInformer {
	t.Helper()
	if opts.Namespace == "" {
		opts.Namespace = busTestNamespace
	}
	inf, err := NewBusInformer(busTestKind, sub, lister, opts)
	require.NoError(t, err)
	if watcher != nil {
		require.NoError(t, inf.AddEventHandler(watcher))
	}
	return inf
}

func TestNewBusInformer_Validation(t *testing.T) {
	sub := resourcebus.NewFake()
	lister := staticLister()

	_, err := NewBusInformer(resource.Kind{}, sub, lister, BusInformerOptions{})
	assert.Error(t, err, "an empty kind is rejected")

	_, err = NewBusInformer(busTestKind, nil, lister, BusInformerOptions{})
	assert.Error(t, err, "a nil subscriber is rejected")

	_, err = NewBusInformer(busTestKind, sub, nil, BusInformerOptions{})
	assert.Error(t, err, "a nil lister is rejected")

	_, err = NewBusInformer(busTestKind, sub, lister, BusInformerOptions{Payload: "nonsense"})
	assert.Error(t, err, "an unknown payload is rejected")

	_, err = NewBusInformer(busTestKind, sub, lister, BusInformerOptions{Payload: PayloadHydrated})
	assert.Error(t, err, "the hydrated payload requires a getter")

	inf, err := NewBusInformer(busTestKind, sub, lister, BusInformerOptions{})
	require.NoError(t, err)
	assert.Equal(t, WatchDetailIdentity, inf.WatchOptions().Detail, "the identity payload is the default")
	assert.Equal(t, busTestKind.Schema, inf.Schema())
	assert.Error(t, inf.AddEventHandler(nil))
}

// The initial list drives an add per existing object and marks the informer synced, so a consumer can start
// reconciling the full set.
func TestBusInformer_InitialListDeliversAdds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		watcher := &recordingWatcher{}
		inf := newBusInformer(t, sub, staticLister(busObj("a"), busObj("b")), watcher, BusInformerOptions{})

		stop := startBusInformer(t, inf)
		defer stop()

		assert.True(t, sub.Subscribed(busTestGVR(), busTestNamespace))
		assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames())
	})
}

// The group reaches the bus whole, delivery included: a subscriber which cannot provide the requested sharing has
// to be able to see what was asked for in order to refuse it.
func TestBusInformer_SubscribesWithTheConfiguredGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		group := resourcebus.Group{Name: "widgets-operator", Delivery: resourcebus.GroupDeliveryRoundRobin}
		inf := newBusInformer(t, sub, staticLister(), &recordingWatcher{}, BusInformerOptions{Group: group})

		stop := startBusInformer(t, inf)
		defer stop()

		assert.Equal(t, group, sub.Group(busTestGVR(), busTestNamespace))
	})
}

// With no group every replica receives every event, so nothing is passed to the bus.
func TestBusInformer_SubscribesWithoutAGroupByDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		inf := newBusInformer(t, sub, staticLister(), &recordingWatcher{}, BusInformerOptions{})

		stop := startBusInformer(t, inf)
		defer stop()

		assert.Equal(t, resourcebus.Group{}, sub.Group(busTestGVR(), busTestNamespace))
	})
}

// A live added goes through Add, a modified of a known object through Update, and a deleted through Delete. In
// identity mode the delivered object carries the event's identity and nothing else.
func TestBusInformer_LiveEventsDispatchIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		watcher := &recordingWatcher{}
		inf := newBusInformer(t, sub, staticLister(busObj("changed")), watcher, BusInformerOptions{})
		stop := startBusInformer(t, inf)
		defer stop()

		ctx := context.Background()
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbAdded, "fresh", "11")))
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbModified, "changed", "12")))
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbDeleted, "gone", "13")))
		synctest.Wait()

		assert.Equal(t, []string{"changed", "fresh"}, watcher.addedNames(), "the initial list, then the live add")
		assert.Equal(t, []string{"changed"}, watcher.updatedNames())
		assert.Equal(t, []string{"gone"}, watcher.deletedNames())

		delivered := watcher.added()[1]
		assert.Equal(t, busTestNamespace, delivered.GetNamespace())
		assert.Equal(t, "11", delivered.GetResourceVersion())
		assert.Equal(t, busTestKind.GroupVersionKind(), delivered.GroupVersionKind())
	})
}

// A modification of an object the informer has never seen has no previous state to pair it with, so it is delivered
// as an add — which is what the re-list diff calls a first-seen key too. Dispatching it as an update with the same
// object on both sides would read as a resync to the InformerController and be dropped by an opinionated watcher.
func TestBusInformer_FirstSeenModificationIsAnAdd(t *testing.T) {
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{})

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "unseen", "11"))

	assert.Equal(t, []string{"unseen"}, watcher.addedNames())
	assert.Empty(t, watcher.updatedNames())
	assert.Equal(t, []string{"unseen"}, busNames(inf.store.List()), "and it is written through, so the next re-list does not re-report it")
}

// An update carries the previous and current state, never the same object twice: the InformerController reads equal
// resource versions as a resync, and OpinionatedWatcher drops an update whose generations match.
func TestBusInformer_UpdatesCarryPreviousAndCurrent(t *testing.T) {
	watcher := &recordingWatcher{}
	first, second := busObjRV("x", 100), busObjRV("x", 200)
	getter := &fakeGetter{objects: map[string]resource.Object{busStoreKey(busTestNamespace, "x"): first}}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
		Payload: PayloadHydrated,
		Getter:  getter,
	})

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "100"))
	getter.objects[busStoreKey(busTestNamespace, "x")] = second
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "x", "200"))

	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	require.Len(t, watcher.updates, 1)
	assert.Equal(t, "100", watcher.updateSources[0].GetResourceVersion(), "the previous state is the one the store held")
	assert.Equal(t, "200", watcher.updates[0].GetResourceVersion())
}

// A write the store refuses — a newer delete landed while the payload was being read — must not be dispatched: the
// snapshot does not hold the object, so no later re-list can diff away a spurious add.
func TestBusInformer_DoesNotDispatchAWriteTheStoreRefuses(t *testing.T) {
	watcher := &recordingWatcher{}
	var handled []error
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})

	// The delete at 300 tombstones the key; the add at 200 predates it.
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "300"))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

	assert.Empty(t, watcher.addedNames(), "the superseded add is not delivered")
	assert.Equal(t, []string{"x"}, watcher.deletedNames(), "the delete is the last thing watchers saw")
	assert.Empty(t, inf.store.List())
	require.Len(t, handled, 1)
	assert.Contains(t, handled[0].Error(), "superseded by a newer write")
}

// The subscriber callback must not block, so a full queue drops the event and forces a re-list rather than waiting
// on the bus's delivery path.
func TestBusInformer_FullQueueDropsAndForcesARelist(t *testing.T) {
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), &recordingWatcher{}, BusInformerOptions{
		EventQueueSize: 1,
	})

	// Nothing is draining the queue: the first event fills it, the second is dropped.
	inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "queued", "1"))
	inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "dropped", "2"))

	select {
	case <-inf.relistSignal:
	default:
		t.Fatal("a dropped event must force a re-list to recover it")
	}
}

// A live delete drops the object from the snapshot, and a live add writes through to it, so the next re-list diffs
// against what has actually been delivered.
func TestBusInformer_LiveEventsWriteThroughToSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		inf := newBusInformer(t, sub, staticLister(busObj("keep"), busObj("gone")), &recordingWatcher{}, BusInformerOptions{})
		stop := startBusInformer(t, inf)
		defer stop()

		ctx := context.Background()
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbDeleted, "gone", "13")))
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbAdded, "fresh", "14")))
		synctest.Wait()

		assert.ElementsMatch(t, []string{"keep", "fresh"}, busNames(inf.store.List()))
	})
}

// In hydrated mode the snapshot holds whole objects, so a live delete delivers the last-known one rather than a
// bare identity.
func TestBusInformer_HydratedLiveDeleteDeliversLastKnownObject(t *testing.T) {
	watcher := &recordingWatcher{}
	listed := busObjRV("gone", 100)
	listed.Spec = map[string]any{"field": "value"}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(listed), watcher, BusInformerOptions{
		Payload: PayloadHydrated,
		Getter:  &fakeGetter{objects: map[string]resource.Object{}},
	})
	require.NoError(t, inf.relist(context.Background(), initialRelist))

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "gone", "200"))

	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	require.Len(t, watcher.deletes, 1)
	assert.Equal(t, map[string]any{"field": "value"}, watcher.deletes[0].GetSpec())
}

// In identity mode the snapshot retains identities only. Keeping the listed bodies would make steady-state memory
// scale with total payload size, which is the cost this informer exists to avoid — and the default lister returns
// full objects, as does an identity lister falling back.
func TestBusInformer_IdentityModeRetainsNoObjectBodies(t *testing.T) {
	listed := busObjRV("x", 100)
	listed.Spec = map[string]any{"field": "value"}
	listed.Labels = map[string]string{"app": "test"}
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(listed), watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))

	stored, ok := inf.store.Get(busTestNamespace, "x")
	require.True(t, ok)
	assert.Empty(t, stored.GetSpec(), "no spec is retained")
	assert.Equal(t, "100", stored.GetResourceVersion(), "but the version the arbitration needs is kept")
	assert.Equal(t, busTestNamespace, stored.GetNamespace())
	assert.Equal(t, map[string]string{"app": "test"}, stored.GetLabels(),
		"metadata is kept, because live events carry it too")

	// An identity modification keeps that identity rather than reintroducing a body.
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "x", "200"))
	stored, ok = inf.store.Get(busTestNamespace, "x")
	require.True(t, ok)
	assert.Empty(t, stored.GetSpec())
}

// An unknown verb is reported to the error handler and skipped; a valid event after it still arrives.
func TestBusInformer_SkipsUnknownVerb(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		watcher := &recordingWatcher{}
		var handled []string
		inf := newBusInformer(t, sub, staticLister(), watcher, BusInformerOptions{
			ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err.Error()) },
		})
		stop := startBusInformer(t, inf)
		defer stop()

		ctx := context.Background()
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent("nonsense", "x", "1")))
		require.NoError(t, sub.Publish(ctx, busTestGVR(), busTestNamespace, busEvent(resourcebus.VerbModified, "survivor", "2")))
		synctest.Wait()

		assert.Equal(t, []string{"survivor"}, watcher.addedNames(), "first seen, so delivered as an add")
		assert.Len(t, handled, 1)
		assert.Contains(t, handled[0], "unknown verb")
	})
}

// An error from a watcher reaches the error handler without stopping delivery to the rest.
func TestBusInformer_WatcherErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	first := &recordingWatcher{addErr: errors.New("boom")}
	second := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObj("a")), first, BusInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, inf.AddEventHandler(second))

	require.NoError(t, inf.relist(context.Background(), initialRelist))

	assert.Equal(t, []string{"a"}, second.addedNames(), "the second watcher still receives the event")
	require.Len(t, handled, 1)
	assert.EqualError(t, handled[0], "boom")
}

// A re-list diffs against the previous snapshot: an object which has vanished is delivered as a delete carrying its
// last-known state, which is how a change the live stream never delivered is caught.
func TestBusInformer_RelistDiffEmitsDeletes(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObj("a"), busObj("b")}, "", nil
		}
		return []resource.Object{busObj("a")}, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames())
	assert.Equal(t, []string{"b"}, watcher.deletedNames(), "a vanished object must be delivered as a delete")
}

// An object first seen on a re-list rather than the initial list is delivered as an add, not an update, so an
// add-only watcher wakes for it.
func TestBusInformer_RelistEmitsAddForNewKeys(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObj("a")}, "", nil
		}
		return []resource.Object{busObj("a"), busObj("b")}, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames(), "b must be an add, not an update")
	assert.Equal(t, []string{"a"}, watcher.updatedNames(), "the already-known object is an update")
	assert.Empty(t, watcher.deletedNames())
}

// A failing re-list is reported and returns an error, leaving the snapshot untouched so the next tick retries
// against the same baseline instead of diffing a partial set as a mass deletion.
func TestBusInformer_FailedRelistLeavesSnapshotIntact(t *testing.T) {
	watcher := &recordingWatcher{}
	var fail atomic.Bool
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		if fail.Load() {
			return nil, "", errors.New("api unavailable")
		}
		return []resource.Object{busObj("a")}, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{
		ErrorHandler: func(context.Context, error) {},
	})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	fail.Store(true)
	require.Error(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"a"}, busNames(inf.store.List()))
	assert.Empty(t, watcher.deletedNames())
}

// A reconnect drives an out-of-band re-list, so events missed while the subscription was down are reconciled
// without waiting for the re-list cadence.
func TestBusInformer_ReconnectTriggersRelist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls.Add(1)
			return []resource.Object{busObj("a")}, "", nil
		})
		// An hour-long cadence guarantees only the reconnect can trigger the second list.
		inf := newBusInformer(t, sub, lister, &recordingWatcher{}, BusInformerOptions{ResyncInterval: time.Hour})
		stop := startBusInformer(t, inf)
		defer stop()

		require.Equal(t, int64(1), calls.Load(), "the subscription opens before the list, so startup lists once")

		sub.Reconnect(busTestGVR(), busTestNamespace)
		synctest.Wait()

		assert.Equal(t, int64(2), calls.Load(), "a reconnect must trigger a re-list")
	})
}

// A failing subscription gates startup by default: the informer retries and only lists, and reports synced, once it
// opens, then delivers live events.
func TestBusInformer_RetriesSubscribeUntilAvailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailSubscribes(2)
		watcher := &recordingWatcher{}
		inf := newBusInformer(t, sub, staticLister(busObj("a")), watcher, BusInformerOptions{ResyncInterval: time.Hour})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.False(t, sub.Subscribed(busTestGVR(), busTestNamespace), "must not subscribe while Subscribe fails")
		require.False(t, inf.HasSynced(), "must not sync before the subscription opens")

		for range 5 {
			if sub.Subscribed(busTestGVR(), busTestNamespace) {
				break
			}
			time.Sleep(DefaultBusRetryInterval)
			synctest.Wait()
		}
		require.True(t, sub.Subscribed(busTestGVR(), busTestNamespace), "the retry must eventually subscribe")
		require.True(t, inf.HasSynced(), "must sync once subscribed")
		assert.Equal(t, []string{"a"}, watcher.addedNames(), "the initial list must be delivered")

		require.NoError(t, sub.Publish(context.Background(), busTestGVR(), busTestNamespace,
			busEvent(resourcebus.VerbAdded, "fresh", "11")))
		synctest.Wait()
		assert.Equal(t, []string{"a", "fresh"}, watcher.addedNames())
	})
}

// With a degraded start a failing subscription no longer gates startup: the informer lists, reports synced, and
// keeps re-listing while the subscription retries in the background. Once it opens, a re-list covers the gap.
func TestBusInformer_DegradedStartListsWithoutSubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailSubscribes(2)
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls.Add(1)
			return []resource.Object{busObj("a")}, "", nil
		})
		inf := newBusInformer(t, sub, lister, watcher, BusInformerOptions{
			ResyncInterval:     time.Hour,
			AllowDegradedStart: true,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.False(t, sub.Subscribed(busTestGVR(), busTestNamespace), "the subscription is still failing")
		require.True(t, inf.HasSynced(), "a degraded start must sync from the initial list")
		assert.Equal(t, []string{"a"}, watcher.addedNames())

		before := calls.Load()
		for range 5 {
			if sub.Subscribed(busTestGVR(), busTestNamespace) {
				break
			}
			time.Sleep(DefaultBusRetryInterval)
			synctest.Wait()
		}
		require.True(t, sub.Subscribed(busTestGVR(), busTestNamespace), "the background retry must eventually subscribe")
		assert.Greater(t, calls.Load(), before, "opening the subscription must trigger a gap-closing re-list")

		require.NoError(t, sub.Publish(context.Background(), busTestGVR(), busTestNamespace,
			busEvent(resourcebus.VerbAdded, "fresh", "11")))
		synctest.Wait()
		assert.Equal(t, []string{"a", "fresh"}, watcher.addedNames())
	})
}

// A failing initial list must not mark the informer synced: WaitForSync keeps blocking until the first list
// succeeds, then the listed objects are delivered.
func TestBusInformer_DoesNotSyncUntilInitialListSucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if calls.Add(1) < 3 {
				return nil, "", errors.New("api unavailable")
			}
			return []resource.Object{busObj("a")}, "", nil
		})
		inf := newBusInformer(t, sub, lister, watcher, BusInformerOptions{
			ResyncInterval: time.Hour,
			ErrorHandler:   func(context.Context, error) {},
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, sub.Subscribed(busTestGVR(), busTestNamespace))
		require.False(t, inf.HasSynced(), "must not sync while the initial list fails")
		require.Error(t, inf.HealthCheck(ctx))

		for range 5 {
			if inf.HasSynced() {
				break
			}
			time.Sleep(DefaultBusRetryInterval)
			synctest.Wait()
		}
		require.True(t, inf.HasSynced(), "must sync after the initial list succeeds")
		require.NoError(t, inf.HealthCheck(ctx))
		assert.Equal(t, []string{"a"}, watcher.addedNames())
	})
}

// WaitForSync returns the context's error rather than blocking forever when the informer never syncs.
func TestBusInformer_WaitForSyncHonorsContext(t *testing.T) {
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, inf.WaitForSync(ctx), context.Canceled)
}

// signalReconnect never blocks: a pending signal coalesces further reconnects, so a burst while the run loop is busy
// cannot deadlock the bus client's reconnect goroutine.
func TestBusInformer_SignalReconnectDoesNotBlock(t *testing.T) {
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{})
	for range 100 {
		inf.signalReconnect()
	}
}

// A live add is written to the snapshot, so the next re-list re-delivers it as a retained update rather than
// re-reporting a delivered add as a change the live stream missed.
func TestBusInformer_LiveAddNotReCountedAsRelistRecovery(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return nil, "", nil
		}
		return []resource.Object{busObj("x")}, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "55"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "the live add is the only add")
	assert.Equal(t, []string{"x"}, watcher.updatedNames(), "the re-list re-delivers it as an update")
	assert.Empty(t, watcher.deletedNames())
}

// A live add which lands after a re-list read its snapshot must not be diffed as a delete: the snapshot's silence
// about it is stale, so the store carries it forward until a snapshot which postdates it.
func TestBusInformer_LiveAddNewerThanRelistSnapshotIsNotDeleted(t *testing.T) {
	watcher := &recordingWatcher{}
	// Every list is read at 150 and omits "x"; "x" is live-added at 200, after that snapshot.
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "150", nil })
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "the live add is the only add")
	assert.Empty(t, watcher.deletedNames(), "a live add newer than the snapshot must not be diffed as a delete")
	assert.Equal(t, []string{"x"}, busNames(inf.store.List()), "x is carried forward")
}

// A live delete which lands after a re-list read its snapshot must not be resurrected: the store suppresses the
// stale re-add rather than dispatching a spurious add for a just-deleted object.
func TestBusInformer_LiveDeleteNewerThanRelistSnapshotIsNotResurrected(t *testing.T) {
	watcher := &recordingWatcher{}
	// Every list is read at 150 and still lists "x" at 120; "x" is live-deleted at 200.
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return []resource.Object{busObjRV("x", 120)}, "150", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "x is added once, by the initial list")
	assert.Equal(t, []string{"x"}, watcher.deletedNames(), "the live delete is delivered")
	assert.Empty(t, watcher.updatedNames(), "the stale re-list must not re-deliver the deleted object")
	assert.Empty(t, inf.store.List(), "x stays evicted")
}

// With opaque resource versions the store's arbitration is off, so a live delete racing a re-list produces a
// spurious add/delete pair — the documented degradation — which the next re-list corrects.
func TestBusInformer_OpaqueResourceVersionsDegradeToSpuriousPair(t *testing.T) {
	watcher := &recordingWatcher{}
	listed := busObj("x")
	listed.ResourceVersion = "opaque-120"
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls <= 2 {
			return []resource.Object{listed}, "opaque-150", nil
		}
		return nil, "opaque-250", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "opaque-200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x", "x"}, watcher.addedNames(), "the stale snapshot re-adds the deleted object")
	assert.Equal(t, []string{"x"}, watcher.deletedNames())

	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	assert.Equal(t, []string{"x", "x"}, watcher.deletedNames(), "the next re-list corrects it")
	assert.Empty(t, inf.store.List())
}

// fakeGetter serves the hydrated payload's reads.
type fakeGetter struct {
	mu      sync.Mutex
	objects map[string]resource.Object
	err     error
	onGet   func()
}

func (g *fakeGetter) Get(_ context.Context, id resource.Identifier) (resource.Object, error) {
	g.mu.Lock()
	onGet, err := g.onGet, g.err
	obj, ok := g.objects[busStoreKey(id.Namespace, id.Name)]
	g.mu.Unlock()
	if onGet != nil {
		onGet()
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: busTestKind.Group(), Resource: busTestKind.Plural()}, id.Name)
	}
	return obj, nil
}

func TestBusInformer_HydratedPayload(t *testing.T) {
	hydrated := func(name, rv string) *resource.UntypedObject {
		o := busObj(name)
		o.ResourceVersion = rv
		o.Spec = map[string]any{"field": name}
		return o
	}

	t.Run("delivers the object read from the API", func(t *testing.T) {
		watcher := &recordingWatcher{}
		getter := &fakeGetter{objects: map[string]resource.Object{
			busStoreKey(busTestNamespace, "x"): hydrated("x", "200"),
		}}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  getter,
		})
		assert.Equal(t, WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerChange | WatchTriggerSchedule},
			inf.WatchOptions())

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		require.Len(t, watcher.added(), 1)
		assert.Equal(t, map[string]any{"field": "x"}, watcher.added()[0].GetSpec())
		assert.Equal(t, []string{"x"}, busNames(inf.store.List()), "the hydrated object is written through")
	})

	t.Run("a modification is written through", func(t *testing.T) {
		getter := &fakeGetter{objects: map[string]resource.Object{
			busStoreKey(busTestNamespace, "x"): hydrated("x", "300"),
		}}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), &recordingWatcher{}, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  getter,
		})

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "x", "300"))

		stored, ok := inf.store.Get(busTestNamespace, "x")
		require.True(t, ok, "a hydrated modification carries the whole object, so it is safe to store")
		assert.Equal(t, "300", stored.GetResourceVersion())
	})

	t.Run("a read failure drops the event", func(t *testing.T) {
		watcher := &recordingWatcher{}
		var handled []error
		getter := &fakeGetter{err: errors.New("api unavailable")}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
			Payload:      PayloadHydrated,
			Getter:       getter,
			ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
		})

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		assert.Empty(t, watcher.addedNames(), "nothing is dispatched for an event whose object cannot be read")
		require.Len(t, handled, 1)
		assert.Contains(t, handled[0].Error(), "api unavailable")
	})

	t.Run("an object deleted mid-flight drops the event", func(t *testing.T) {
		watcher := &recordingWatcher{}
		var handled []error
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
			Payload:      PayloadHydrated,
			Getter:       &fakeGetter{objects: map[string]resource.Object{}},
			ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
		})

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		assert.Empty(t, watcher.addedNames())
		assert.Empty(t, busNames(inf.store.List()), "a dropped event leaves no trace in the snapshot")
		require.Len(t, handled, 1)
		assert.Contains(t, handled[0].Error(), "no longer exists")
	})

	t.Run("a delete is dispatched without a read", func(t *testing.T) {
		watcher := &recordingWatcher{}
		var gets int
		getter := &fakeGetter{objects: map[string]resource.Object{}, onGet: func() { gets++ }}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(hydrated("x", "120")), watcher, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  getter,
		})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "200"))

		assert.Zero(t, gets, "the object is already gone from storage, so there is nothing to read")
		assert.Equal(t, []string{"x"}, watcher.deletedNames())
	})

	t.Run("a read which races a re-list cannot resurrect a deleted object", func(t *testing.T) {
		watcher := &recordingWatcher{}
		getter := &fakeGetter{objects: map[string]resource.Object{
			busStoreKey(busTestNamespace, "x"): hydrated("x", "200"),
		}}
		// The list is read at 150 and omits "x"; the live add at 200 lands after it.
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "150", nil })
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  getter,
		})

		require.NoError(t, inf.relist(context.Background(), initialRelist))
		// The read happens while a re-list is in flight; whichever order they complete in, the version the hydrated
		// object carries is what the snapshot arbitrates against.
		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))
		require.NoError(t, inf.relist(context.Background(), periodicRelist))

		assert.Equal(t, []string{"x"}, watcher.addedNames())
		assert.Empty(t, watcher.deletedNames(), "the stale snapshot must not delete the hydrated object")
		assert.Equal(t, []string{"x"}, busNames(inf.store.List()))
	})
}

// The snapshot reports what the informer last reconciled with storage, so a staleness-tolerant count costs no API
// list. It tracks the re-list diff and the live write-throughs alike.
func TestBusInformer_Snapshot(t *testing.T) {
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObj("a"), busObj("b")}, "", nil
		}
		return []resource.Object{busObj("a")}, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, &recordingWatcher{}, BusInformerOptions{})

	assert.Empty(t, inf.Snapshot(), "nothing is listed before the first re-list")

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	assert.ElementsMatch(t, []resource.Identifier{
		{Namespace: busTestNamespace, Name: "a"},
		{Namespace: busTestNamespace, Name: "b"},
	}, inf.Snapshot())

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "live", "11"))
	assert.Len(t, inf.Snapshot(), 3, "a live add is written through")

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "live", "12"))
	assert.Len(t, inf.Snapshot(), 2, "a live delete is evicted")

	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	assert.Equal(t, []resource.Identifier{{Namespace: busTestNamespace, Name: "a"}}, inf.Snapshot(),
		"the re-list drops what storage no longer lists")
}

// A live delete and a re-list diff are applied and delivered on one goroutine, so one finishes before the other
// starts. Which goes first is the run loop's select, and both orders are correct: either the delete lands alone and
// the stale re-list is suppressed, or the re-list adds the listed object and the newer delete removes it again.
// What must hold in both is that the delete is last and the snapshot ends empty — were the two to interleave,
// watchers could finish on an add for an object the store no longer holds.
func TestBusInformer_LiveEventsAndRelistsShareOneOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		var order []string
		watcher := &SimpleWatcher{
			AddFunc:    func(context.Context, resource.Object) error { order = append(order, "add"); return nil },
			DeleteFunc: func(context.Context, resource.Object) error { order = append(order, "delete"); return nil },
		}
		var calls int
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls++
			if calls == 1 {
				return nil, "100", nil
			}
			return []resource.Object{busObjRV("x", 120)}, "150", nil
		})
		inf := newBusInformer(t, sub, lister, watcher, BusInformerOptions{ResyncInterval: time.Hour})
		stop := startBusInformer(t, inf)
		defer stop()

		// Queue a delete and ask for a re-list which lists the same key.
		require.NoError(t, sub.Publish(context.Background(), busTestGVR(), busTestNamespace,
			busEvent(resourcebus.VerbDeleted, "x", "200")))
		sub.Reconnect(busTestGVR(), busTestNamespace)
		synctest.Wait()

		assert.Empty(t, inf.store.List(), "the delete at 200 outranks the snapshot read at 150")
		require.NotEmpty(t, order)
		assert.Equal(t, "delete", order[len(order)-1], "whichever ran first, the delete is the last thing watchers saw")
		assert.Contains(t, [][]string{{"delete"}, {"add", "delete"}}, order)
	})
}

func TestBusInformer_DropsEventsASupersedingListAlreadyAccountsFor(t *testing.T) {
	t.Run("a queued delete cannot evict a re-creation the list saw", func(t *testing.T) {
		watcher := &recordingWatcher{}
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			return []resource.Object{busObjRV("x", 300)}, "300", nil
		})
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{
			ErrorHandler: func(context.Context, error) {},
		})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "200"))

		assert.Empty(t, watcher.deletedNames(), "the delete predates the listed re-creation")
		assert.Equal(t, []string{"x"}, busNames(inf.store.List()))
	})

	t.Run("a queued add cannot resurrect a key the list confirmed absent", func(t *testing.T) {
		watcher := &recordingWatcher{}
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "300", nil })
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		assert.Empty(t, watcher.addedNames(), "the list read at 300 already accounts for the add at 200")
		assert.Empty(t, inf.store.List())
	})

	t.Run("an event newer than the list is applied", func(t *testing.T) {
		watcher := &recordingWatcher{}
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "300", nil })
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "400"))

		assert.Equal(t, []string{"x"}, watcher.addedNames())
	})

	// The whole comparison rests on numeric versions, so an opaque one leaves it off rather than guessing.
	t.Run("opaque versions disable the check", func(t *testing.T) {
		watcher := &recordingWatcher{}
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "opaque", nil })
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		assert.Equal(t, []string{"x"}, watcher.addedNames())
	})
}

// An identity-mode modification keeps the stored object but must still advance the version the key is ordered by,
// or a later out-of-order delete between the listed and modified versions reads as newer and evicts it.
func TestBusInformer_IdentityModificationAdvancesTheOrderingVersion(t *testing.T) {
	watcher := &recordingWatcher{}
	var handled []error
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("x", 100)), watcher, BusInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, inf.relist(context.Background(), initialRelist))

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "x", "300"))
	assert.Equal(t, []string{"x"}, watcher.updatedNames())
	stored, ok := inf.store.Get(busTestNamespace, "x")
	require.True(t, ok)
	assert.Equal(t, "100", stored.GetResourceVersion(), "the listed object is kept, identity not stored over it")

	// The delete at 200 predates the modification at 300, so it must not evict the key.
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "200"))

	assert.Empty(t, watcher.deletedNames(), "the stale delete is not delivered")
	assert.Equal(t, []string{"x"}, busNames(inf.store.List()))
	require.Len(t, handled, 1)
	assert.Contains(t, handled[0].Error(), "superseded by a newer write")
}

// A dropped event forces a re-list, but a sustained backlog drops many and one list recovers all of them, so
// scheduling a list per drop only spends requests on work already in hand.
func TestBusInformer_OverflowRelistIsRateLimited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), &recordingWatcher{}, BusInformerOptions{
			EventQueueSize:         1,
			OverflowRelistInterval: time.Minute,
		})

		// Nothing drains the queue, so every event after the first overflows.
		for i := range 50 {
			inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", strconv.Itoa(i)))
		}

		assert.Len(t, inf.relistSignal, 1, "the drops coalesce into one re-list request")

		// Past the interval, a further drop may schedule another.
		<-inf.relistSignal
		time.Sleep(time.Minute + time.Second)
		inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "99"))
		assert.Len(t, inf.relistSignal, 1)
	})
}

// The drain is bounded, which is what stops a backlog from starving the re-list: the run loop falls through to the
// select after at most this many events, so the resync timer is read even while the queue has more waiting.
func TestBusInformer_DrainEventsIsBounded(t *testing.T) {
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
		EventQueueSize:  100,
		EventDrainBatch: 4,
	})
	for i := range 10 {
		inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, strconv.Itoa(i), "1"))
	}

	inf.drainEvents(context.Background())

	assert.Len(t, watcher.addedNames(), 4, "one pass applies at most the batch")
	assert.Len(t, inf.events, 6, "the rest stay queued for the next pass")

	// An empty queue returns immediately rather than waiting for one.
	inf.drainEvents(context.Background())
	inf.drainEvents(context.Background())
	assert.Len(t, inf.events, 0)
	inf.drainEvents(context.Background())
	assert.Len(t, watcher.addedNames(), 10)
}

// A canceled context stops the drain rather than applying the rest of the batch.
func TestBusInformer_DrainEventsStopsOnContextCancellation(t *testing.T) {
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{EventQueueSize: 10})
	for i := range 5 {
		inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, strconv.Itoa(i), "1"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inf.drainEvents(ctx)

	assert.Empty(t, watcher.addedNames())
	assert.Len(t, inf.events, 5, "the queue is left for a later pass")
}

// Subscribe returning is not delivery being ready. A subscription which never becomes ready must be closed and
// retried, and must not let the initial list run — listing against it would report synced while changes are still
// being missed, which is the whole reason the subscription is opened before the list.
func TestBusInformer_WaitsForTheSubscriptionToBeReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailReady(1)
		watcher := &recordingWatcher{}
		var lists atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return []resource.Object{busObj("a")}, "", nil
		})
		inf := newBusInformer(t, sub, lister, watcher, BusInformerOptions{
			ResyncInterval:           time.Hour,
			SubscriptionReadyTimeout: 5 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.False(t, inf.HasSynced(), "the initial list must not run against a subscription which cannot deliver")
		require.Zero(t, lists.Load())

		// The readiness deadline expires, that handle is closed, and the retry opens one which is ready.
		time.Sleep(5*time.Second + DefaultBusRetryInterval + time.Second)
		synctest.Wait()

		require.True(t, inf.HasSynced(), "the retry's subscription is ready, so the list runs")
		assert.True(t, sub.Subscribed(busTestGVR(), busTestNamespace))
		assert.Equal(t, []string{"a"}, watcher.addedNames())

		require.NoError(t, sub.Publish(context.Background(), busTestGVR(), busTestNamespace,
			busEvent(resourcebus.VerbAdded, "fresh", "11")))
		synctest.Wait()
		assert.Equal(t, []string{"a", "fresh"}, watcher.addedNames(), "and live events flow on it")
	})
}

// A degraded start must not leave degraded mode on a subscription which is accepted but cannot deliver.
func TestBusInformer_DegradedStartWaitsForReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailSubscribes(1)
		sub.FailReady(1)
		var lists atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "", nil
		})
		inf := newBusInformer(t, sub, lister, &recordingWatcher{}, BusInformerOptions{
			ResyncInterval:           time.Hour,
			AllowDegradedStart:       true,
			SubscriptionReadyTimeout: 5 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, inf.HasSynced(), "a degraded start still lists")

		// The first retry is accepted but never becomes ready, so degraded mode is not left: leaving it re-lists,
		// and nothing has.
		before := lists.Load()
		time.Sleep(DefaultBusRetryInterval + 5*time.Second + time.Second)
		synctest.Wait()
		assert.Equal(t, before, lists.Load(), "a subscription which cannot deliver is not an open one")

		// The next retry is ready, which forces the gap-closing re-list.
		time.Sleep(DefaultBusRetryInterval + time.Second)
		synctest.Wait()
		assert.Greater(t, lists.Load(), before, "leaving degraded mode re-lists to cover the gap")
	})
}

// A live event carries whatever metadata the bus announced with it, so a consumer can decide without reading the
// object — which is the point of announcing it at all.
func TestBusInformer_LiveEventsCarryAnnouncedMetadata(t *testing.T) {
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{})

	deletionTime := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	createdTime := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	evt := busEvent(resourcebus.VerbAdded, "x", "100")
	evt.Metadata = &resourcebus.EventMetadata{
		Labels:            map[string]string{"app": "test"},
		Annotations:       map[string]string{"note": "hi"},
		Finalizers:        []string{"myapp.grafana.app/finalizer"},
		Generation:        7,
		CreationTimestamp: createdTime,
		DeletionTimestamp: &deletionTime,
	}

	inf.handleEvent(context.Background(), evt)

	require.Len(t, watcher.added(), 1)
	got := watcher.added()[0]
	assert.Equal(t, map[string]string{"app": "test"}, got.GetLabels())
	assert.Equal(t, map[string]string{"note": "hi"}, got.GetAnnotations())
	assert.Equal(t, []string{"myapp.grafana.app/finalizer"}, got.GetFinalizers())
	assert.Equal(t, int64(7), got.GetGeneration())
	assert.Equal(t, createdTime, got.GetCreationTimestamp().Time)
	require.NotNil(t, got.GetDeletionTimestamp())
	assert.Equal(t, deletionTime, got.GetDeletionTimestamp().Time)
}

// An event with no announced metadata leaves the identity bare: an absent field means unknown, so nothing is
// invented for it.
func TestBusInformer_LiveEventsWithoutMetadataDeliverBareIdentities(t *testing.T) {
	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{})

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "100"))

	require.Len(t, watcher.added(), 1)
	got := watcher.added()[0]
	assert.Empty(t, got.GetLabels())
	assert.Empty(t, got.GetFinalizers())
	assert.Zero(t, got.GetGeneration())
	assert.Nil(t, got.GetDeletionTimestamp())
}

// A re-list must not deliver less than the live stream: the identity projection keeps the metadata events carry.
func TestBusInformer_RelistIdentitiesKeepAnnouncedMetadata(t *testing.T) {
	deletionTime := metav1.NewTime(time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC))
	listed := busObjRV("x", 100)
	listed.Spec = map[string]any{"field": "value"}
	listed.Labels = map[string]string{"app": "test"}
	listed.Finalizers = []string{"myapp.grafana.app/finalizer"}
	listed.Generation = 7
	listed.DeletionTimestamp = &deletionTime

	watcher := &recordingWatcher{}
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(listed), watcher, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))

	require.Len(t, watcher.added(), 1)
	got := watcher.added()[0]
	assert.Empty(t, got.GetSpec(), "the body is still dropped")
	assert.Equal(t, map[string]string{"app": "test"}, got.GetLabels())
	assert.Equal(t, []string{"myapp.grafana.app/finalizer"}, got.GetFinalizers())
	assert.Equal(t, int64(7), got.GetGeneration())
	require.NotNil(t, got.GetDeletionTimestamp())
}

func labelled(name string, labels map[string]string) *resource.UntypedObject {
	obj := busObj(name)
	obj.Labels = labels
	return obj
}

// A bus delivers by namespace alone, so a filtered informer checks every live event against the same selectors the
// list is narrowed by. Otherwise excluded objects would land in the snapshot and be diffed as deletes.
func TestBusInformer_FiltersLiveEventsBySelector(t *testing.T) {
	newFiltered := func(t *testing.T, watcher ResourceWatcher, lister ResourceLister) *BusInformer {
		t.Helper()
		return newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{
			ListWatchOptions:  ListWatchOptions{LabelFilters: []string{"app=mine"}},
			AnnouncedMetadata: AnnouncedMetadata{Labels: []string{"app"}},
		})
	}
	event := func(verb resourcebus.Verb, name, rv string, labels map[string]string) resourcebus.Event {
		evt := busEvent(verb, name, rv)
		evt.Metadata = &resourcebus.EventMetadata{Labels: labels}
		return evt
	}

	t.Run("a matching object is delivered", func(t *testing.T) {
		watcher := &recordingWatcher{}
		inf := newFiltered(t, watcher, staticLister())

		inf.handleEvent(context.Background(), event(resourcebus.VerbAdded, "mine", "100", map[string]string{"app": "mine"}))

		assert.Equal(t, []string{"mine"}, watcher.addedNames())
		assert.Equal(t, []string{"mine"}, busNames(inf.store.List()))
	})

	t.Run("an excluded object is dropped without a word", func(t *testing.T) {
		watcher := &recordingWatcher{}
		inf := newFiltered(t, watcher, staticLister())

		inf.handleEvent(context.Background(), event(resourcebus.VerbAdded, "theirs", "100", map[string]string{"app": "theirs"}))

		assert.Empty(t, watcher.addedNames(), "it was never in scope")
		assert.Empty(t, watcher.deletedNames())
		assert.Empty(t, inf.store.List(), "and it must not reach the snapshot, which the re-list diffs against")
	})

	// The membership transition: an object whose labels change so it leaves the selector is gone as far as a
	// watcher is concerned, and is delivered as a delete — the same thing a filtered watch reports.
	t.Run("an object which leaves the selector is delivered as a delete", func(t *testing.T) {
		watcher := &recordingWatcher{}
		inf := newFiltered(t, watcher, staticLister())
		inf.handleEvent(context.Background(), event(resourcebus.VerbAdded, "x", "100", map[string]string{"app": "mine"}))
		require.Equal(t, []string{"x"}, watcher.addedNames())

		inf.handleEvent(context.Background(), event(resourcebus.VerbModified, "x", "200", map[string]string{"app": "theirs"}))

		assert.Equal(t, []string{"x"}, watcher.deletedNames(), "it left the scope")
		assert.Empty(t, watcher.updatedNames())
		assert.Empty(t, inf.store.List())
	})

	t.Run("an object which enters the selector is delivered as an add", func(t *testing.T) {
		watcher := &recordingWatcher{}
		inf := newFiltered(t, watcher, staticLister())

		inf.handleEvent(context.Background(), event(resourcebus.VerbModified, "x", "100", map[string]string{"app": "theirs"}))
		require.Empty(t, watcher.addedNames())
		inf.handleEvent(context.Background(), event(resourcebus.VerbModified, "x", "200", map[string]string{"app": "mine"}))

		assert.Equal(t, []string{"x"}, watcher.addedNames(), "first seen in scope, so an add")
	})

	// The server narrows the list, but the snapshot is what the live path arbitrates against, so it holds only
	// objects in scope however the lister was implemented.
	t.Run("a re-list drops what the selector excludes", func(t *testing.T) {
		watcher := &recordingWatcher{}
		lister := staticLister(
			labelled("mine", map[string]string{"app": "mine"}),
			labelled("theirs", map[string]string{"app": "theirs"}),
		)
		inf := newFiltered(t, watcher, lister)

		require.NoError(t, inf.relist(context.Background(), initialRelist))

		assert.Equal(t, []string{"mine"}, watcher.addedNames())
		assert.Equal(t, []string{"mine"}, busNames(inf.store.List()))
	})
}

// A hydrated payload carries the whole object, so a field selector can be evaluated against it where the kind
// declares the field selectable.
func TestBusInformer_FiltersByFieldSelector(t *testing.T) {
	kind := resource.Kind{Schema: resource.NewSimpleSchema("example.grafana.app", "v1",
		&resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind("Widget"), resource.WithPlural("widgets"),
		resource.WithSelectableFields([]resource.SelectableField{{
			FieldSelector:  "spec.tier",
			FieldValueFunc: func(o resource.Object) (string, error) { return o.GetLabels()["tier"], nil },
		}}))}

	inScope, outOfScope := busObjRV("in", 100), busObjRV("out", 100)
	inScope.Labels = map[string]string{"tier": "gold"}
	outOfScope.Labels = map[string]string{"tier": "bronze"}
	getter := &fakeGetter{objects: map[string]resource.Object{
		busStoreKey(busTestNamespace, "in"):  inScope,
		busStoreKey(busTestNamespace, "out"): outOfScope,
	}}

	watcher := &recordingWatcher{}
	inf, err := NewBusInformer(kind, resourcebus.NewFake(), staticLister(), BusInformerOptions{
		Namespace:        busTestNamespace,
		Payload:          PayloadHydrated,
		Getter:           getter,
		ListWatchOptions: ListWatchOptions{FieldSelectors: []string{"spec.tier=gold"}},
	})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(watcher))

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "in", "100"))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "out", "100"))

	assert.Equal(t, []string{"in"}, watcher.addedNames())
}

func TestNewBusInformer_SelectorValidation(t *testing.T) {
	sub, lister := resourcebus.NewFake(), staticLister()

	t.Run("a field selector the kind does not declare selectable is refused", func(t *testing.T) {
		_, err := NewBusInformer(busTestKind, sub, lister, BusInformerOptions{
			Payload:          PayloadHydrated,
			Getter:           &fakeGetter{objects: map[string]resource.Object{}},
			ListWatchOptions: ListWatchOptions{FieldSelectors: []string{"spec.nope=1"}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not declare it selectable")
	})

	t.Run("a malformed label filter is refused", func(t *testing.T) {
		_, err := NewBusInformer(busTestKind, sub, lister, BusInformerOptions{
			ListWatchOptions:  ListWatchOptions{LabelFilters: []string{"!!!"}},
			AnnouncedMetadata: AnnouncedMetadata{Labels: []string{"app"}},
		})
		assert.Error(t, err)
	})

	t.Run("no selectors builds no matcher", func(t *testing.T) {
		inf, err := NewBusInformer(busTestKind, sub, lister, BusInformerOptions{})
		require.NoError(t, err)
		assert.Nil(t, inf.matcher, "an unfiltered informer pays nothing")
		assert.True(t, inf.matcher.Matches(busObj("anything")), "and a nil matcher accepts everything")
	})
}

// A count served from the snapshot is what a quota check reads instead of an API list per reconcile.
func TestBusInformer_CountObserveAndForget(t *testing.T) {
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("a", 10)), nil, BusInformerOptions{})
	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.Equal(t, 1, inf.Count(busTestNamespace))

	t.Run("an object the consumer read for itself is counted before the next re-list", func(t *testing.T) {
		inf.Observe(busObjRV("b", 20))
		assert.Equal(t, 2, inf.Count(busTestNamespace))
		assert.Equal(t, 2, inf.Count(""), "every namespace")
		assert.ElementsMatch(t, []string{"a", "b"}, identifierNames(inf.Snapshot()))
	})

	t.Run("observing dispatches nothing", func(t *testing.T) {
		watcher := &recordingWatcher{}
		require.NoError(t, inf.AddEventHandler(watcher))
		inf.Observe(busObjRV("c", 30))
		assert.Empty(t, watcher.addedNames(), "the consumer is already handling the object")
		inf.Forget(resource.Identifier{Namespace: busTestNamespace, Name: "c"})
	})

	t.Run("an absent object stops being counted", func(t *testing.T) {
		inf.Forget(resource.Identifier{Namespace: busTestNamespace, Name: "b"})
		assert.Equal(t, 1, inf.Count(busTestNamespace))
	})

	// Without this a repository created and then deleted while its delete went to another replica would be counted
	// for as long as the process ran.
	t.Run("a re-list absorbs an observed object which is gone", func(t *testing.T) {
		inf.Observe(busObjRV("d", 40))
		require.Equal(t, 2, inf.Count(busTestNamespace))
		require.NoError(t, inf.relist(context.Background(), periodicRelist))
		assert.Equal(t, 1, inf.Count(busTestNamespace), "the list is authoritative about what is gone")
	})
}

// A stale read must not roll the snapshot back, and a read of an object the app does not watch must not enter it.
func TestBusInformer_ObserveIsArbitratedLikeALiveEvent(t *testing.T) {
	t.Run("a read older than what is held is refused", func(t *testing.T) {
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("a", 20)), nil, BusInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))
		inf.Observe(busObjRV("a", 10))
		held, ok := inf.store.Get(busTestNamespace, "a")
		require.True(t, ok)
		assert.Equal(t, "20", held.GetResourceVersion())
	})

	t.Run("a read the selectors exclude is refused", func(t *testing.T) {
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{
			ListWatchOptions:  ListWatchOptions{LabelFilters: []string{"app=test"}},
			AnnouncedMetadata: AnnouncedMetadata{Labels: []string{"app"}},
		})
		inf.Observe(busObjRV("a", 10))
		assert.Zero(t, inf.Count(""))

		labeled := busObjRV("b", 10)
		labeled.SetLabels(map[string]string{"app": "test"})
		inf.Observe(labeled)
		assert.Equal(t, 1, inf.Count(""))
	})

	// In identity mode the snapshot holds identities whatever it was handed, so a read of a full object does not
	// make steady-state memory scale with payload size.
	t.Run("an identity payload stores the identity, not the object read", func(t *testing.T) {
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{})
		obj := busObjRV("a", 10)
		obj.SetAnnotations(map[string]string{"kept": "yes"})
		inf.Observe(obj)

		held, ok := inf.store.Get(busTestNamespace, "a")
		require.True(t, ok)
		assert.NotSame(t, obj, held)
		assert.Equal(t, map[string]string{"kept": "yes"}, held.GetAnnotations())
	})
}

func identifierNames(ids []resource.Identifier) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Name
	}
	return out
}

// What identity mode retains is the identity plus the metadata a notification can carry, and nothing of the body.
// Dropping the metadata would make a re-list deliver less than the live stream and leave a selector nothing to
// match on; keeping the body is what this informer exists to avoid.
func TestBusInformer_SnapshotRetainsMetadataButNoBody(t *testing.T) {
	listed := busObjRV("a", 10)
	listed.SetLabels(map[string]string{"app": "test"})
	listed.SetAnnotations(map[string]string{"example.grafana.app/paused": "true"})
	listed.SetFinalizers([]string{"example.grafana.app/finalizer"})
	listed.SetGeneration(7)
	listed.Spec = map[string]any{"field": "a body which must not be retained"}

	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(listed), nil, BusInformerOptions{})
	require.NoError(t, inf.relist(context.Background(), initialRelist))

	held, ok := inf.store.Get(busTestNamespace, "a")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"app": "test"}, held.GetLabels())
	assert.Equal(t, map[string]string{"example.grafana.app/paused": "true"}, held.GetAnnotations())
	assert.Equal(t, []string{"example.grafana.app/finalizer"}, held.GetFinalizers())
	assert.Equal(t, int64(7), held.GetGeneration())
	assert.Equal(t, "10", held.GetResourceVersion())
	assert.Empty(t, held.(*resource.UntypedObject).Spec, "the body is what identity mode drops")
}
