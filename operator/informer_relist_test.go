package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

var relistTestKind = resource.Kind{
	Schema: resource.NewSimpleSchema("example.grafana.app", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind("Widget"), resource.WithPlural("widgets")),
}

// listerFunc adapts a function to ResourceLister.
type listerFunc func(ctx context.Context) ([]resource.Object, string, error)

func (f listerFunc) List(ctx context.Context) ([]resource.Object, string, error) { return f(ctx) }

func staticLister(objs ...resource.Object) ResourceLister {
	return listerFunc(func(context.Context) ([]resource.Object, string, error) { return objs, "", nil })
}

// identityStaticLister is a staticLister which declares the identity payload.
type identityStaticLister struct{ ResourceLister }

func (identityStaticLister) IdentityPayload() bool { return true }

// recordingWatcher captures every dispatch an informer makes.
type recordingWatcher struct {
	mu      sync.Mutex
	adds    []resource.Object
	updates []resource.Object
	deletes []resource.Object
	addErr  error
}

func (w *recordingWatcher) Add(_ context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.adds = append(w.adds, obj)
	return w.addErr
}

func (w *recordingWatcher) Update(_ context.Context, _, tgt resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
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
	return relistNames(w.adds)
}

func (w *recordingWatcher) updatedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return relistNames(w.updates)
}

func (w *recordingWatcher) deletedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return relistNames(w.deletes)
}

func (w *recordingWatcher) added() []resource.Object {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]resource.Object(nil), w.adds...)
}

var _ ResourceWatcher = (*recordingWatcher)(nil)

// fakeSource is a ChangeSource whose watches a test drives by hand.
type fakeSource struct {
	mu       sync.Mutex
	watches  []*fakeWatch
	failN    int
	identity bool
}

var _ ChangeSource = (*fakeSource)(nil)

func (s *fakeSource) Watch(context.Context, string, resource.WatchOptions) (resource.WatchResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failN > 0 {
		s.failN--
		return nil, errors.New("source unavailable")
	}
	w := &fakeWatch{ch: make(chan resource.WatchEvent), stopped: make(chan struct{})}
	s.watches = append(s.watches, w)
	return w, nil
}

func (s *fakeSource) IdentityPayload() bool { return s.identity }

func (s *fakeSource) opened() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.watches)
}

func (s *fakeSource) current() *fakeWatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[len(s.watches)-1]
}

type fakeWatch struct {
	ch       chan resource.WatchEvent
	stopOnce sync.Once
	stopped  chan struct{}
}

func (w *fakeWatch) WatchEvents() <-chan resource.WatchEvent { return w.ch }

func (w *fakeWatch) Stop() { w.stopOnce.Do(func() { close(w.stopped) }) }

// send delivers evt, blocking until the informer takes it or stops the watch.
func (w *fakeWatch) send(evt resource.WatchEvent) {
	select {
	case w.ch <- evt:
	case <-w.stopped:
	}
}

// end closes the watch from the source's side, as a server timeout or a lost bus subscription does.
func (w *fakeWatch) end() { close(w.ch) }

func liveEvent(eventType, name, rv string) resource.WatchEvent {
	return resource.WatchEvent{
		EventType: eventType,
		Object:    NewIdentityObject(relistTestKind, relistTestNamespace, name, rv),
	}
}

func busEvent(verb resourcebus.Verb, name, rv string) resourcebus.Event {
	return resourcebus.Event{Verb: verb, Namespace: relistTestNamespace, Name: name, ResourceVersion: rv}
}

func relistTestGVR() schema.GroupVersionResource { return relistTestKind.GroupVersionResource() }

// startInformer runs an informer inside the current synctest bubble and lets the run loop quiesce, so it has opened
// its watch and finished its initial list. The returned stop func must be deferred: the bubble requires every
// goroutine it spawned to have exited before the test body returns.
func startInformer(t *testing.T, inf *RelistInformer) func() {
	t.Helper()
	stop := runInformer(inf)
	// Wait blocks until the run goroutine is durably blocked, i.e. parked in the main select after opening the watch
	// and the initial list. None of that needs the fake clock to advance.
	synctest.Wait()
	require.True(t, inf.HasSynced(), "informer never synced")
	return stop
}

// runInformer runs an informer without waiting for it to sync.
func runInformer(inf *RelistInformer) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = inf.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
		synctest.Wait()
	}
}

