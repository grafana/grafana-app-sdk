package operator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/metrics"
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
// drain, checking again on the short interval rather than waiting out another whole cadence.
func TestPeriodicInformer_DefersAPassUntilThePreviousDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var lists, pending atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			lists.Add(1)
			return nil, "", nil
		})
		inf, err := NewPeriodicInformer(listerTestKind, lister, PeriodicInformerOptions{
			ResyncInterval: time.Hour,
			RetryInterval:  time.Second,
		})
		require.NoError(t, err)
		inf.backlog = func() float64 { return float64(pending.Load()) }

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.Equal(t, int64(1), lists.Load(), "the initial pass")

		pending.Store(3)
		time.Sleep(time.Hour + 7*time.Minute)
		synctest.Wait()
		assert.Equal(t, int64(1), lists.Load(), "no pass starts while the last one is still queued")

		pending.Store(0)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assert.Equal(t, int64(2), lists.Load(), "the deferred pass runs on the short interval once drained")
	})
}

// Shutdown must not spend a whole snapshot dispatching with a canceled context.
func TestPeriodicInformer_StopsDispatchingWhenCanceled(t *testing.T) {
	watcher := &addRecordingWatcher{}
	inf, err := NewPeriodicInformer(listerTestKind,
		staticLister(listerObj("a"), listerObj("b"), listerObj("c")), PeriodicInformerOptions{})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(watcher))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, inf.resync(ctx), context.Canceled)
	assert.Empty(t, watcher.addedNames())
}

func TestPeriodicInformer_WatcherErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(listerObj("a")), PeriodicInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(&addRecordingWatcher{addErr: errors.New("boom")}))

	require.NoError(t, inf.resync(context.Background()))

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

	assert.ErrorIs(t, inf.resync(context.Background()), expected)
	require.Len(t, handled, 1)
	assert.ErrorIs(t, handled[0], expected)
}

// The opinionated handlers write finalizer updates back into the object they were given, so a second handler must
// not be handed what the first one changed.
func TestPeriodicInformer_CopiesPerHandler(t *testing.T) {
	first, second := &addRecordingWatcher{}, &addRecordingWatcher{}
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(listerObj("a")), PeriodicInformerOptions{})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(&SimpleWatcher{
		AddFunc: func(_ context.Context, obj resource.Object) error {
			obj.SetFinalizers([]string{"mutated-by-the-first-handler"})
			return first.Add(context.Background(), obj)
		},
	}))
	require.NoError(t, inf.AddEventHandler(second))

	require.NoError(t, inf.resync(context.Background()))

	require.Len(t, second.added(), 1)
	assert.Empty(t, second.added()[0].GetFinalizers(), "the second handler saw the first handler's mutation")
}

func TestPeriodicInformer_WaitForSyncHonorsContext(t *testing.T) {
	inf, err := NewPeriodicInformer(listerTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, inf.WaitForSync(ctx), context.Canceled)
}

type stubClients struct{ client resource.Client }

func (s stubClients) ClientFor(resource.Kind) (resource.Client, error) {
	if s.client != nil {
		return s.client, nil
	}
	return stubClient{}, nil
}

func (stubClients) GetCustomRouteClient(schema.GroupVersion, string) (resource.CustomRouteClient, error) {
	return nil, nil
}

func (stubClients) DiscoveryClient() (resource.DiscoveryClient, error) { return nil, nil }

type stubClient struct{ resource.Client }

func (stubClient) ListInto(context.Context, string, resource.ListOptions, resource.ListObject) error {
	return nil
}

// listingClient records what the lister asked the API for.
type listingClient struct {
	resource.Client
	listInto func(ctx context.Context, namespace string, opts resource.ListOptions, into resource.ListObject) error
}

func (c listingClient) ListInto(ctx context.Context, namespace string, opts resource.ListOptions, into resource.ListObject) error {
	return c.listInto(ctx, namespace, opts, into)
}

// A kind is served by this mechanism only by needing no live channel, which is what the App's check against the
// informer it built enforces.
func TestPeriodicInformerMechanism_ProvidesAResyncAndNothingLive(t *testing.T) {
	mechanism := NewPeriodicMechanism(PeriodicMechanismOptions{})
	resyncOnly := WatchOptions{
		Resync: &WatchResyncOptions{
			Detail:       WatchDetailFullObject,
			Distribution: WatchDistributionAll,
		},
	}
	assert.Equal(t, resyncOnly, mechanism.WatchOptions(listerTestKind),
		"the mechanism states what it provides, so listing it cannot advertise the wrong thing")

	inf, err := mechanism.NewInformer(listerTestKind, stubClients{}, InformerOptions{})
	require.NoError(t, err)

	// Stated by the informer itself, so NewSelectingInformerSupplier reads it rather than re-advertising for it.
	_, directly := inf.(WatchOptionsInformer)
	assert.True(t, directly, "what NewInformer returns must say what it provides without being unwrapped")
	provided, stated := InformerWatchOptions(inf)
	require.True(t, stated)
	assert.Equal(t, resyncOnly, provided, "the informer reports what the mechanism advertised")

	// Handlers still run on per-key workers, as under the other suppliers.
	unwrapped, ok := inf.(interface{ Unwrap() Informer })
	require.True(t, ok)
	concurrent, ok := unwrapped.Unwrap().(*ConcurrentInformer)
	require.True(t, ok)
	periodic, ok := concurrent.Unwrap().(*PeriodicInformer)
	require.True(t, ok)
	assert.NotNil(t, periodic.backlog, "a pass must wait for the workers to drain the last one")
}

