package simple

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// identityInformer is a mockInformer which declares the identity payload, standing in for a bus informer in the
// wiring tests.
type identityInformer struct {
	mockInformer
	identity                bool
	announcedForOpinionated bool
}

func (i identityInformer) IdentityPayload() bool { return i.identity }

func (i identityInformer) DeliversOpinionatedMetadata() bool { return i.announcedForOpinionated }

func supplierFor(inf operator.Informer) InformerSupplier {
	return func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
		return inf, nil
	}
}

// A nil subscriber selects the watch, so that an app can be wired for a bus which is switched off without branching
// at every call site.
func TestBusInformerSupplier_NilSubscriberFallsBackToTheWatch(t *testing.T) {
	kind := testKind()
	clients := stubClients{}

	busInf, busErr := BusInformerSupplier(nil, BusInformerConfig{})(kind, clients, operator.InformerOptions{})
	defaultInf, defaultErr := DefaultInformerSupplier(kind, clients, operator.InformerOptions{})

	assert.IsType(t, defaultInf, busInf)
	assert.Equal(t, defaultErr == nil, busErr == nil)
}

func TestBusInformerSupplier_CreatesBusInformers(t *testing.T) {
	kind := testKind()
	clients := stubClients{}
	supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
		ResyncInterval: time.Minute,
		Group:          resourcebus.Group{Name: "group"},
	})

	inf, err := supplier(kind, clients, operator.InformerOptions{
		ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
	})
	require.NoError(t, err)

	wrapped, ok := inf.(*operator.ConcurrentInformer)
	require.True(t, ok, "watchers run on per-key workers, as under the other suppliers")
	assert.True(t, wrapped.IdentityPayload(), "the identity payload is the default, and survives wrapping")
}

func TestBusInformerSupplier_HydratedPayload(t *testing.T) {
	inf, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{Payload: operator.PayloadHydrated})(
		testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)
	assert.False(t, inf.(*operator.ConcurrentInformer).IdentityPayload())
}

func TestBusInformerSupplier_ListerFactory(t *testing.T) {
	t.Run("is used for the kind's lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			PageSize: 25,
			ListerFactory: func(_ resource.Kind, _ resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "", nil }), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
		})
		require.NoError(t, err)

		assert.Equal(t, 25, gotOpts.PageSize)
		assert.Equal(t, "ns", gotOpts.ListWatchOptions.Namespace)
	})

	// The bus delivers by namespace only, so a selector could be applied to the list alone. The live stream would
	// keep delivering excluded objects, which the next filtered re-list would then diff as deletes.
	t.Run("selectors are rejected rather than applied to the list alone", func(t *testing.T) {
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{})

		for _, tt := range []struct {
			name string
			opts operator.ListWatchOptions
		}{
			{"label filters", operator.ListWatchOptions{LabelFilters: []string{"app=test"}}},
			{"field selectors", operator.ListWatchOptions{FieldSelectors: []string{"spec.x=1"}}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{ListWatchOptions: tt.opts})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not supported by the bus informer")
			})
		}
	})

	// A hydrated payload promises whole objects; an identity projection would deliver less on every re-list.
	t.Run("a key lister factory is rejected with the hydrated payload", func(t *testing.T) {
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			Payload: operator.PayloadHydrated,
			KeyListerFactory: func(resource.Kind, operator.ListWatchOptions) (operator.ResourceKeyLister, error) {
				return keysReturning("150"), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be used with PayloadHydrated")
	})

	t.Run("its error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			ListerFactory: func(resource.Kind, resource.Client, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return nil, expected
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})
}

// stubClients hands out a client the wiring tests never call: they only check which informer and lister the
// supplier builds.
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

// ListInto serves the full-object fallback, so a test can tell which lister a re-list went through.
func (stubClient) ListInto(_ context.Context, _ string, _ resource.ListOptions, into resource.ListObject) error {
	into.SetItems([]resource.Object{&resource.UntypedObject{ObjectMeta: metav1.ObjectMeta{Name: "from-objects"}}})
	return nil
}

type listerFunc func(ctx context.Context) ([]resource.Object, string, error)

func (f listerFunc) List(ctx context.Context) ([]resource.Object, string, error) { return f(ctx) }