func newRelistInformer(t *testing.T, lister ResourceLister, source ChangeSource, watcher ResourceWatcher, opts RelistInformerOptions) *RelistInformer {
	t.Helper()
	if opts.ListWatchOptions.Namespace == "" {
		opts.ListWatchOptions.Namespace = relistTestNamespace
	}
	inf, err := NewRelistInformer(relistTestKind, lister, source, opts)
	require.NoError(t, err)
	if watcher != nil {
		require.NoError(t, inf.AddEventHandler(watcher))
	}
	return inf
}

func TestNewRelistInformer_Validation(t *testing.T) {
	lister := staticLister()

	_, err := NewRelistInformer(resource.Kind{}, lister, nil, RelistInformerOptions{})
	assert.Error(t, err, "an empty kind is rejected")

	_, err = NewRelistInformer(relistTestKind, nil, nil, RelistInformerOptions{})
	assert.Error(t, err, "a nil lister is rejected")

	inf, err := NewRelistInformer(relistTestKind, lister, nil, RelistInformerOptions{})
	require.NoError(t, err, "a nil source is re-list only")
	assert.False(t, inf.IdentityPayload(), "full objects from a lister which declares nothing")
	assert.Equal(t, relistTestKind.Schema, inf.Schema())
	assert.Error(t, inf.AddEventHandler(nil))
}

// The informer's payload is identity if either the source or the lister declares it, since both deliver to the same
// watchers.
func TestRelistInformer_IdentityPayload(t *testing.T) {
	for _, tt := range []struct {
		name     string
		source   ChangeSource
		lister   ResourceLister
		identity bool
	}{
		{name: "full source, full lister", source: &fakeSource{}, lister: staticLister()},
		{name: "identity source", source: &fakeSource{identity: true}, lister: staticLister(), identity: true},
		{name: "identity lister", source: &fakeSource{}, lister: identityStaticLister{staticLister()}, identity: true},
		{name: "identity lister, no source", lister: identityStaticLister{staticLister()}, identity: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inf := newRelistInformer(t, tt.lister, tt.source, nil, RelistInformerOptions{})
			assert.Equal(t, tt.identity, inf.IdentityPayload())
		})
	}
}

// The initial list drives an add per existing object and marks the informer synced, so a consumer can start
// reconciling the full set.
func TestRelistInformer_InitialListDeliversAdds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &fakeSource{}
		watcher := &recordingWatcher{}
		inf := newRelistInformer(t, staticLister(relistObj("a"), relistObj("b")), source, watcher, RelistInformerOptions{})

		stop := startInformer(t, inf)
		defer stop()

		assert.Equal(t, 1, source.opened(), "the watch opens before the list")
		assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames())
	})
}

// Without a source the informer is re-list only: it syncs from the list, and the cadence diffs the next one.
func TestRelistInformer_NoSourceRelistsOnCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if calls.Add(1) == 1 {
				return []resource.Object{relistObj("a"), relistObj("b")}, "", nil
			}
			return []resource.Object{relistObj("a")}, "", nil
		})
		inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{ResyncInterval: time.Minute})
		stop := startInformer(t, inf)
		defer stop()

		// The jitter spreads each interval by up to 10%, so sleeping past the longest possible one guarantees
		// exactly one more pass.
		time.Sleep(time.Minute + 7*time.Second)
		synctest.Wait()

		assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames())
		assert.Equal(t, []string{"a"}, watcher.updatedNames())
		assert.Equal(t, []string{"b"}, watcher.deletedNames())
	})
}