// The list is the informer's only source, so the selectors reach the server and nothing else has to honor them.
func TestPeriodicInformerMechanism_Listing(t *testing.T) {
	t.Run("the list request carries the namespace, selectors and page size", func(t *testing.T) {
		var requested resource.ListOptions
		var namespace string
		client := listingClient{listInto: func(_ context.Context, ns string, opts resource.ListOptions, _ resource.ListObject) error {
			namespace, requested = ns, opts
			return nil
		}}

		inf, err := NewPeriodicMechanism(PeriodicMechanismOptions{PageSize: 25}).NewInformer(
			listerTestKind, stubClients{client: client}, InformerOptions{
				ListWatchOptions: ListWatchOptions{
					Namespace:      "ns",
					LabelFilters:   []string{"app=test"},
					FieldSelectors: []string{"spec.x=1"},
				},
			})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = inf.Run(ctx) }()
		require.NoError(t, inf.WaitForSync(ctx))

		assert.Equal(t, "ns", namespace)
		assert.Equal(t, []string{"app=test"}, requested.LabelFilters)
		assert.Equal(t, []string{"spec.x=1"}, requested.FieldSelectors)
		assert.Equal(t, 25, requested.Limit)
	})

	t.Run("the app-wide page size serves a kind which asked for no resync at all", func(t *testing.T) {
		var limit int
		client := listingClient{listInto: func(_ context.Context, _ string, opts resource.ListOptions, _ resource.ListObject) error {
			limit = opts.Limit
			return nil
		}}
		inf, err := NewPeriodicMechanism(PeriodicMechanismOptions{}).
			NewInformer(listerTestKind, stubClients{client: client}, InformerOptions{WatchListPageSize: 5000})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = inf.Run(ctx) }()
		require.NoError(t, inf.WaitForSync(ctx))
		assert.Equal(t, 5000, limit)
	})

	// Two kinds would otherwise both export concurrent_watcher_queue_depth{kind=""}, and a registry keeps the first.
	t.Run("each kind's queue-depth metric is its own", func(t *testing.T) {
		other := resource.Kind{Schema: resource.NewSimpleSchema("example.grafana.app", "v1",
			&resource.UntypedObject{}, &resource.UntypedList{},
			resource.WithKind("Gadget"), resource.WithPlural("gadgets"))}

		registry := prometheus.NewRegistry()
		for _, kind := range []resource.Kind{listerTestKind, other} {
			inf, err := NewPeriodicMechanism(PeriodicMechanismOptions{}).
				NewInformer(kind, stubClients{}, InformerOptions{})
			require.NoError(t, err)
			provider, ok := inf.(metrics.Provider)
			require.True(t, ok)
			for _, collector := range provider.PrometheusCollectors() {
				require.NoError(t, registry.Register(collector), "kind %s", kind.Kind())
			}
		}
	})

	t.Run("a client generator is required", func(t *testing.T) {
		_, err := NewPeriodicMechanism(PeriodicMechanismOptions{}).NewInformer(listerTestKind, nil, InformerOptions{})
		assert.Error(t, err)
	})
}

// A mechanism takes a cadence and a page size rather than offering them: ResyncOptions resolves what the kind asked
// for against the app-wide figures, and these options answer for whatever neither set.
func TestPeriodicMechanism_ResyncCadence(t *testing.T) {
	t.Run("the kind's cadence is what the informer resyncs at", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var lists atomic.Int64
			client := listingClient{listInto: func(context.Context, string, resource.ListOptions, resource.ListObject) error {
				lists.Add(1)
				return nil
			}}
			inf, err := NewPeriodicMechanism(PeriodicMechanismOptions{ResyncInterval: time.Hour}).
				NewInformer(listerTestKind, stubClients{client: client}, InformerOptions{
					WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{Interval: time.Minute}},
				})
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); _ = inf.Run(ctx) }()
			defer func() { cancel(); <-done; synctest.Wait() }()

			synctest.Wait()
			require.Equal(t, int64(1), lists.Load(), "the initial resync")

			// Past the longest jittered minute, but far short of the hour the options asked for.
			time.Sleep(time.Minute + 7*time.Second)
			synctest.Wait()
			assert.Equal(t, int64(2), lists.Load())
		})
	})

	t.Run("resolves the page size the kind left unset", func(t *testing.T) {
		pageSizeFor := func(options InformerOptions) int {
			var limit int
			client := listingClient{listInto: func(_ context.Context, _ string, opts resource.ListOptions, _ resource.ListObject) error {
				limit = opts.Limit
				return nil
			}}
			inf, err := NewPeriodicMechanism(PeriodicMechanismOptions{PageSize: 500}).
				NewInformer(listerTestKind, stubClients{client: client}, options)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = inf.Run(ctx) }()
			require.NoError(t, inf.WaitForSync(ctx))
			return limit
		}
		hourly := WatchOptions{Resync: &WatchResyncOptions{Interval: time.Hour}}

		assert.Equal(t, 500, pageSizeFor(InformerOptions{WatchNeeds: hourly}),
			"a resync asked for without a page size takes the mechanism's own")
		assert.Equal(t, 25, pageSizeFor(InformerOptions{WatchNeeds: hourly, WatchListPageSize: 25}),
			"the app-wide page size serves a kind which stated none")
		assert.Equal(t, 25, pageSizeFor(InformerOptions{
			WatchListPageSize: 500,
			WatchNeeds:        WatchOptions{Resync: &WatchResyncOptions{Interval: time.Hour, ListPageSize: 25}},
		}), "the kind's own wins over the app-wide one")
	})
}
