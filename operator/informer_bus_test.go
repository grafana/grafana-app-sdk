package operator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

var busTestKind = resource.Kind{
	Schema: resource.NewSimpleSchema("example.grafana.app", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind("Widget"), resource.WithPlural("widgets")),
}

// listerFunc adapts a function to ResourceLister.
type listerFunc func(ctx context.Context) ([]resource.Object, string, error)

func (f listerFunc) List(ctx context.Context) ([]resource.Object, string, error) { return f(ctx) }

func staticLister(objs ...resource.Object) ResourceLister {
	return listerFunc(func(context.Context) ([]resource.Object, string, error) { return objs, "", nil })
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
	assert.True(t, inf.IdentityPayload(), "the identity payload is the default")
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
	m := newTestBusMetrics()
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), &recordingWatcher{}, BusInformerOptions{
		EventQueueSize: 1,
		Metrics:        m,
	})

	// Nothing is draining the queue: the first event fills it, the second is dropped.
	inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "queued", "1"))
	inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "dropped", "2"))

	assert.Equal(t, float64(1), busMetricValue(t, m, "dropped_events"))
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

// A live event carries the version it was announced at on the context, which the delivered object does not in
// general have: a delete delivers the last-known object and a hydrated read whatever the API returned.
func TestBusInformer_AnnouncedVersionIsCarriedSeparately(t *testing.T) {
	type seen struct {
		announced string
		onObject  string
	}

	t.Run("a delete delivers the last-known object, not the announcement", func(t *testing.T) {
		var deletes []seen
		watcher := &SimpleWatcher{
			DeleteFunc: func(ctx context.Context, obj resource.Object) error {
				deletes = append(deletes, seen{AnnouncedVersionFromContext(ctx), obj.GetResourceVersion()})
				return nil
			},
		}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("x", 100)), watcher, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  &fakeGetter{objects: map[string]resource.Object{}},
		})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "200"))

		require.Len(t, deletes, 1)
		assert.Equal(t, "200", deletes[0].announced, "the announcement's version is on the context")
		assert.Equal(t, "100", deletes[0].onObject, "the object keeps its own, not the announcement's")
	})

	t.Run("a hydrated read delivers the version the API returned", func(t *testing.T) {
		var adds []seen
		watcher := &SimpleWatcher{
			AddFunc: func(ctx context.Context, obj resource.Object) error {
				adds = append(adds, seen{AnnouncedVersionFromContext(ctx), obj.GetResourceVersion()})
				return nil
			},
		}
		// The announcement says 200; the read returns 250, having picked up a later write.
		getter := &fakeGetter{objects: map[string]resource.Object{
			busStoreKey(busTestNamespace, "x"): busObjRV("x", 250),
		}}
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), watcher, BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  getter,
		})

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		require.Len(t, adds, 1)
		assert.Equal(t, "200", adds[0].announced)
		assert.Equal(t, "250", adds[0].onObject, "the read's version is not overwritten by the announcement's")
	})
}

// A re-list delivery was caused by no announcement, so there is no version to report.
func TestBusInformer_RelistDeliveriesCarryNoAnnouncedVersion(t *testing.T) {
	var announced []string
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("x", 100)), &SimpleWatcher{
		AddFunc: func(ctx context.Context, _ resource.Object) error {
			announced = append(announced, AnnouncedVersionFromContext(ctx))
			return nil
		},
	}, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))

	assert.Equal(t, []string{""}, announced)
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
		assert.False(t, inf.IdentityPayload())

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

func newTestBusMetrics() *BusInformerMetrics {
	return NewBusInformerMetrics(BusInformerMetricsConfig{
		MetricsConfig:       metrics.DefaultConfig("test"),
		ResourceVersionTime: func(rv int64) time.Time { return time.UnixMilli(rv) },
	})
}

// busKindLabels is the label prefix every bus metric carries, one dimension per GVK component.
var busKindLabels = []string{busTestKind.Group(), busTestKind.Version(), busTestKind.Kind()}