// A live added goes through Add, a modified through Update, and a deleted through Delete. Through the bus's identity
// payload the delivered object carries the event's identity and nothing else.
func TestRelistInformer_BusLiveEventsDispatchIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		watcher := &recordingWatcher{}
		inf := newRelistInformer(t, staticLister(), newBusSource(t, sub, BusWatchOptions{}, nil), watcher, RelistInformerOptions{})
		stop := startInformer(t, inf)
		defer stop()

		ctx := context.Background()
		require.NoError(t, sub.Publish(ctx, relistTestGVR(), relistTestNamespace, busEvent(resourcebus.VerbAdded, "fresh", "11")))
		require.NoError(t, sub.Publish(ctx, relistTestGVR(), relistTestNamespace, busEvent(resourcebus.VerbModified, "changed", "12")))
		require.NoError(t, sub.Publish(ctx, relistTestGVR(), relistTestNamespace, busEvent(resourcebus.VerbDeleted, "gone", "13")))
		synctest.Wait()

		assert.Equal(t, []string{"fresh"}, watcher.addedNames())
		assert.Equal(t, []string{"changed"}, watcher.updatedNames())
		assert.Equal(t, []string{"gone"}, watcher.deletedNames())

		delivered := watcher.added()[0]
		assert.Equal(t, relistTestNamespace, delivered.GetNamespace())
		assert.Equal(t, "11", delivered.GetResourceVersion())
		assert.Equal(t, relistTestKind.GroupVersionKind(), delivered.GroupVersionKind())
	})
}

// A live delete drops the object from the snapshot, and a live add writes through to it, so the next re-list diffs
// against what has actually been delivered.
func TestRelistInformer_LiveEventsWriteThroughToSnapshot(t *testing.T) {
	inf := newRelistInformer(t, staticLister(relistObj("keep"), relistObj("gone")), nil, &recordingWatcher{}, RelistInformerOptions{})
	ctx := context.Background()
	require.NoError(t, inf.relist(ctx, initialRelist))

	inf.onEvent(ctx, liveEvent(watchEventDeleted, "gone", "13"))
	inf.onEvent(ctx, liveEvent(watchEventAdded, "fresh", "14"))

	assert.ElementsMatch(t, []string{"keep", "fresh"}, relistNames(inf.store.List()))
}

// A full-object modification is written through, but an identity one is not: it would overwrite the stored object's
// real deletion timestamp and the last-known state a delete delivers.
func TestRelistInformer_LiveModificationWriteThrough(t *testing.T) {
	listed := relistObj("x")
	listed.Spec = map[string]any{"field": "value"}

	t.Run("a full object is written through", func(t *testing.T) {
		inf := newRelistInformer(t, staticLister(listed), &fakeSource{}, &recordingWatcher{}, RelistInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.onEvent(context.Background(), liveEvent(watchEventModified, "x", "300"))

		stored, ok := inf.store.Get(relistTestNamespace, "x")
		require.True(t, ok)
		assert.Equal(t, "300", stored.GetResourceVersion())
	})

	t.Run("an identity object is not", func(t *testing.T) {
		inf := newRelistInformer(t, staticLister(listed), &fakeSource{identity: true}, &recordingWatcher{}, RelistInformerOptions{})
		require.NoError(t, inf.relist(context.Background(), initialRelist))

		inf.onEvent(context.Background(), liveEvent(watchEventModified, "x", "300"))

		stored, ok := inf.store.Get(relistTestNamespace, "x")
		require.True(t, ok)
		assert.Equal(t, map[string]any{"field": "value"}, stored.GetSpec())
	})
}

// A live delete delivers the last-known object when the snapshot still holds one, so a watcher which reads the
// object on delete is not handed a bare identity.
func TestRelistInformer_LiveDeleteDeliversLastKnownObject(t *testing.T) {
	watcher := &recordingWatcher{}
	listed := relistObj("gone")
	listed.Spec = map[string]any{"field": "value"}
	inf := newRelistInformer(t, staticLister(listed), nil, watcher, RelistInformerOptions{})
	require.NoError(t, inf.relist(context.Background(), initialRelist))

	inf.onEvent(context.Background(), liveEvent(watchEventDeleted, "gone", "13"))

	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	require.Len(t, watcher.deletes, 1)
	assert.Equal(t, map[string]any{"field": "value"}, watcher.deletes[0].GetSpec())
}

// An unknown event type, or one with no object, is reported to the error handler and skipped; a bookmark is skipped
// silently.
func TestRelistInformer_SkipsUndeliverableEvents(t *testing.T) {
	watcher := &recordingWatcher{}
	var handled []string
	inf := newRelistInformer(t, staticLister(), nil, watcher, RelistInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err.Error()) },
	})

	ctx := context.Background()
	inf.onEvent(ctx, liveEvent("nonsense", "x", "1"))
	inf.onEvent(ctx, resource.WatchEvent{EventType: watchEventAdded})
	inf.onEvent(ctx, liveEvent(watchEventBookmark, "x", "1"))
	inf.onEvent(ctx, liveEvent(watchEventModified, "survivor", "2"))

	assert.Equal(t, []string{"survivor"}, watcher.updatedNames())
	require.Len(t, handled, 2)
	assert.Contains(t, handled[0], "unknown type")
	assert.Contains(t, handled[1], "no object")
}

