package simple

import (
	"context"
	"errors"
	"testing"
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

// A kind is served by this supplier only by declaring it needs no delivery per change, which is what the App's
// check against the informer it built enforces.
func TestPeriodicInformerSupplier_ProvidesScheduledFullObjects(t *testing.T) {
	inf, err := PeriodicInformerSupplier(PeriodicInformerConfig{})(testKind(), stubClients{}, operator.InformerOptions{})
	require.NoError(t, err)

	assert.IsType(t, &operator.ConcurrentInformer{}, inf, "handlers run on per-key workers, as under the other suppliers")
	provided, stated := operator.InformerWatchOptions(inf)
	require.True(t, stated, "the wrapper must not hide what the periodic informer states")
	assert.Equal(t,
		operator.WatchOptions{Detail: operator.WatchDetailFullObject, Triggers: operator.WatchTriggerSchedule},
		provided)
}

// Unlike a mechanism with a live stream, the list is the informer's only source, so selectors reach the server and
// nothing else has to honor them.
func TestPeriodicInformerSupplier_Listing(t *testing.T) {
	t.Run("passes the namespace and selectors to the lister", func(t *testing.T) {
		var gotOpts operator.ClientListerOptions
		supplier := PeriodicInformerSupplier(PeriodicInformerConfig{
			ResyncInterval: time.Minute,
			PageSize:       25,
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

// The whole point of the supplier: a deployment offers it as its cadence mechanism and the kinds which declare they
// need nothing per change are served by it, with no watch and no cache.
func TestPeriodicInformerSupplier_AsACadenceOffer(t *testing.T) {
	appWith := func(provides, needs operator.WatchOptions) error {
		_, err := NewApp(AppConfig{
			ClientGenerator: stubClients{},
			InformerConfig: AppInformerConfig{
				InformerSupplierDecider: NewInformerSupplierDecider(InformerSupplierOffer{
					Provides: provides,
					Supplier: PeriodicInformerSupplier(PeriodicInformerConfig{ResyncInterval: time.Hour}),
				}),
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

	scheduled := operator.WatchOptions{Detail: operator.WatchDetailFullObject, Triggers: operator.WatchTriggerSchedule}
	assert.NoError(t, appWith(scheduled, scheduled))

	perChange := operator.WatchOptions{Detail: operator.WatchDetailIdentity, Triggers: operator.WatchTriggerChange}
	err := appWith(perChange, perChange)
	require.Error(t, err, "an offer against a trigger the periodic informer cannot deliver on is refused")
	assert.Contains(t, err.Error(),
		"provides detail=full-object triggers=schedule, but its reconciler or watcher needs detail=identity triggers=change")
}
