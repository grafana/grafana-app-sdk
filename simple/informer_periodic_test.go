package simple

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

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

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

func staticLister(objs ...resource.Object) operator.ResourceLister {
	return listerFunc(func(context.Context) ([]resource.Object, string, error) { return objs, "", nil })
}

type listerFunc func(ctx context.Context) ([]resource.Object, string, error)

func (f listerFunc) List(ctx context.Context) ([]resource.Object, string, error) { return f(ctx) }

// A kind is served by this mechanism only by needing no live channel, which is what the App's check against the
// informer it built enforces.
func TestPeriodicInformerMechanism_ProvidesASweepAndNothingLive(t *testing.T) {
	mechanism := PeriodicInformerMechanism(PeriodicInformerConfig{})
	sweepOnly := operator.WatchOptions{
		Sweep: &operator.WatchSweepOptions{
			Detail:       operator.WatchDetailFullObject,
			Distribution: operator.WatchDistributionAll,
		},
	}
	assert.Equal(t, sweepOnly, mechanism.WatchOptions(testKind()),
		"the mechanism states what it provides, so listing it cannot advertise the wrong thing")

	inf, err := mechanism.NewInformer(testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)

	assert.IsType(t, &operator.ConcurrentInformer{}, inf, "handlers run on per-key workers, as under the other suppliers")
	provided, stated := operator.InformerWatchOptions(inf)
	require.True(t, stated, "the wrapper must not hide what the periodic informer states")
	assert.Equal(t, sweepOnly, provided, "the informer it builds states what the mechanism advertised")
}

// Unlike a mechanism with a live stream, the list is the informer's only source, so selectors reach the server and
// nothing else has to honor them.
func TestPeriodicInformerSupplier_Listing(t *testing.T) {
	t.Run("passes the namespace and selectors to the lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			SweepInterval: time.Minute,
			PageSize:      25,
			ListerFactory: func(_ resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return staticLister(), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			ListWatchOptions: operator.ListWatchOptions{
				Namespace:      "ns",
				LabelFilters:   []string{"app=test"},
				FieldSelectors: []string{"spec.x=1"},
			},
		})
		require.NoError(t, err)

		assert.Equal(t, "ns", gotOpts.ListWatchOptions.Namespace)
		assert.Equal(t, []string{"app=test"}, gotOpts.ListWatchOptions.LabelFilters)
		assert.Equal(t, []string{"spec.x=1"}, gotOpts.ListWatchOptions.FieldSelectors)
		assert.Equal(t, 25, gotOpts.PageSize)
	})

	t.Run("the default lister applies them to the list request", func(t *testing.T) {
		var requested resource.ListOptions
		var namespace string
		client := listingClient{listInto: func(_ context.Context, ns string, opts resource.ListOptions, _ resource.ListObject) error {
			namespace, requested = ns, opts
			return nil
		}}

		inf, err := PeriodicInformerSupplier(PeriodicInformerConfig{})(testKind(), stubClients{client: client},
			operator.InformerOptions{
				ListWatchOptions: operator.ListWatchOptions{Namespace: "ns", LabelFilters: []string{"app=test"}},
			})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = inf.Run(ctx) }()
		require.NoError(t, inf.WaitForSync(ctx))

		assert.Equal(t, "ns", namespace)
		assert.Equal(t, []string{"app=test"}, requested.LabelFilters)
	})

	t.Run("the kind's own page size wins over the config's", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			PageSize: 500,
			ListerFactory: func(_ resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return staticLister(), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			WatchNeeds: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{PageSize: 25}},
		})
		require.NoError(t, err)
		assert.Equal(t, 25, gotOpts.PageSize)
	})

	t.Run("a lister factory error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return nil, expected
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})

	// Wiring outside an App has its own clients and no resource.ClientGenerator to hand over, so a configuration
	// which reads nothing through one must not insist on it.
	t.Run("a lister factory serves a nil client generator", func(t *testing.T) {
		_, err := PeriodicInformerSupplier(PeriodicInformerConfig{
			ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return staticLister(), nil
			},
		})(testKind(), nil, operator.InformerOptions{})
		require.NoError(t, err)
	})

	t.Run("the default lister requires one", func(t *testing.T) {
		_, err := PeriodicInformerSupplier(PeriodicInformerConfig{})(testKind(), nil, operator.InformerOptions{})
		assert.Error(t, err)
	})
}