// An error from a watcher reaches the error handler without stopping delivery to the rest.
func TestRelistInformer_WatcherErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	first := &recordingWatcher{addErr: errors.New("boom")}
	second := &recordingWatcher{}
	inf := newRelistInformer(t, staticLister(relistObj("a")), nil, first, RelistInformerOptions{
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
func TestRelistInformer_RelistDiffEmitsDeletes(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{relistObj("a"), relistObj("b")}, "", nil
		}
		return []resource.Object{relistObj("a")}, "", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames())
	assert.Equal(t, []string{"b"}, watcher.deletedNames(), "a vanished object must be delivered as a delete")
}

// An object first seen on a re-list rather than the initial list is delivered as an add, not an update, so an
// add-only watcher wakes for it.
func TestRelistInformer_RelistEmitsAddForNewKeys(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{relistObj("a")}, "", nil
		}
		return []resource.Object{relistObj("a"), relistObj("b")}, "", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.ElementsMatch(t, []string{"a", "b"}, watcher.addedNames(), "b must be an add, not an update")
	assert.Equal(t, []string{"a"}, watcher.updatedNames(), "the already-known object is an update")
	assert.Empty(t, watcher.deletedNames())
}

// A failing re-list is reported and returns an error, leaving the snapshot untouched so the next tick retries
// against the same baseline instead of diffing a partial set as a mass deletion.
func TestRelistInformer_FailedRelistLeavesSnapshotIntact(t *testing.T) {
	watcher := &recordingWatcher{}
	var fail atomic.Bool
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		if fail.Load() {
			return nil, "", errors.New("api unavailable")
		}
		return []resource.Object{relistObj("a")}, "", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{
		ErrorHandler: func(context.Context, error) {},
	})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	fail.Store(true)
	require.Error(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"a"}, relistNames(inf.store.List()))
	assert.Empty(t, watcher.deletedNames())
}

// countingLister lists a fixed set and counts the calls.
func countingLister(calls *atomic.Int64, objs ...resource.Object) ResourceLister {
	return listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls.Add(1)
		return objs, "", nil
	})
}

// A watch which ends is a gap: the informer re-opens it and re-lists to cover whatever changed in between, without
// waiting for the re-list cadence. An ERROR event ends a watch the same way.
func TestRelistInformer_EndedWatchReopensAndRelists(t *testing.T) {
	for _, tt := range []struct {
		name string
		end  func(w *fakeWatch)
	}{
		{name: "closed channel", end: (*fakeWatch).end},
		{name: "error event", end: func(w *fakeWatch) { w.send(resource.WatchEvent{EventType: watchEventError}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				source := &fakeSource{}
				var calls atomic.Int64
				// An hour-long cadence guarantees only the gap can trigger the second list.
				inf := newRelistInformer(t, countingLister(&calls, relistObj("a")), source, &recordingWatcher{},
					RelistInformerOptions{ResyncInterval: time.Hour})
				stop := startInformer(t, inf)
				defer stop()
				require.Equal(t, int64(1), calls.Load())

				tt.end(source.current())
				time.Sleep(DefaultRelistRetryInterval)
				synctest.Wait()

				assert.Equal(t, 2, source.opened(), "the watch must be re-opened")
				assert.Equal(t, int64(2), calls.Load(), "the re-open must trigger a re-list")

				watcher := &recordingWatcher{}
				require.NoError(t, inf.AddEventHandler(watcher))
				source.current().send(liveEvent(watchEventAdded, "fresh", "11"))
				synctest.Wait()
				assert.Equal(t, []string{"fresh"}, watcher.addedNames(), "the re-opened watch is served")
			})
		})
	}
}

