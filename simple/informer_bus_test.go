package simple

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// identityInformer is a mockInformer which declares the identity payload, standing in for a relist informer in the
// wiring tests.
type identityInformer struct {
	mockInformer
	identity bool
}

func (i identityInformer) IdentityPayload() bool { return i.identity }

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

func TestBusInformerSupplier_CreatesRelistInformers(t *testing.T) {
	kind := testKind()
	clients := stubClients{}
	supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
		RelistInformerConfig: RelistInformerConfig{ResyncInterval: time.Minute},
		Group:                resourcebus.Group{Name: "group"},
	})

	inf, err := supplier(kind, clients, operator.InformerOptions{
		ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
	})
	require.NoError(t, err)

	busInf, ok := inf.(*operator.RelistInformer)
	require.True(t, ok, "the supplier must return a RelistInformer")
	assert.True(t, busInf.IdentityPayload(), "the identity payload is the default")
	assert.Equal(t, kind.Schema, busInf.Schema())
}

func TestBusInformerSupplier_HydratedPayload(t *testing.T) {
	inf, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{Payload: operator.PayloadHydrated})(
		testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)
	assert.False(t, inf.(*operator.RelistInformer).IdentityPayload())
}

// The source factory decides what drives the informer between re-lists, and receives the kind's client and options.
func TestRelistInformerSupplier_SourceFactory(t *testing.T) {
	t.Run("none is re-list only", func(t *testing.T) {
		inf, err := RelistInformerSupplier(RelistInformerConfig{})(testKind(), stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)
		assert.IsType(t, &operator.RelistInformer{}, inf)
	})

	t.Run("the kubernetes watch is the kind's client", func(t *testing.T) {
		source, err := KubernetesWatchSource(testKind(), stubClient{}, operator.InformerOptions{})
		require.NoError(t, err)
		assert.Equal(t, stubClient{}, source)
	})

	t.Run("receives the kind's options", func(t *testing.T) {
		var gotNamespace string
		_, err := RelistInformerSupplier(RelistInformerConfig{
			SourceFactory: func(_ resource.Kind, client resource.Client, options operator.InformerOptions) (operator.ChangeSource, error) {
				gotNamespace = options.ListWatchOptions.Namespace
				return client, nil
			},
		})(testKind(), stubClients{}, operator.InformerOptions{ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"}})
		require.NoError(t, err)
		assert.Equal(t, "ns", gotNamespace)
	})

	t.Run("its error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		_, err := RelistInformerSupplier(RelistInformerConfig{
			SourceFactory: func(resource.Kind, resource.Client, operator.InformerOptions) (operator.ChangeSource, error) {
				return nil, expected
			},
		})(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})
}

// The bus supplier replaces any source factory with the bus: a config which reuses a relist config wholesale must not
// silently watch the API server instead.
func TestBusInformerSupplier_IgnoresSourceFactory(t *testing.T) {
	var called bool
	_, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{RelistInformerConfig: RelistInformerConfig{
		SourceFactory: func(_ resource.Kind, client resource.Client, _ operator.InformerOptions) (operator.ChangeSource, error) {
			called = true
			return client, nil
		},
	}})(testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)
	assert.False(t, called)
}

func TestBusInformerSupplier_ListerFactory(t *testing.T) {
	t.Run("is used for the kind's lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{RelistInformerConfig: RelistInformerConfig{
			PageSize: 25,
			ListerFactory: func(_ resource.Kind, _ resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return listerFunc(func(context.Context) ([]resource.Object, string, error) { return nil, "", nil }), nil
			},
		}})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			ListWatchOptions: operator.ListWatchOptions{Namespace: "ns", LabelFilters: []string{"app=test"}},
		})
		require.NoError(t, err)

		assert.Equal(t, 25, gotOpts.PageSize)
		assert.Equal(t, "ns", gotOpts.ListWatchOptions.Namespace)
		assert.Equal(t, []string{"app=test"}, gotOpts.ListWatchOptions.LabelFilters)
	})

	t.Run("its error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{RelistInformerConfig: RelistInformerConfig{
			ListerFactory: func(resource.Kind, resource.Client, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return nil, expected
			},
		}})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})
}

// stubClients hands out a client the wiring tests never call: they only check which informer and lister the
// supplier builds.
type stubClients struct{}

func (stubClients) ClientFor(resource.Kind) (resource.Client, error) { return stubClient{}, nil }

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
func TestRelistInformerSupplier_KeyListerFactory(t *testing.T) {
	t.Run("identities serve the re-list", func(t *testing.T) {
		cfg := RelistInformerConfig{
			KeyListerFactory: func(resource.Kind) (operator.ResourceKeyLister, error) {
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

	// A re-list the key lister cannot serve falls back to full objects rather than failing: a full list is still a
	// correct snapshot, and failing the tick would stop the informer reconciling altogether.
	t.Run("a scoped re-list falls back to the object lister", func(t *testing.T) {
		cfg := RelistInformerConfig{
			KeyListerFactory: func(resource.Kind) (operator.ResourceKeyLister, error) {
				return keysReturning("150"), nil
			},
		}

		lister, err := cfg.lister(testKind(), stubClient{}, operator.ClientListerOptions{
			ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
		})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "from-objects", objs[0].GetName())
	})

	t.Run("its error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := RelistInformerSupplier(RelistInformerConfig{
			KeyListerFactory: func(resource.Kind) (operator.ResourceKeyLister, error) { return nil, expected },
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})

	t.Run("ListerFactory takes precedence", func(t *testing.T) {
		var keysBuilt bool
		supplier := RelistInformerSupplier(RelistInformerConfig{
			KeyListerFactory: func(resource.Kind) (operator.ResourceKeyLister, error) {
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
