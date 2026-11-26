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

// added returns the objects as delivered, for a test which reads more than their names. The slice must be read
// under the lock: passing w.adds as an argument would read the slice header before the callee could acquire it,
// racing a concurrent append.
func (w *addRecordingWatcher) added() []resource.Object {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]resource.Object(nil), w.adds...)
}

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

// A resync carrying whole objects to every instance, and no live channel, so only a kind which needs no live
// delivery can be served by one.
func TestPeriodicInformer_ProvidesAResyncAndNothingLive(t *testing.T) {
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)

	provided, stated := InformerWatchOptions(inf)
	require.True(t, stated)
	assert.Equal(t, WatchOptions{
		Resync: &WatchResyncOptions{Detail: WatchDetailFullObject, Distribution: WatchDistributionAll},
	}, provided)

	assert.False(t, WatchOptions{}.SatisfiedBy(provided), "a kind which declares nothing needs the live channel")
	assert.True(t, WatchOptions{Resync: &WatchResyncOptions{}}.SatisfiedBy(provided))
	assert.True(t, WatchOptions{
		Resync: &WatchResyncOptions{Detail: WatchDetailIdentity, Distribution: WatchDistributionAll},
	}.SatisfiedBy(provided), "a kind needing less detail of the resync is served too")
	assert.True(t, WatchOptions{
		Resync: &WatchResyncOptions{Interval: time.Hour, ListPageSize: 5000},
	}.SatisfiedBy(provided), "the cadence and page size are requests, not part of the comparison")
	assert.True(t, WatchOptions{
		Resync: &WatchResyncOptions{Distribution: WatchDistributionShared},
	}.SatisfiedBy(provided), "a kind which permits the resync to be divided is served by one which divides nothing")
	assert.False(t, WatchOptions{
		Resync: &WatchResyncOptions{Distribution: WatchDistributionSharded},
	}.SatisfiedBy(provided), "every instance lists the whole kind, so the affinity sharding gives is not available")
	assert.False(t, WatchOptions{
		Live:   &WatchLiveOptions{},
		Resync: &WatchResyncOptions{},
	}.SatisfiedBy(provided), "a kind which also acts on changes needs a mechanism with a live channel")
}

// Every pass delivers every listed object as an add, unchanged objects included: re-delivery is what re-triggers
// cadence-driven work.
func TestPeriodicInformer_ResyncsOnCadence(t *testing.T) {
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

// A failed pass must not cost the kind a whole interval: it retries on the short one, and returns to the cadence
// once a pass succeeds.
func TestPeriodicInformer_RetriesAFailedPassOnTheShortInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			// The initial pass succeeds; the next two fail.
			if n := calls.Add(1); n > 1 && n < 4 {
				return nil, "", errors.New("api unavailable")
			}
			return nil, "", nil
		})
		inf, err := NewPeriodicInformer(listerTestKind, lister, PeriodicInformerOptions{
			ResyncInterval: time.Hour,
			RetryInterval:  time.Second,
			ErrorHandler:   func(context.Context, error) {},
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.Equal(t, int64(1), calls.Load())

		// Past the first jittered hour, plus seconds: the failed pass and its two retries all fit, which an hourly
		// retry could not.
		time.Sleep(time.Hour + 7*time.Minute + 5*time.Second)
		synctest.Wait()
		require.Equal(t, int64(4), calls.Load(), "retried on the short interval until one succeeded")

		// Back to the cadence once a pass succeeds: nothing more for the rest of the hour.
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int64(4), calls.Load())
	})
}

// Handlers slower than the cadence must not accumulate a backlog: a pass waits for the last one's deliveries to

func TestPeriodicInformerWaitsForHandlerBeforeNextSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var calls atomic.Int64
		var handlerErr error
		informer, err := NewPeriodicInformer(listerTestKind, listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls.Add(1)
			return []resource.Object{listerObj("a")}, "", nil
		}), PeriodicInformerOptions{
			ResyncInterval: time.Minute,
			EventTimeout:   2 * time.Minute,
			ErrorHandler:   func(_ context.Context, err error) { handlerErr = err },
		})
		require.NoError(t, err)
		require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(ctx context.Context, _ resource.Object) error {
			<-ctx.Done()
			return ctx.Err()
		}}))
		go func() { _ = informer.Run(ctx) }()
		time.Sleep(90 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(1), calls.Load())
		require.False(t, informer.HasSynced())
		time.Sleep(30 * time.Second)
		synctest.Wait()
		require.ErrorIs(t, handlerErr, context.DeadlineExceeded)
		require.True(t, informer.HasSynced())
		require.Equal(t, int64(1), calls.Load())
		time.Sleep(67 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(2), calls.Load())
	})
}

func TestPeriodicInformerControllerRetriesWithFreshTimeout(t *testing.T) {
	for _, name := range []string{"watcher error", "reconciler error", "requeue"} {
		t.Run(name, func(t *testing.T) {
			runCtx, stop := context.WithCancel(t.Context())
			defer stop()
			const timeout = time.Second
			informer, err := NewPeriodicInformer(listerTestKind, staticLister(listerObj("item")), PeriodicInformerOptions{EventTimeout: timeout})
			require.NoError(t, err)
			controller := NewInformerController(InformerControllerConfig{})
			const key = "widget"
			require.NoError(t, controller.AddInformer(informer, key))

			var attempts []context.Context
			var attemptErrors []error
			record := func(ctx context.Context) error {
				attempts = append(attempts, ctx)
				attemptErrors = append(attemptErrors, ctx.Err())
				if len(attempts) == 1 && name != "requeue" {
					return errors.New("retry requested")
				}
				return nil
			}
			if name == "watcher error" {
				require.NoError(t, controller.AddWatcher(&SimpleWatcher{AddFunc: func(ctx context.Context, _ resource.Object) error {
					return record(ctx)
				}}, key))
			} else {
				require.NoError(t, controller.AddReconciler(&SimpleReconciler{ReconcileFunc: func(ctx context.Context, _ ReconcileRequest) (ReconcileResult, error) {
					err := record(ctx)
					if len(attempts) == 1 && name == "requeue" {
						return ReconcileResult{RequeueAfter: new(time.Second)}, nil
					}
					return ReconcileResult{}, err
				}}, key))
			}

			require.NoError(t, informer.sweep(runCtx))
			require.Len(t, attempts, 1)
			require.ErrorIs(t, attempts[0].Err(), context.Canceled)
			keys := controller.toRetry.Keys()
			require.Len(t, keys, 1)
			var retry retryInfo
			controller.toRetry.RemoveItems(keys[0], func(item retryInfo) bool {
				retry = item
				return true
			}, 1)
			_, err = retry.retryFunc()
			require.NoError(t, err)
			require.Len(t, attempts, 2)
			require.NoError(t, attemptErrors[1])
			retryDeadline, ok := attempts[1].Deadline()
			require.True(t, ok)
			originalDeadline, _ := attempts[0].Deadline()
			require.True(t, retryDeadline.After(originalDeadline))
			// Shutdown also cancels any later attempt.
			stop()
			_, err = retry.retryFunc()
			require.ErrorIs(t, err, context.Canceled)
			require.Len(t, attempts, 2)
		})
	}
}
