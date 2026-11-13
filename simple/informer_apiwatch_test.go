package simple

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

type failingClients struct {
	stubClients
	err error
}

func (f failingClients) ClientFor(resource.Kind) (resource.Client, error) { return nil, f.err }

// The watch is the mechanism every other offer is measured against, so what it states has to be read off the
// informer the supplier actually built, wrapper and all.
func TestAPIWatchInformerSupplier_Provides(t *testing.T) {
	tests := []struct {
		name           string
		cfg            APIWatchInformerConfig
		resyncInterval time.Duration
		wantInformer   any
		want           operator.WatchOptions
	}{
		{
			name:         "the default informer delivers each change",
			wantInformer: &operator.KubernetesBasedInformer{},
			want: operator.WatchOptions{
				Detail:   operator.WatchDetailFullObject,
				Triggers: operator.WatchTriggerChange,
			},
		},
		{
			name:           "a cache resync adds the scheduled sweep",
			resyncInterval: time.Hour,
			wantInformer:   &operator.KubernetesBasedInformer{},
			want: operator.WatchOptions{
				Detail:   operator.WatchDetailFullObject,
				Triggers: operator.WatchTriggerChange | operator.WatchTriggerSchedule,
			},
		},
		{
			name:         "the optimized informer delivers each change",
			cfg:          APIWatchInformerConfig{Optimized: true},
			wantInformer: &operator.CustomCacheInformer{},
			want: operator.WatchOptions{
				Detail:   operator.WatchDetailFullObject,
				Triggers: operator.WatchTriggerChange,
			},
		},
		{
			name:           "the optimized informer sweeps on a cache resync too",
			cfg:            APIWatchInformerConfig{Optimized: true},
			resyncInterval: time.Hour,
			wantInformer:   &operator.CustomCacheInformer{},
			want: operator.WatchOptions{
				Detail:   operator.WatchDetailFullObject,
				Triggers: operator.WatchTriggerChange | operator.WatchTriggerSchedule,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inf, err := APIWatchInformerSupplier(tt.cfg)(testKind(), stubClients{}, operator.InformerOptions{
				CacheResyncInterval: tt.resyncInterval,
			})
			require.NoError(t, err)

			require.IsType(t, &operator.ConcurrentInformer{}, inf)
			assert.IsType(t, tt.wantInformer, inf.(operator.InformerWrapper).Unwrap())

			provided, stated := operator.InformerWatchOptions(inf)
			require.True(t, stated, "the API watch must say what it provides, so a kind can be checked against it")
			assert.Equal(t, tt.want, provided)
		})
	}
}

func TestAPIWatchInformerSupplier_ClientErrors(t *testing.T) {
	t.Run("a client error fails informer creation", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		_, err := DefaultInformerSupplier(testKind(), failingClients{err: expected}, operator.InformerOptions{})
		assert.ErrorIs(t, err, expected)
	})

	t.Run("a nil client generator is refused rather than panicking", func(t *testing.T) {
		_, err := DefaultInformerSupplier(testKind(), nil, operator.InformerOptions{})
		assert.Error(t, err)
	})
}

// The kinds an App serves with the watch are now checked against it, which is the point of the supplier stating
// anything: a sweep the deployment never configured is refused rather than waited for.
func TestAPIWatchInformerSupplier_SweepNeedsACacheResync(t *testing.T) {
	appWith := func(resyncInterval time.Duration) error {
		_, err := NewApp(AppConfig{
			ClientGenerator: stubClients{},
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{CacheResyncInterval: resyncInterval},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Triggers: operator.WatchTriggerChange | operator.WatchTriggerSchedule,
					},
					UsePlain: true,
				},
			}},
		})
		return err
	}

	err := appWith(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"provides detail=full-object triggers=change, but its reconciler or watcher needs detail=full-object triggers=change+schedule")

	assert.NoError(t, appWith(time.Hour))
}