// A bus reconnect ends the bus watch, which drives the same re-open and re-list.
func TestRelistInformer_BusReconnectTriggersRelist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		var calls atomic.Int64
		inf := newRelistInformer(t, countingLister(&calls, relistObj("a")), newBusSource(t, sub, BusWatchOptions{}, nil),
			&recordingWatcher{}, RelistInformerOptions{ResyncInterval: time.Hour})
		stop := startInformer(t, inf)
		defer stop()

		require.Equal(t, int64(1), calls.Load(), "the subscription opens before the list, so startup lists once")

		sub.Reconnect(relistTestGVR(), relistTestNamespace)
		time.Sleep(DefaultRelistRetryInterval)
		synctest.Wait()

		assert.True(t, sub.Subscribed(relistTestGVR(), relistTestNamespace), "the informer must subscribe again")
		assert.Equal(t, int64(2), calls.Load(), "a reconnect must trigger a re-list")
	})
}

// A failing watch gates startup by default: the informer retries and only lists, and reports synced, once it opens,
// then delivers live events.
func TestRelistInformer_RetriesWatchUntilAvailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailSubscribes(2)
		watcher := &recordingWatcher{}
		inf := newRelistInformer(t, staticLister(relistObj("a")), newBusSource(t, sub, BusWatchOptions{}, nil), watcher,
			RelistInformerOptions{ResyncInterval: time.Hour})

		stop := runInformer(inf)
		defer stop()

		synctest.Wait()
		require.False(t, sub.Subscribed(relistTestGVR(), relistTestNamespace), "must not subscribe while Subscribe fails")
		require.False(t, inf.HasSynced(), "must not sync before the watch opens")

		for range 5 {
			if sub.Subscribed(relistTestGVR(), relistTestNamespace) {
				break
			}
			time.Sleep(DefaultRelistRetryInterval)
			synctest.Wait()
		}
		require.True(t, sub.Subscribed(relistTestGVR(), relistTestNamespace), "the retry must eventually subscribe")
		require.True(t, inf.HasSynced(), "must sync once subscribed")
		assert.Equal(t, []string{"a"}, watcher.addedNames(), "the initial list must be delivered")

		require.NoError(t, sub.Publish(context.Background(), relistTestGVR(), relistTestNamespace,
			busEvent(resourcebus.VerbAdded, "fresh", "11")))
		synctest.Wait()
		assert.Equal(t, []string{"a", "fresh"}, watcher.addedNames())
	})
}

// With a degraded start a failing watch no longer gates startup: the informer lists, reports synced, and keeps
// re-listing while the watch retries in the background. Once it opens, a re-list covers the gap.
func TestRelistInformer_DegradedStartListsWithoutWatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		sub.FailSubscribes(2)
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		inf := newRelistInformer(t, countingLister(&calls, relistObj("a")), newBusSource(t, sub, BusWatchOptions{}, nil),
			watcher, RelistInformerOptions{ResyncInterval: time.Hour, AllowDegradedStart: true})

		stop := runInformer(inf)
		defer stop()

		synctest.Wait()
		require.False(t, sub.Subscribed(relistTestGVR(), relistTestNamespace), "the subscription is still failing")
		require.True(t, inf.HasSynced(), "a degraded start must sync from the initial list")
		assert.Equal(t, []string{"a"}, watcher.addedNames())

		before := calls.Load()
		for range 5 {
			if sub.Subscribed(relistTestGVR(), relistTestNamespace) {
				break
			}
			time.Sleep(DefaultRelistRetryInterval)
			synctest.Wait()
		}
		require.True(t, sub.Subscribed(relistTestGVR(), relistTestNamespace), "the background retry must eventually subscribe")
		assert.Greater(t, calls.Load(), before, "opening the watch must trigger a gap-closing re-list")

		require.NoError(t, sub.Publish(context.Background(), relistTestGVR(), relistTestNamespace,
			busEvent(resourcebus.VerbAdded, "fresh", "11")))
		synctest.Wait()
		assert.Equal(t, []string{"a", "fresh"}, watcher.addedNames())
	})
}