// An identity-payload informer cannot be paired with an opinionated reconciler or watcher, which read the deletion
// timestamp and the finalizer list off the delivered object. Startup is where that has to fail.
func TestApp_RejectsIdentityPayloadWithOpinionated(t *testing.T) {
	for _, tt := range []struct {
		name    string
		kind    AppManagedKind
		wantErr bool
	}{
		{
			name: "opinionated reconciler",
			kind: AppManagedKind{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: supplierFor(identityInformer{identity: true})},
			},
			wantErr: true,
		},
		{
			name: "opinionated watcher",
			kind: AppManagedKind{
				Kind:             testKind(),
				Watcher:          &operator.SimpleWatcher{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: supplierFor(identityInformer{identity: true})},
			},
			wantErr: true,
		},
		{
			name: "plain reconciler",
			kind: AppManagedKind{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					UsePlain:         true,
					InformerSupplier: supplierFor(identityInformer{identity: true}),
				},
			},
		},
		{
			name: "hydrated payload with an opinionated reconciler",
			kind: AppManagedKind{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: supplierFor(identityInformer{identity: false})},
			},
		},
		{
			name: "an identity informer behind the concurrent wrapper",
			kind: AppManagedKind{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: func(
					k resource.Kind, _ resource.ClientGenerator, opts operator.InformerOptions,
				) (operator.Informer, error) {
					// What the bus supplier itself returns: wrapping must not hide the payload mode.
					return operator.NewConcurrentInformerFromOptions(identityInformer{identity: true}, opts)
				}},
			},
			wantErr: true,
		},
		{
			name: "an identity payload whose bus announces what the opinionated path reads",
			kind: AppManagedKind{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: supplierFor(
					identityInformer{identity: true, announcedForOpinionated: true},
				)},
			},
		},
		{
			name: "an informer which declares no payload",
			kind: AppManagedKind{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{InformerSupplier: supplierFor(&mockInformer{})},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewApp(AppConfig{ManagedKinds: []AppManagedKind{tt.kind}})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "identity-payload informer")
				return
			}
			require.NoError(t, err)
		})
	}
}

// A per-kind supplier overrides the app-level one, which in turn overrides the default.
func TestApp_PerKindInformerSupplier(t *testing.T) {
	kind := testKind()
	perKind, appLevel := false, false

	_, err := NewApp(AppConfig{
		InformerConfig: AppInformerConfig{
			InformerSupplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
				appLevel = true
				return &mockInformer{}, nil
			},
		},
		ManagedKinds: []AppManagedKind{{
			Kind:       kind,
			Reconciler: &Reconciler{},
			ReconcileOptions: BasicReconcileOptions{
				InformerSupplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
					perKind = true
					return &mockInformer{}, nil
				},
			},
		}},
		UnmanagedKinds: []AppUnmanagedKind{{
			Kind:       testUnmanagedKind(),
			Reconciler: &Reconciler{},
		}},
	})
	require.NoError(t, err)

	assert.True(t, perKind, "the managed kind must use its own supplier")
	assert.True(t, appLevel, "the unmanaged kind, with no supplier of its own, must use the app-level one")
}

func testUnmanagedKind() resource.Kind {
	return resource.Kind{
		Schema: resource.NewSimpleSchema("foo", "v1", &resource.UntypedObject{}, &resource.UntypedList{},
			resource.WithKind("Unmanaged")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
}

// keyListerFunc adapts a function to operator.ResourceKeyLister.
type keyListerFunc func(ctx context.Context) (string, iter.Seq2[operator.ResourceKey, error])

func (f keyListerFunc) ListKeys(ctx context.Context) (string, iter.Seq2[operator.ResourceKey, error]) {
	return f(ctx)
}

func keysReturning(listRV string, keys ...operator.ResourceKey) operator.ResourceKeyLister {
	return keyListerFunc(func(context.Context) (string, iter.Seq2[operator.ResourceKey, error]) {
		return listRV, func(yield func(operator.ResourceKey, error) bool) {
			for _, key := range keys {
				if !yield(key, nil) {
					return
				}
			}
		}
	})
}

// A key lister factory is all an adopter with an identity-serving endpoint supplies: the supplier composes the
// identity lister and the full-object fallback around it.
func TestBusInformerSupplier_KeyListerFactory(t *testing.T) {
	t.Run("identities serve the re-list", func(t *testing.T) {
		cfg := BusInformerConfig{
			KeyListerFactory: func(resource.Kind, operator.ListWatchOptions) (operator.ResourceKeyLister, error) {
				return keysReturning("150", operator.ResourceKey{Name: "a", ResourceVersion: "11"}), nil
			},
		}

		lister, err := cfg.lister(testKind(), stubClient{}, operator.ClientListerOptions{})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "a", objs[0].GetName())
		assert.Equal(t, "150", listRV)
	})

	// A factory which cannot honor the kind's scope says so, and that kind lists full objects rather than failing
	// the informer: a full list is still a correct snapshot, and it is the one which can apply the filters.
	t.Run("a factory which cannot honor the scope leaves the object lister", func(t *testing.T) {
		var gotOpts operator.ListWatchOptions
		cfg := BusInformerConfig{
			KeyListerFactory: func(_ resource.Kind, opts operator.ListWatchOptions) (operator.ResourceKeyLister, error) {
				gotOpts = opts
				return nil, fmt.Errorf("the keys endpoint cannot filter: %w", operator.ErrProjectionUnsupported)
			},
		}

		lister, err := cfg.lister(testKind(), stubClient{}, operator.ClientListerOptions{
			ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
		})
		require.NoError(t, err)
		assert.Equal(t, "ns", gotOpts.Namespace, "the factory is told what scope it has to honor")

		objs, _, err := lister.List(context.Background())
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "from-objects", objs[0].GetName())
	})

	t.Run("its error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			KeyListerFactory: func(resource.Kind, operator.ListWatchOptions) (operator.ResourceKeyLister, error) {
				return nil, expected
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})

	t.Run("ListerFactory takes precedence", func(t *testing.T) {
		var keysBuilt bool
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			KeyListerFactory: func(resource.Kind, operator.ListWatchOptions) (operator.ResourceKeyLister, error) {
				keysBuilt = true
				return keysReturning("150"), nil
			},
			ListerFactory: func(resource.Kind, resource.Client, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "", nil }), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)
		assert.False(t, keysBuilt, "a whole-lister replacement does not also build a key lister")
	})
}

