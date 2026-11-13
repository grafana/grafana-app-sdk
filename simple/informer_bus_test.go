package simple

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// What the informers provide is what a deployment offers them under, so the payload has to survive the wrapping the
// supplier does.
func TestBusInformerSupplier_Provides(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config BusInformerConfig
		want   operator.WatchDetail
	}{
		{"identity is the default payload", BusInformerConfig{}, operator.WatchDetailIdentity},
		{"the hydrated payload carries the object", BusInformerConfig{Payload: operator.PayloadHydrated},
			operator.WatchDetailFullObject},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inf, err := BusInformerSupplier(resourcebus.NewFake(), tt.config)(
				testKind(), stubClients{}, operator.InformerOptions{})
			require.NoError(t, err)

			assert.IsType(t, &operator.ConcurrentInformer{}, inf,
				"watchers run on per-key workers, as under the other suppliers")
			provided, described := operator.InformerWatchOptions(inf)
			require.True(t, described, "the wrapper must not hide what the bus informer provides")
			assert.Equal(t, operator.WatchOptions{
				Detail:   tt.want,
				Triggers: operator.WatchTriggerChange | operator.WatchTriggerSchedule,
			}, provided, "the re-list is a pass over every object, as well as the recovery mechanism")
		})
	}
}

// A deployment with no bus offers no bus: a nil supplier is an offer it cannot provide, so the kinds it would have
// served fall through to the next offer or to the API watch.
func TestBusInformerSupplier_NilSubscriberProvidesNoSupplier(t *testing.T) {
	assert.Nil(t, BusInformerSupplier(nil, BusInformerConfig{}))
}

func TestBusInformerSupplier_Listing(t *testing.T) {
	t.Run("passes the namespace and page size to the lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			ResyncInterval: time.Minute,
			PageSize:       25,
			ListerFactory: func(_ resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
				gotOpts = opts
				return staticLister(), nil
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{
			ListWatchOptions: operator.ListWatchOptions{Namespace: "ns"},
		})
		require.NoError(t, err)

		assert.Equal(t, "ns", gotOpts.ListWatchOptions.Namespace)
		assert.Equal(t, 25, gotOpts.PageSize)
	})

	t.Run("a lister factory error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		supplier := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return nil, expected
			},
		})

		_, err := supplier(testKind(), stubClients{}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})

	// Wiring outside an App has its own clients and no resource.ClientGenerator to hand over, so a configuration
	// which reads nothing through one must not insist on it.
	t.Run("a lister factory and an identity payload serve a nil client generator", func(t *testing.T) {
		_, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return staticLister(), nil
			},
		})(testKind(), nil, operator.InformerOptions{})
		require.NoError(t, err)
	})

	t.Run("the hydrated payload requires one even with a lister factory", func(t *testing.T) {
		_, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			Payload: operator.PayloadHydrated,
			ListerFactory: func(resource.Kind, operator.ClientListerOptions) (operator.ResourceLister, error) {
				return staticLister(), nil
			},
		})(testKind(), nil, operator.InformerOptions{})
		assert.Error(t, err)
	})

	t.Run("the default lister requires one", func(t *testing.T) {
		_, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{})(
			testKind(), nil, operator.InformerOptions{})
		assert.Error(t, err)
	})

	t.Run("the default lister applies the namespace and selectors to the list request", func(t *testing.T) {
		var requested resource.ListOptions
		var namespace string
		client := listingClient{listInto: func(_ context.Context, ns string, opts resource.ListOptions, _ resource.ListObject) error {
			namespace, requested = ns, opts
			return nil
		}}

		inf, err := BusInformerSupplier(resourcebus.NewFake(), BusInformerConfig{
			AnnouncedMetadata: operator.AnnouncedMetadata{Labels: []string{"app"}},
		})(testKind(), stubClients{client: client}, operator.InformerOptions{
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
}

// The subscription is scoped by namespace alone, so a selector the live stream cannot be matched against is refused
// at construction: narrowing the list while live events keep delivering excluded objects would leave the two paths
// disagreeing about what the informer watches.
func TestBusInformerSupplier_SelectorValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config BusInformerConfig
		opts   operator.ListWatchOptions
		want   string
	}{
		{
			name: "a label the bus does not announce",
			opts: operator.ListWatchOptions{LabelFilters: []string{"app=test"}},
			want: `label filter on "app"`,
		},
		{
			name: "any field selector on an identity payload",
			opts: operator.ListWatchOptions{FieldSelectors: []string{"spec.x=1"}},
			want: "field selectors cannot be used with identity",
		},
		{
			name:   "an announced label is accepted",
			config: BusInformerConfig{AnnouncedMetadata: operator.AnnouncedMetadata{Labels: []string{"app"}}},
			opts:   operator.ListWatchOptions{LabelFilters: []string{"app=test"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BusInformerSupplier(resourcebus.NewFake(), tt.config)(
				testKind(), stubClients{}, operator.InformerOptions{ListWatchOptions: tt.opts})
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// The whole point of the supplier: a deployment offers its bus, and the kinds which declare they read no more than
// an identity are served by it — with no watch and no cache — while the rest take the API watch.
func TestBusInformerSupplier_AsAChangeOffer(t *testing.T) {
	appWith := func(provides operator.WatchOptions, cfg BusInformerConfig, reconcile BasicReconcileOptions) error {
		_, err := NewApp(AppConfig{
			ClientGenerator: stubClients{},
			InformerConfig: AppInformerConfig{
				InformerSupplierDecider: NewInformerSupplierDecider(InformerSupplierOffer{
					Provides: provides,
					Supplier: BusInformerSupplier(resourcebus.NewFake(), cfg),
				}),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: reconcile,
			}},
		})
		return err
	}

	perChange := operator.WatchTriggerChange | operator.WatchTriggerSchedule
	identityOffer := operator.WatchOptions{Detail: operator.WatchDetailIdentity, Triggers: perChange}
	hydratedOffer := operator.WatchOptions{Detail: operator.WatchDetailFullObject, Triggers: perChange}

	assert.NoError(t, appWith(identityOffer, BusInformerConfig{ResyncInterval: time.Hour}, BasicReconcileOptions{
		WatchOptions: operator.WatchOptions{Detail: operator.WatchDetailIdentity},
		UsePlain:     true,
	}), "a kind which reads no more than an identity is served by the bus")

	assert.NoError(t, appWith(hydratedOffer, BusInformerConfig{Payload: operator.PayloadHydrated},
		BasicReconcileOptions{}), "a hydrated payload serves an opinionated reconciler, at a read per event")

	// The Opinionated reconciler reads the deletion timestamp and the finalizer list off the delivered object,
	// which an identity delivery does not carry whatever the bus announces alongside it. Offered honestly the kind
	// would pass over the bus and take the API watch; offered as whole objects the informer itself refuses it.
	err := appWith(hydratedOffer, BusInformerConfig{}, BasicReconcileOptions{})
	require.Error(t, err, "an offer labelled above what the payload carries is refused")
	assert.Contains(t, err.Error(),
		"provides detail=identity triggers=change+schedule, but its opinionated reconciler or watcher needs detail=full-object triggers=change")
}