// The whole point of the mechanism: a deployment lists it and the kinds which need no live channel are served by
// it, with no watch and no cache.
func TestPeriodicInformerMechanism_ServesTheKindsWhichOnlySweep(t *testing.T) {
	appWith := func(needs operator.WatchOptions, supplier InformerSupplier) error {
		_, err := NewApp(AppConfig{
			ClientGenerator: stubClients{},
			InformerConfig: AppInformerConfig{
				InformerSupplier: supplier,
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: needs,
					UsePlain:     true,
				},
			}},
		})
		return err
	}
	selecting := NewSelectingInformerSupplier(nil, PeriodicInformerMechanism(PeriodicInformerConfig{
		SweepInterval: time.Hour,
	}))
	live := operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}}

	assert.NoError(t, appWith(operator.WatchOptions{Sweep: &operator.WatchSweepOptions{Every: time.Hour}}, selecting))

	// A kind needing the live channel is passed over, and falls back to the API watch rather than to this.
	assert.NoError(t, appWith(live, selecting))

	// Handed this mechanism outright, the check refuses it: what the selection would never pick, nothing else may
	// either.
	err := appWith(live, PeriodicInformerSupplier(PeriodicInformerConfig{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"provides live=none sweep=full-object/all, but its reconciler or watcher needs live=identity/all sweep=none")
}

// A mechanism takes a cadence and a page size rather than offering them, so a kind states the ones it needs and the
// App hands them to whichever mechanism it selected.
func TestPeriodicInformerMechanism_SweepCadence(t *testing.T) {
	t.Run("the app passes the kind's own needs to the mechanism", func(t *testing.T) {
		var got operator.InformerOptions
		recording := recordingMechanism{
			provides: PeriodicInformerMechanism(PeriodicInformerConfig{}).WatchOptions(testKind()),
			record:   func(options operator.InformerOptions) { got = options },
		}
		_, err := NewApp(AppConfig{
			ClientGenerator: stubClients{},
			InformerConfig: AppInformerConfig{
				InformerSupplier: NewSelectingInformerSupplier(nil, recording),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Sweep: &operator.WatchSweepOptions{Every: 90 * time.Minute, PageSize: 25},
					},
					UsePlain: true,
				},
			}},
		})
		require.NoError(t, err)
		require.NotNil(t, got.WatchNeeds.Sweep)
		assert.Equal(t, 90*time.Minute, got.WatchNeeds.Sweep.Every)
		assert.Equal(t, int64(25), got.WatchNeeds.Sweep.PageSize)
	})

	t.Run("the kind's cadence is what the informer sweeps at", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var lists atomic.Int64
			inf, err := PeriodicInformerSupplier(PeriodicInformerConfig{
				SweepInterval: time.Hour,
				ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
					return listerFunc(func(context.Context) ([]resource.Object, string, error) {
						lists.Add(1)
						return nil, "", nil
					}), nil
				},
			})(testKind(), nil, operator.InformerOptions{
				WatchNeeds: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{Every: time.Minute}},
			})
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); _ = inf.Run(ctx) }()
			defer func() { cancel(); <-done; synctest.Wait() }()

			synctest.Wait()
			require.Equal(t, int64(1), lists.Load(), "the initial sweep")

			// Past the longest jittered minute, but far short of the hour the config asked for.
			time.Sleep(time.Minute + 7*time.Second)
			synctest.Wait()
			assert.Equal(t, int64(2), lists.Load())
		})
	})
}

// recordingMechanism is a mechanism which provides what it is told to and records the options it was asked to build
// with, so a test can read what the App passed on.
type recordingMechanism struct {
	provides operator.WatchOptions
	record   func(operator.InformerOptions)
}

func (m recordingMechanism) WatchOptions(resource.Kind) operator.WatchOptions { return m.provides }

func (m recordingMechanism) NewInformer(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.Informer, error) {
	m.record(options)
	return PeriodicInformerMechanism(PeriodicInformerConfig{}).NewInformer(kind, clients, options)
}