// Two suppliers sharing a metrics namespace would otherwise each build collectors with identical descriptors, and
// registration keeps only the first — leaving the second supplier's informers updating vectors nothing exports.
func TestBusInformerSupplier_SharesInjectedMetrics(t *testing.T) {
	shared := operator.NewBusInformerMetrics(operator.BusInformerMetricsConfig{
		MetricsConfig: metrics.DefaultConfig("test"),
	})
	cfg := BusInformerConfig{Metrics: shared}

	first, err := BusInformerSupplier(resourcebus.NewFake(), cfg)(testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)
	second, err := BusInformerSupplier(resourcebus.NewFake(), cfg)(testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)

	// The wrapper adds its own queue-depth collector; what matters is that both carry the shared set.
	assert.Subset(t, first.(*operator.ConcurrentInformer).PrometheusCollectors(), shared.PrometheusCollectors())
	assert.Subset(t, second.(*operator.ConcurrentInformer).PrometheusCollectors(), shared.PrometheusCollectors(),
		"both suppliers report the collectors that were actually registered")
}

// The periodic informer's only source is the list, so selectors reach the server and nothing else has to honor
// them — unlike the bus supplier, which rejects them because its live stream cannot.
func TestPeriodicInformerSupplier(t *testing.T) {
	t.Run("passes the namespace and selectors to the lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			ResyncInterval: time.Minute,
			PageSize:       25,
			ListerFactory: func(_ resource.Kind, _ resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "", nil }), nil
			},
		})

		inf, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			ListWatchOptions: operator.ListWatchOptions{
				Namespace:      "ns",
				LabelFilters:   []string{"app=test"},
				FieldSelectors: []string{"spec.x=1"},
			},
		})
		require.NoError(t, err)
		assert.IsType(t, &operator.ConcurrentInformer{}, inf, "handlers run on per-key workers, as under the other suppliers")

		assert.Equal(t, "ns", gotOpts.ListWatchOptions.Namespace)
		assert.Equal(t, []string{"app=test"}, gotOpts.ListWatchOptions.LabelFilters)
		assert.Equal(t, []string{"spec.x=1"}, gotOpts.ListWatchOptions.FieldSelectors)
		assert.Equal(t, 25, gotOpts.PageSize)
	})

	t.Run("the default lister applies them to the list request", func(t *testing.T) {
		var requested resource.ListOptions
		var namespace string
		client := filteringClient{listInto: func(_ context.Context, ns string, opts resource.ListOptions, _ resource.ListObject) error {
			namespace, requested = ns, opts
			return nil
		}}
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{})

		inf, err := supplier(testKind(), stubClients{client: client}, operator.InformerOptions{
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

	t.Run("a lister factory error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			ListerFactory: func(resource.Kind, resource.Client, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return nil, expected
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})
}

// filteringClient records what the lister asked the API for.
type filteringClient struct {
	resource.Client
	listInto func(ctx context.Context, namespace string, opts resource.ListOptions, into resource.ListObject) error
}

func (c filteringClient) ListInto(ctx context.Context, namespace string, opts resource.ListOptions, into resource.ListObject) error {
	return c.listInto(ctx, namespace, opts, into)
}