func busMetricValue(t *testing.T, m *BusInformerMetrics, name string, extra ...string) float64 {
	t.Helper()
	labels := append(append([]string(nil), busKindLabels...), extra...)
	switch name {
	case "live_events":
		return testutil.ToFloat64(m.liveEvents.WithLabelValues(labels...))
	case "relist_events":
		return testutil.ToFloat64(m.relistEvents.WithLabelValues(labels...))
	case "reconnects":
		return testutil.ToFloat64(m.reconnects.WithLabelValues(labels...))
	case "live_subscription":
		return testutil.ToFloat64(m.liveSubscription.WithLabelValues(labels...))
	case "relist_requests":
		return testutil.ToFloat64(m.relistRequests.WithLabelValues(labels...))
	case "superseded_events":
		return testutil.ToFloat64(m.supersededEvents.WithLabelValues(labels...))
	case "dropped_events":
		return testutil.ToFloat64(m.droppedEvents.WithLabelValues(labels...))
	case "relist_projection":
		return testutil.ToFloat64(m.relistProjection.WithLabelValues(labels...))
	}
	t.Fatalf("unknown metric %q", name)
	return 0
}

// Live and re-list deliveries are counted in separate families: a live event is a delivery, a re-list add or delete
// is a change the live stream did not deliver here.
func TestBusInformer_MetricsSeparateLiveFromRelist(t *testing.T) {
	m := newTestBusMetrics()
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		switch calls {
		case 1:
			return []resource.Object{busObjRV("a", 11)}, "100", nil
		case 2:
			return []resource.Object{busObjRV("a", 11), busObjRV("b", 22)}, "100", nil
		default:
			return []resource.Object{busObjRV("a", 11)}, "100", nil
		}
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, &recordingWatcher{}, BusInformerOptions{Metrics: m})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, float64(2), busMetricValue(t, m, "relist_events", busVerbAdd), "a on the initial list, b recovered")
	assert.Equal(t, float64(2), busMetricValue(t, m, "relist_events", busVerbUpdate), "a retained twice")
	assert.Equal(t, float64(1), busMetricValue(t, m, "relist_events", busVerbDelete), "b vanished")
	assert.Zero(t, busMetricValue(t, m, "live_events", busVerbAdd), "re-list deliveries are not live events")

	for _, verb := range []resourcebus.Verb{resourcebus.VerbAdded, resourcebus.VerbModified, resourcebus.VerbDeleted} {
		inf.handleEvent(context.Background(), busEvent(verb, "x", "101"))
	}
	assert.Equal(t, float64(1), busMetricValue(t, m, "live_events", busVerbAdd))
	assert.Equal(t, float64(1), busMetricValue(t, m, "live_events", busVerbUpdate))
	assert.Equal(t, float64(1), busMetricValue(t, m, "live_events", busVerbDelete))
}

// Latency is only observed where the version dates the change: not for the initial list, not for a re-delivery of an
// unchanged object, and not for a re-list-detected delete whose last-known version predates it.
func TestBusInformer_MetricsLatencyOnlyWhereVersionDatesTheChange(t *testing.T) {
	m := newTestBusMetrics()
	recent := time.Now().Add(-time.Second).UnixMilli()
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObjRV("a", recent)}, "1", nil
		}
		return []resource.Object{busObjRV("a", recent), busObjRV("b", recent)}, "1", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, &recordingWatcher{}, BusInformerOptions{Metrics: m})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	assert.Zero(t, testutil.CollectAndCount(m.relistLatency), "the initial list carries no latency")

	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	assert.Equal(t, 1, testutil.CollectAndCount(m.relistLatency), "only the recovered add is timed")

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "opaque"))
	assert.Zero(t, testutil.CollectAndCount(m.liveLatency), "an opaque version cannot date the change")
	assert.Equal(t, float64(1), busMetricValue(t, m, "live_events", busVerbAdd), "but the event is still counted")

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "y", fmt.Sprint(recent)))
	assert.Equal(t, 1, testutil.CollectAndCount(m.liveLatency))
}

