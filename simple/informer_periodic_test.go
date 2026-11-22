package simple

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

type periodicClients struct{}

func (periodicClients) ClientFor(resource.Kind) (resource.Client, error) {
	return periodicClient{}, nil
}

func (periodicClients) GetCustomRouteClient(schema.GroupVersion, string) (resource.CustomRouteClient, error) {
	return nil, nil
}

func (periodicClients) DiscoveryClient() (resource.DiscoveryClient, error) { return nil, nil }

type periodicClient struct{ resource.Client }

func (periodicClient) ListInto(context.Context, string, resource.ListOptions, resource.ListObject) error {
	return nil
}

// Through a real App: the kinds which only resync are served by the re-list, a kind needing the live channel is
// passed over to the watch, and one handed the re-list outright is refused.
func TestApp_PeriodicMechanism(t *testing.T) {
	appWith := func(needs operator.WatchOptions, supplier InformerSupplier) error {
		_, err := NewApp(AppConfig{
			ClientGenerator: periodicClients{},
			InformerConfig:  AppInformerConfig{InformerSupplier: supplier},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: needs,
					UsePlain:   true,
				},
			}},
		})
		return err
	}
	relist := operator.NewPeriodicMechanism(operator.PeriodicMechanismOptions{ResyncInterval: time.Hour})
	selecting := operator.NewSelectingInformerSupplier(DefaultInformerSupplier, relist)
	live := operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}}

	assert.NoError(t, appWith(operator.WatchOptions{Resync: &operator.WatchResyncOptions{Interval: time.Hour}}, selecting))
	assert.NoError(t, appWith(live, selecting), "a kind needing the live channel is watched instead")

	err := appWith(live, relist.NewInformer)
	require.Error(t, err, "handed the re-list outright, the check refuses it")
	assert.Contains(t, err.Error(),
		"provides live=none resync=full-object/all, but its reconciler or watcher needs live=identity/all resync=none")
}

// The figures a kind declares reach the mechanism the App selected, which is what lets one mechanism serve kinds
// resyncing at different rates.
func TestApp_PassesWatchNeedsToTheMechanism(t *testing.T) {
	var got operator.InformerOptions
	recording := operator.NewInformerMechanism(
		operator.PeriodicInformerWatchOptions(),
		func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
			got = options
			return operator.NewPeriodicMechanism(operator.PeriodicMechanismOptions{}).NewInformer(kind, clients, options)
		},
	)
	_, err := NewApp(AppConfig{
		ClientGenerator: periodicClients{},
		InformerConfig: AppInformerConfig{
			InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier, recording),
		},
		ManagedKinds: []AppManagedKind{{
			Kind:       testKind(),
			Reconciler: &Reconciler{},
			ReconcileOptions: BasicReconcileOptions{
				WatchNeeds: operator.WatchOptions{
					Resync: &operator.WatchResyncOptions{Interval: 90 * time.Minute, ListPageSize: 25},
				},
				UsePlain: true,
			},
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, got.WatchNeeds.Resync)
	assert.Equal(t, 90*time.Minute, got.WatchNeeds.Resync.Interval)
	assert.Equal(t, int64(25), got.WatchNeeds.Resync.ListPageSize)
}