// A failing initial list must not mark the informer synced: WaitForSync keeps blocking until the first list
// succeeds, then the listed objects are delivered.
func TestRelistInformer_DoesNotSyncUntilInitialListSucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &fakeSource{}
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if calls.Add(1) < 3 {
				return nil, "", errors.New("api unavailable")
			}
			return []resource.Object{relistObj("a")}, "", nil
		})
		inf := newRelistInformer(t, lister, source, watcher, RelistInformerOptions{
			ResyncInterval: time.Hour,
			ErrorHandler:   func(context.Context, error) {},
		})

		stop := runInformer(inf)
		defer stop()

		ctx := context.Background()
		synctest.Wait()
		require.Equal(t, 1, source.opened())
		require.False(t, inf.HasSynced(), "must not sync while the initial list fails")
		require.Error(t, inf.HealthCheck(ctx))

		for range 5 {
			if inf.HasSynced() {
				break
			}
			time.Sleep(DefaultRelistRetryInterval)
			synctest.Wait()
		}
		require.True(t, inf.HasSynced(), "must sync after the initial list succeeds")
		require.NoError(t, inf.HealthCheck(ctx))
		assert.Equal(t, []string{"a"}, watcher.addedNames())
	})
}

// WaitForSync returns the context's error rather than blocking forever when the informer never syncs.
func TestRelistInformer_WaitForSyncHonorsContext(t *testing.T) {
	inf := newRelistInformer(t, staticLister(), nil, nil, RelistInformerOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, inf.WaitForSync(ctx), context.Canceled)
}

// signalGap never blocks: a pending signal coalesces further gaps, so a burst while the run loop is busy cannot
// deadlock the watch goroutine.
func TestRelistInformer_SignalGapDoesNotBlock(t *testing.T) {
	inf := newRelistInformer(t, staticLister(), nil, nil, RelistInformerOptions{})
	for range 100 {
		inf.signalGap()
	}
}

// A live add is written to the snapshot, so the next re-list re-delivers it as a retained update rather than
// re-reporting a delivered add as a change the live stream missed.
func TestRelistInformer_LiveAddNotReCountedAsRelistRecovery(t *testing.T) {
	watcher := &recordingWatcher{}
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return nil, "", nil
		}
		return []resource.Object{relistObj("x")}, "", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.onEvent(context.Background(), liveEvent(watchEventAdded, "x", "55"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "the live add is the only add")
	assert.Equal(t, []string{"x"}, watcher.updatedNames(), "the re-list re-delivers it as an update")
	assert.Empty(t, watcher.deletedNames())
}

// A live add which lands after a re-list read its snapshot must not be diffed as a delete: the snapshot's silence
// about it is stale, so the store carries it forward until a snapshot which postdates it.
func TestRelistInformer_LiveAddNewerThanRelistSnapshotIsNotDeleted(t *testing.T) {
	watcher := &recordingWatcher{}
	// Every list is read at 150 and omits "x"; "x" is live-added at 200, after that snapshot.
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "150", nil })
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.onEvent(context.Background(), liveEvent(watchEventAdded, "x", "200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "the live add is the only add")
	assert.Empty(t, watcher.deletedNames(), "a live add newer than the snapshot must not be diffed as a delete")
	assert.Equal(t, []string{"x"}, relistNames(inf.store.List()), "x is carried forward")
}

// A live delete which lands after a re-list read its snapshot must not be resurrected: the store suppresses the
// stale re-add rather than dispatching a spurious add for a just-deleted object.
func TestRelistInformer_LiveDeleteNewerThanRelistSnapshotIsNotResurrected(t *testing.T) {
	watcher := &recordingWatcher{}
	// Every list is read at 150 and still lists "x" at 120; "x" is live-deleted at 200.
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return []resource.Object{relistObjRV("x", 120)}, "150", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.onEvent(context.Background(), liveEvent(watchEventDeleted, "x", "200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x"}, watcher.addedNames(), "x is added once, by the initial list")
	assert.Equal(t, []string{"x"}, watcher.deletedNames(), "the live delete is delivered")
	assert.Empty(t, watcher.updatedNames(), "the stale re-list must not re-deliver the deleted object")
	assert.Empty(t, inf.store.List(), "x stays evicted")
}

// With opaque resource versions the store's arbitration is off, so a live delete racing a re-list produces a
// spurious add/delete pair — the documented degradation — which the next re-list corrects.
func TestRelistInformer_OpaqueResourceVersionsDegradeToSpuriousPair(t *testing.T) {
	watcher := &recordingWatcher{}
	listed := relistObj("x")
	listed.ResourceVersion = "opaque-120"
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls <= 2 {
			return []resource.Object{listed}, "opaque-150", nil
		}
		return nil, "opaque-250", nil
	})
	inf := newRelistInformer(t, lister, nil, watcher, RelistInformerOptions{})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	inf.onEvent(context.Background(), liveEvent(watchEventDeleted, "x", "opaque-200"))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, []string{"x", "x"}, watcher.addedNames(), "the stale snapshot re-adds the deleted object")
	assert.Equal(t, []string{"x"}, watcher.deletedNames())

	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	assert.Equal(t, []string{"x", "x"}, watcher.deletedNames(), "the next re-list corrects it")
	assert.Empty(t, inf.store.List())
}

