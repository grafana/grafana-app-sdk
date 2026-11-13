package operator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

const listerTestNamespace = "default"

var listerTestKind = resource.Kind{
	Schema: resource.NewSimpleSchema("example.grafana.app", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind("Widget"), resource.WithPlural("widgets")),
}

func listerObj(name string) *resource.UntypedObject {
	return &resource.UntypedObject{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: listerTestNamespace}}
}

func listerNames(objs []resource.Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.GetName()
	}
	return out
}

// listerFunc adapts a function to ResourceLister.
type listerFunc func(ctx context.Context) ([]resource.Object, string, error)

func (f listerFunc) List(ctx context.Context) ([]resource.Object, string, error) { return f(ctx) }

func staticLister(objs ...resource.Object) ResourceLister {
	return listerFunc(func(context.Context) ([]resource.Object, string, error) { return objs, "", nil })
}

// addRecordingWatcher captures every add a PeriodicInformer dispatches.
type addRecordingWatcher struct {
	mu     sync.Mutex
	adds   []resource.Object
	addErr error
}

func (w *addRecordingWatcher) Add(_ context.Context, obj resource.Object) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.adds = append(w.adds, obj)
	return w.addErr
}

func (w *addRecordingWatcher) Update(context.Context, resource.Object, resource.Object) error {
	return nil
}

func (w *addRecordingWatcher) Delete(context.Context, resource.Object) error { return nil }

// The slice must be read under the lock: passing w.adds as an argument would read the slice header before the
// callee could acquire it, racing a concurrent append.
func (w *addRecordingWatcher) addedNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return listerNames(w.adds)
}

var _ ResourceWatcher = (*addRecordingWatcher)(nil)

func TestNewPeriodicInformer_Validation(t *testing.T) {
	_, err := NewPeriodicInformer(resource.Kind{}, staticLister(), PeriodicInformerOptions{})
	assert.Error(t, err, "an empty kind is rejected")

	_, err = NewPeriodicInformer(listerTestKind, nil, PeriodicInformerOptions{})
	assert.Error(t, err, "a nil lister is rejected")

	inf, err := NewPeriodicInformer(listerTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)
	assert.Error(t, inf.AddEventHandler(nil))
	assert.Equal(t, listerTestKind.Schema, inf.Schema())
}

// The deliveries carry whole objects but are caused by the schedule alone, so only a kind which needs no more than
// that can be served by one.
func TestPeriodicInformer_ProvidesScheduledFullObjects(t *testing.T) {
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)

	provided, stated := InformerWatchOptions(inf)
	require.True(t, stated)
	assert.Equal(t, WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerSchedule}, provided)
	assert.False(t, WatchOptions{}.SatisfiedBy(provided), "a need for a delivery per change is not satisfied")
	assert.True(t, WatchOptions{Triggers: WatchTriggerSchedule}.SatisfiedBy(provided))
	assert.True(t, WatchOptions{Detail: WatchDetailIdentity, Triggers: WatchTriggerSchedule}.SatisfiedBy(provided),
		"a kind needing less detail than the whole object is served too")
	assert.False(t, WatchOptions{Triggers: WatchTriggerChange | WatchTriggerSchedule}.SatisfiedBy(provided),
		"a kind needing both triggers needs a mechanism which also delivers each change")
}

// Every pass delivers every listed object as an add, unchanged objects included: re-delivery is what re-triggers
// cadence-driven work.
func TestPeriodicInformer_RelistsOnCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := &addRecordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls.Add(1)
			return []resource.Object{listerObj("a")}, "", nil
		})
		inf, err := NewPeriodicInformer(listerTestKind, lister, PeriodicInformerOptions{ResyncInterval: time.Minute})
		require.NoError(t, err)
		require.NoError(t, inf.AddEventHandler(watcher))

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, inf.HasSynced())
		require.NoError(t, inf.HealthCheck(ctx))
		assert.Equal(t, []string{"a"}, watcher.addedNames())

		// The jitter spreads each interval by up to 10%, so sleeping past the longest possible one guarantees
		// exactly one more pass.
		time.Sleep(time.Minute + 7*time.Second)
		synctest.Wait()
		assert.Equal(t, []string{"a", "a"}, watcher.addedNames())
		assert.Equal(t, int64(2), calls.Load())
	})
}

// A failing initial list is retried, and the informer does not report synced until one succeeds.
func TestPeriodicInformer_RetriesInitialList(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := &addRecordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if calls.Add(1) < 3 {
				return nil, "", errors.New("api unavailable")
			}
			return []resource.Object{listerObj("a")}, "", nil
		})
		inf, err := NewPeriodicInformer(listerTestKind, lister, PeriodicInformerOptions{
			ResyncInterval: time.Hour,
			ErrorHandler:   func(context.Context, error) {},
		})
		require.NoError(t, err)
		require.NoError(t, inf.AddEventHandler(watcher))

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.False(t, inf.HasSynced(), "must not sync while the initial list fails")
		require.Error(t, inf.HealthCheck(ctx))

		for range 5 {
			if inf.HasSynced() {
				break
			}
			time.Sleep(DefaultPeriodicRetryInterval)
			synctest.Wait()
		}
		require.True(t, inf.HasSynced())
		assert.Equal(t, []string{"a"}, watcher.addedNames())
	})
}

func TestPeriodicInformer_WatcherErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(listerObj("a")), PeriodicInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(&addRecordingWatcher{addErr: errors.New("boom")}))

	require.NoError(t, inf.relist(context.Background()))

	require.Len(t, handled, 1)
	assert.EqualError(t, handled[0], "boom")
}

func TestPeriodicInformer_ListErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	expected := errors.New("api unavailable")
	inf, err := NewPeriodicInformer(listerTestKind, listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return nil, "", expected
	}), PeriodicInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, err)

	assert.ErrorIs(t, inf.relist(context.Background()), expected)
	require.Len(t, handled, 1)
	assert.ErrorIs(t, handled[0], expected)
}

func TestPeriodicInformer_WaitForSyncHonorsContext(t *testing.T) {
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, inf.WaitForSync(ctx), context.Canceled)
}