// The subscription gauge tracks what the informer can see: closed until the subscription opens, open once it does,
// and closed again once the informer stops.
func TestBusInformer_MetricsObserveSubscriptionAndReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestBusMetrics()
		sub := resourcebus.NewFake()
		inf := newBusInformer(t, sub, staticLister(), &recordingWatcher{}, BusInformerOptions{
			ResyncInterval: time.Hour,
			Metrics:        m,
		})

		stop := startBusInformer(t, inf)
		assert.Equal(t, float64(1), busMetricValue(t, m, "live_subscription"))

		sub.Reconnect(busTestGVR(), busTestNamespace)
		synctest.Wait()
		assert.Equal(t, float64(1), busMetricValue(t, m, "reconnects"))

		stop()
		assert.Equal(t, float64(0), busMetricValue(t, m, "live_subscription"))
	})
}

// A degraded start holds the gauge closed while it serves re-lists, and opens it once the background retry succeeds.
func TestBusInformer_MetricsDegradedStartGauge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestBusMetrics()
		sub := resourcebus.NewFake()
		sub.FailSubscribes(1)
		inf := newBusInformer(t, sub, staticLister(), &recordingWatcher{}, BusInformerOptions{
			ResyncInterval:     time.Hour,
			AllowDegradedStart: true,
			Metrics:            m,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, inf.HasSynced(), "degraded start syncs from the list alone")
		assert.Equal(t, float64(0), busMetricValue(t, m, "live_subscription"))

		time.Sleep(DefaultBusRetryInterval)
		synctest.Wait()
		require.True(t, sub.Subscribed(busTestGVR(), busTestNamespace))
		assert.Equal(t, float64(1), busMetricValue(t, m, "live_subscription"))
	})
}

func TestBusInformerMetrics_PrometheusCollectors(t *testing.T) {
	assert.Len(t, newTestBusMetrics().PrometheusCollectors(), 10)
	assert.Nil(t, (*BusInformerMetrics)(nil).PrometheusCollectors())

	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{})
	assert.Nil(t, inf.PrometheusCollectors(), "an informer with no metrics exposes no collectors")
}

func TestRelistRecorder_ZeroValueDiscards(t *testing.T) {
	var recorder RelistRecorder
	recorder.Request()
	recorder.Projection(ProjectionIdentities)
	assert.Equal(t, RelistRecorder{}, (*BusInformerMetrics)(nil).RelistRecorder(busTestKind))
}

func TestRelistRecorder_RecordsRequestsAndProjection(t *testing.T) {
	m := newTestBusMetrics()
	recorder := m.RelistRecorder(busTestKind)

	recorder.Request()
	recorder.Request()
	recorder.Projection(ProjectionIdentities)
	recorder.Projection(ProjectionObjects)

	assert.Equal(t, float64(2), busMetricValue(t, m, "relist_requests"))
	assert.Equal(t, float64(1), busMetricValue(t, m, "relist_projection", string(ProjectionIdentities)))
	assert.Equal(t, float64(1), busMetricValue(t, m, "relist_projection", string(ProjectionObjects)))
}