func newTestRelistMetrics() *RelistInformerMetrics {
	return NewRelistInformerMetrics(RelistInformerMetricsConfig{
		MetricsConfig:       metrics.DefaultConfig("test"),
		ResourceVersionTime: func(rv int64) time.Time { return time.UnixMilli(rv) },
	})
}

func relistMetricValue(t *testing.T, m *RelistInformerMetrics, name string, labels ...string) float64 {
	t.Helper()
	switch name {
	case "live_events":
		return testutil.ToFloat64(m.liveEvents.WithLabelValues(labels...))
	case "relist_events":
		return testutil.ToFloat64(m.relistEvents.WithLabelValues(labels...))
	case "rewatches":
		return testutil.ToFloat64(m.rewatches.WithLabelValues(labels...))
	case "live_watch":
		return testutil.ToFloat64(m.liveWatch.WithLabelValues(labels...))
	case "relist_requests":
		return testutil.ToFloat64(m.relistRequests.WithLabelValues(labels...))
	case "relist_projection":
		return testutil.ToFloat64(m.relistProjection.WithLabelValues(labels...))
	}
	t.Fatalf("unknown metric %q", name)
	return 0
}

// Live and re-list deliveries are counted in separate families: a live event is a delivery, a re-list add or delete
// is a change the live stream did not deliver here.
func TestRelistInformer_MetricsSeparateLiveFromRelist(t *testing.T) {
	m := newTestRelistMetrics()
	kindLabel := relistTestKind.GroupVersionKind().String()
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		switch calls {
		case 1:
			return []resource.Object{relistObjRV("a", 11)}, "100", nil
		case 2:
			return []resource.Object{relistObjRV("a", 11), relistObjRV("b", 22)}, "100", nil
		default:
			return []resource.Object{relistObjRV("a", 11)}, "100", nil
		}
	})
	inf := newRelistInformer(t, lister, nil, &recordingWatcher{}, RelistInformerOptions{Metrics: m})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	assert.Equal(t, float64(2), relistMetricValue(t, m, "relist_events", kindLabel, relistVerbAdd), "a on the initial list, b recovered")
	assert.Equal(t, float64(2), relistMetricValue(t, m, "relist_events", kindLabel, relistVerbUpdate), "a retained twice")
	assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_events", kindLabel, relistVerbDelete), "b vanished")
	assert.Zero(t, relistMetricValue(t, m, "live_events", kindLabel, relistVerbAdd), "re-list deliveries are not live events")

	for _, eventType := range []string{watchEventAdded, watchEventModified, watchEventDeleted} {
		inf.onEvent(context.Background(), liveEvent(eventType, "x", "101"))
	}
	assert.Equal(t, float64(1), relistMetricValue(t, m, "live_events", kindLabel, relistVerbAdd))
	assert.Equal(t, float64(1), relistMetricValue(t, m, "live_events", kindLabel, relistVerbUpdate))
	assert.Equal(t, float64(1), relistMetricValue(t, m, "live_events", kindLabel, relistVerbDelete))
}

// Latency is only observed where the version dates the change: not for the initial list, not for a re-delivery of an
// unchanged object, and not for a re-list-detected delete whose last-known version predates it.
func TestRelistInformer_MetricsLatencyOnlyWhereVersionDatesTheChange(t *testing.T) {
	m := newTestRelistMetrics()
	kindLabel := relistTestKind.GroupVersionKind().String()
	recent := time.Now().Add(-time.Second).UnixMilli()
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{relistObjRV("a", recent)}, "1", nil
		}
		return []resource.Object{relistObjRV("a", recent), relistObjRV("b", recent)}, "1", nil
	})
	inf := newRelistInformer(t, lister, nil, &recordingWatcher{}, RelistInformerOptions{Metrics: m})

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	assert.Zero(t, testutil.CollectAndCount(m.relistLatency), "the initial list carries no latency")

	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	assert.Equal(t, 1, testutil.CollectAndCount(m.relistLatency), "only the recovered add is timed")

	inf.onEvent(context.Background(), liveEvent(watchEventAdded, "x", "opaque"))
	assert.Zero(t, testutil.CollectAndCount(m.liveLatency), "an opaque version cannot date the change")
	assert.Equal(t, float64(1), relistMetricValue(t, m, "live_events", kindLabel, relistVerbAdd), "but the event is still counted")

	inf.onEvent(context.Background(), liveEvent(watchEventAdded, "y", fmt.Sprint(recent)))
	assert.Equal(t, 1, testutil.CollectAndCount(m.liveLatency))
}