// A watcher can tell a live event from a re-list one, which instrumentation that attributes an event to its cause
// needs and a watcher that does not care never looks at.
func TestBusInformer_DispatchesWithTheDeliveryOnTheContext(t *testing.T) {
	var deliveries []Delivery
	record := &SimpleWatcher{
		AddFunc: func(ctx context.Context, _ resource.Object) error {
			deliveries = append(deliveries, DeliveryFromContext(ctx))
			return nil
		},
		UpdateFunc: func(ctx context.Context, _, _ resource.Object) error {
			deliveries = append(deliveries, DeliveryFromContext(ctx))
			return nil
		},
		DeleteFunc: func(ctx context.Context, _ resource.Object) error {
			deliveries = append(deliveries, DeliveryFromContext(ctx))
			return nil
		},
	}

	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObj("a")}, "", nil
		}
		return nil, "", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, record, BusInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "live", "11"))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "live", "12"))
	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "live", "13"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []Delivery{
		DeliveryRelist, // the initial list's add
		DeliveryLive,   // the live add
		DeliveryLive,   // the live update
		DeliveryLive,   // the live delete
		DeliveryRelist, // the re-list's delete of "a"
	}, deliveries)
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
		m := newTestBusMetrics()
		watcher := &recordingWatcher{}
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "300", nil })
		inf := newBusInformer(t, resourcebus.NewFake(), lister, watcher, BusInformerOptions{Metrics: m})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "200"))

		assert.Empty(t, watcher.addedNames(), "the list read at 300 already accounts for the add at 200")
		assert.Empty(t, inf.store.List())
		assert.Equal(t, float64(1), busMetricValue(t, m, "superseded_events"))
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
		m := newTestBusMetrics()
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), &recordingWatcher{}, BusInformerOptions{
			EventQueueSize:         1,
			OverflowRelistInterval: time.Minute,
			Metrics:                m,
		})

		// Nothing drains the queue, so every event after the first overflows.
		for i := range 50 {
			inf.onEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", strconv.Itoa(i)))
		}

		assert.Equal(t, float64(49), busMetricValue(t, m, "dropped_events"), "every drop is still counted")
		assert.Len(t, inf.relistSignal, 1, "but they coalesce into one re-list request")

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
		m := newTestBusMetrics()
		var lists atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "", nil
		})
		inf := newBusInformer(t, sub, lister, &recordingWatcher{}, BusInformerOptions{
			ResyncInterval:           time.Hour,
			AllowDegradedStart:       true,
			SubscriptionReadyTimeout: 5 * time.Second,
			Metrics:                  m,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, inf.HasSynced(), "a degraded start still lists")

		// The first retry is accepted but never becomes ready, so the gauge stays closed.
		time.Sleep(DefaultBusRetryInterval + 5*time.Second + time.Second)
		synctest.Wait()
		assert.Equal(t, float64(0), busMetricValue(t, m, "live_subscription"),
			"a subscription which cannot deliver is not an open one")

		// The next retry is ready, which opens the gauge and forces the gap-closing re-list.
		before := lists.Load()
		time.Sleep(DefaultBusRetryInterval + time.Second)
		synctest.Wait()
		assert.Equal(t, float64(1), busMetricValue(t, m, "live_subscription"))
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

// What the bus announces decides whether an identity payload is enough for an opinionated reconciler or watcher,
// since the SDK cannot see the allowlist the resource owner registered.
func TestBusInformer_DeliversOpinionatedMetadata(t *testing.T) {
	complete := AnnouncedMetadata{DeletionTimestamp: true, Finalizers: true, Generation: true}

	for _, tt := range []struct {
		name string
		opts BusInformerOptions
		want bool
	}{
		{"identity with nothing announced", BusInformerOptions{}, false},
		{"identity missing finalizers", BusInformerOptions{
			AnnouncedMetadata: AnnouncedMetadata{DeletionTimestamp: true, Generation: true},
		}, false},
		{"identity missing the deletion timestamp", BusInformerOptions{
			AnnouncedMetadata: AnnouncedMetadata{Finalizers: true, Generation: true},
		}, false},
		{"identity with all three announced", BusInformerOptions{AnnouncedMetadata: complete}, true},
		{"hydrated carries the whole object", BusInformerOptions{
			Payload: PayloadHydrated,
			Getter:  &fakeGetter{objects: map[string]resource.Object{}},
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inf := newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, tt.opts)
			assert.Equal(t, tt.want, inf.DeliversOpinionatedMetadata())
		})
	}
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