// The watch gauge tracks what the informer can see: open once the watch opens, closed while it is re-opened after a
// gap, open again with the re-watch counted, and closed once the informer stops.
func TestRelistInformer_MetricsObserveWatchAndRewatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestRelistMetrics()
		kindLabel := relistTestKind.GroupVersionKind().String()
		source := &fakeSource{}
		inf := newRelistInformer(t, staticLister(), source, &recordingWatcher{}, RelistInformerOptions{
			ResyncInterval: time.Hour,
			Metrics:        m,
		})

		stop := startInformer(t, inf)
		assert.Equal(t, float64(1), relistMetricValue(t, m, "live_watch", kindLabel))

		source.current().end()
		synctest.Wait()
		assert.Equal(t, float64(0), relistMetricValue(t, m, "live_watch", kindLabel), "no watch while re-opening")

		time.Sleep(DefaultRelistRetryInterval)
		synctest.Wait()
		assert.Equal(t, float64(1), relistMetricValue(t, m, "live_watch", kindLabel))
		assert.Equal(t, float64(1), relistMetricValue(t, m, "rewatches", kindLabel))

		stop()
		assert.Equal(t, float64(0), relistMetricValue(t, m, "live_watch", kindLabel))
	})
}

// A degraded start holds the gauge closed while it serves re-lists, and opens it once the background retry succeeds.
func TestRelistInformer_MetricsDegradedStartGauge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestRelistMetrics()
		kindLabel := relistTestKind.GroupVersionKind().String()
		source := &fakeSource{failN: 1}
		inf := newRelistInformer(t, staticLister(), source, &recordingWatcher{}, RelistInformerOptions{
			ResyncInterval:     time.Hour,
			AllowDegradedStart: true,
			Metrics:            m,
		})

		stop := runInformer(inf)
		defer stop()

		synctest.Wait()
		require.True(t, inf.HasSynced(), "degraded start syncs from the list alone")
		assert.Equal(t, float64(0), relistMetricValue(t, m, "live_watch", kindLabel))

		time.Sleep(DefaultRelistRetryInterval)
		synctest.Wait()
		require.Equal(t, 1, source.opened())
		assert.Equal(t, float64(1), relistMetricValue(t, m, "live_watch", kindLabel))
	})
}

func TestRelistInformerMetrics_PrometheusCollectors(t *testing.T) {
	assert.Len(t, newTestRelistMetrics().PrometheusCollectors(), 8)
	assert.Nil(t, (*RelistInformerMetrics)(nil).PrometheusCollectors())

	inf := newRelistInformer(t, staticLister(), nil, nil, RelistInformerOptions{})
	assert.Nil(t, inf.PrometheusCollectors(), "an informer with no metrics exposes no collectors")
}

func TestRelistRecorder_ZeroValueDiscards(t *testing.T) {
	var recorder RelistRecorder
	recorder.Request()
	recorder.Projection(ProjectionIdentities)
	assert.Equal(t, RelistRecorder{}, (*RelistInformerMetrics)(nil).RelistRecorder(relistTestKind))
}

func TestRelistRecorder_RecordsRequestsAndProjection(t *testing.T) {
	m := newTestRelistMetrics()
	kindLabel := relistTestKind.GroupVersionKind().String()
	recorder := m.RelistRecorder(relistTestKind)

	recorder.Request()
	recorder.Request()
	recorder.Projection(ProjectionIdentities)
	recorder.Projection(ProjectionObjects)

	assert.Equal(t, float64(2), relistMetricValue(t, m, "relist_requests", kindLabel))
	assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_projection", kindLabel, string(ProjectionIdentities)))
	assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_projection", kindLabel, string(ProjectionObjects)))
}
