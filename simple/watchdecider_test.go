package simple

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestApp_WatchOptions(t *testing.T) {
	// Each mechanism is labelled so a kind served by the API watch is told apart from one served by an offer which
	// provides the same options.
	const (
		apiWatch       = "api-watch"
		eventBus       = "event-bus"
		hydratingBus   = "hydrating-bus"
		reList         = "re-list"
		sharedEventBus = "shared-event-bus"
		keysOnlyBus    = "keys-only-bus"
		resyncingWatch = "resyncing-watch"
	)

	provisions := map[string]operator.WatchOptions{
		apiWatch:       {Live: &operator.WatchLiveOptions{}},
		resyncingWatch: {Live: &operator.WatchLiveOptions{}, Sweep: &operator.WatchSweepOptions{}},
		eventBus: {
			Live:  &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
			Sweep: &operator.WatchSweepOptions{},
		},
		hydratingBus: {Live: &operator.WatchLiveOptions{}, Sweep: &operator.WatchSweepOptions{}},
		reList:       {Sweep: &operator.WatchSweepOptions{}},
		// A keys-only lister, so even its sweep carries identities.
		keysOnlyBus: {
			Live:  &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
			Sweep: &operator.WatchSweepOptions{Detail: operator.WatchDetailIdentity},
		},
		// A round-robin group shares the live channel; the re-list still lists the whole kind.
		sharedEventBus: {
			Live: &operator.WatchLiveOptions{
				Detail:       operator.WatchDetailIdentity,
				Distribution: operator.WatchDistributionShared,
			},
			Sweep: &operator.WatchSweepOptions{},
		},
	}

	supplierOf := func(mechanism string, called *string) InformerSupplier {
		return func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			if called != nil {
				*called = mechanism
			}
			return &optionedInformer{options: provisions[mechanism]}, nil
		}
	}

	tests := []struct {
		name   string
		needs  operator.WatchOptions
		offers []string
		plain  bool
		want   string
	}{
		{
			name:   "an unset need is served by the API watch",
			offers: []string{reList, eventBus},
			plain:  true,
			want:   apiWatch,
		},
		{
			name:   "an identity need takes the event bus",
			needs:  operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   eventBus,
		},
		{
			name:   "a kind needing whole objects per change takes the hydrating bus over the API watch",
			offers: []string{reList, eventBus, hydratingBus},
			plain:  true,
			want:   hydratingBus,
		},
		{
			name:   "a kind needing whole objects on a schedule takes the re-list",
			needs:  operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name:   "a kind needing whole objects on a schedule passes over a keys-only bus offered first",
			needs:  operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
			offers: []string{keysOnlyBus, reList},
			plain:  true,
			want:   reList,
		},
		{
			name:   "a kind needing whole objects on a schedule is served by a bus whose re-list reads them",
			needs:  operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
			offers: []string{eventBus, reList},
			plain:  true,
			want:   eventBus,
		},
		{
			name: "a kind needing only a sweep of identities takes whichever is offered first",
			needs: operator.WatchOptions{
				Sweep: &operator.WatchSweepOptions{Detail: operator.WatchDetailIdentity},
			},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name: "a kind needing each change and a sweep passes over the re-list and the API watch",
			needs: operator.WatchOptions{
				Live:  &operator.WatchLiveOptions{},
				Sweep: &operator.WatchSweepOptions{},
			},
			offers: []string{reList, hydratingBus},
			plain:  true,
			want:   hydratingBus,
		},
		{
			name:   "an identity need with no bus offered falls back to the API watch",
			needs:  operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
			offers: []string{reList},
			plain:  true,
			want:   apiWatch,
		},
		{
			name:   "an opinionated handler needs whole objects, so an identity bus is passed over",
			needs:  operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
			offers: []string{eventBus, hydratingBus},
			want:   hydratingBus,
		},
		{
			name: "a kind which lets its live work be divided takes the shared bus",
			needs: operator.WatchOptions{
				Live: &operator.WatchLiveOptions{
					Detail:       operator.WatchDetailIdentity,
					Distribution: operator.WatchDistributionShared,
				},
			},
			offers: []string{eventBus, sharedEventBus},
			plain:  true,
			want:   sharedEventBus,
		},
		{
			name:   "a kind which says nothing about its live distribution passes over the shared bus",
			needs:  operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
			offers: []string{sharedEventBus, eventBus},
			plain:  true,
			want:   eventBus,
		},
	}

	for _, tt := range tests {
		informerConfig := func(called *string) AppInformerConfig {
			mechanisms := make([]InformerMechanism, 0, len(tt.offers))
			for _, mechanism := range tt.offers {
				mechanisms = append(mechanisms, NewInformerMechanism(provisions[mechanism], supplierOf(mechanism, called)))
			}
			return AppInformerConfig{
				InformerSupplier: NewSelectingInformerSupplier(supplierOf(apiWatch, called), mechanisms...),
			}
		}

		t.Run("managed kind: "+tt.name, func(t *testing.T) {
			var called string
			createTestApp(t, AppConfig{
				InformerConfig: informerConfig(&called),
				ManagedKinds: []AppManagedKind{{
					Kind:       testKind(),
					Reconciler: &Reconciler{},
					ReconcileOptions: BasicReconcileOptions{
						WatchOptions: tt.needs,
						UsePlain:     tt.plain,
					},
				}},
			})
			assert.Equal(t, tt.want, called)
		})

		t.Run("unmanaged kind: "+tt.name, func(t *testing.T) {
			var called string
			createTestApp(t, AppConfig{
				InformerConfig: informerConfig(&called),
				UnmanagedKinds: []AppUnmanagedKind{{
					Kind:       testKind(),
					Reconciler: &Reconciler{},
					ReconcileOptions: UnmanagedKindReconcileOptions{
						WatchOptions:   tt.needs,
						UseOpinionated: !tt.plain,
					},
				}},
			})
			assert.Equal(t, tt.want, called)
		})
	}

	t.Run("each kind gets the cadence and page size it asked for", func(t *testing.T) {
		type sweepConfig struct {
			interval time.Duration
			pageSize int64
		}
		configs := map[string]sweepConfig{}
		needs := map[string]operator.WatchOptions{}
		record := func(kind resource.Kind, _ resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
			configs[kind.Kind()] = sweepConfig{opts.CacheResyncInterval, opts.WatchListPageSize}
			needs[kind.Kind()] = opts.WatchNeeds
			return &mockInformer{}, nil
		}

		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{
					CacheResyncInterval: time.Minute,
					WatchListPageSize:   100,
				},
				InformerSupplier: record,
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       namedTestKind("Hourly"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Sweep: &operator.WatchSweepOptions{
							Every:    time.Hour,
							PageSize: 5000,
						},
					},
					UsePlain: true,
				},
			}, {
				Kind:       namedTestKind("AppWide"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
					UsePlain:     true,
				},
			}},
		})

		assert.Equal(t, map[string]sweepConfig{
			"Hourly":  {interval: time.Hour, pageSize: 5000},
			"AppWide": {interval: time.Minute, pageSize: 100},
		}, configs)
		// The needs reach the mechanism as declared, so one which configures itself does not read them back out
		// of the options above.
		assert.Equal(t, &operator.WatchSweepOptions{
			Detail:       operator.WatchDetailFullObject,
			Distribution: operator.WatchDistributionAll,
			Every:        time.Hour,
			PageSize:     5000,
		}, needs["Hourly"].Sweep)
		assert.Nil(t, needs["Hourly"].Live)
	})

	// Mirrors the worked example in docs/writing-a-reconciler.md, so the walk-through there cannot drift.
	t.Run("the documented example selects as documented", func(t *testing.T) {
		served := map[string]string{}
		record := func(mechanism string) InformerSupplier {
			return func(kind resource.Kind, _ resource.ClientGenerator, _ operator.InformerOptions) (operator.Informer, error) {
				served[kind.Kind()] = mechanism
				return &optionedInformer{options: provisions[mechanism]}, nil
			}
		}

		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: NewSelectingInformerSupplier(
					record(apiWatch),
					NewInformerMechanism(provisions[reList], record(reList)),
					NewInformerMechanism(provisions[sharedEventBus], record(sharedEventBus)),
					NewInformerMechanism(provisions[resyncingWatch], record(resyncingWatch)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       namedTestKind("MyKind"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Live: &operator.WatchLiveOptions{
							Detail:       operator.WatchDetailIdentity,
							Distribution: operator.WatchDistributionShared,
						},
					},
					UsePlain: true,
				},
			}, {
				Kind:       namedTestKind("MyCleanupKind"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
					UsePlain:     true,
				},
			}},
		})

		assert.Equal(t, map[string]string{
			"MyKind":        sharedEventBus,
			"MyCleanupKind": reList,
		}, served)
	})

	t.Run("a nil mechanism is passed over", func(t *testing.T) {
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: NewSelectingInformerSupplier(
					supplierOf(apiWatch, &called),
					nil,
					NewInformerMechanism(provisions[hydratingBus], supplierOf(hydratingBus, &called)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
					UsePlain:     true,
				},
			}},
		})
		assert.Equal(t, hydratingBus, called)
	})

	t.Run("the supplier is handed the kind and its needs", func(t *testing.T) {
		var suppliedFor string
		var suppliedNeeds operator.WatchOptions
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(kind resource.Kind, _ resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
					suppliedFor = kind.GroupVersionKind().String()
					suppliedNeeds = opts.WatchNeeds
					return &mockInformer{}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
					},
					UsePlain: true,
				},
			}},
		})
		assert.Equal(t, testKind().GroupVersionKind().String(), suppliedFor)
		assert.Equal(t, operator.WatchOptions{
			Live: &operator.WatchLiveOptions{
				Detail:       operator.WatchDetailIdentity,
				Distribution: operator.WatchDistributionAll,
			},
		}, suppliedNeeds)
	})

	t.Run("a kind no mechanism satisfies is built by the fallback", func(t *testing.T) {
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: NewSelectingInformerSupplier(
					supplierOf(apiWatch, &called),
					NewInformerMechanism(provisions[reList], supplierOf(reList, &called)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
		assert.Equal(t, apiWatch, called)
	})

	t.Run("an unset supplier falls back to the API watch", func(t *testing.T) {
		createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
	})

	t.Run("a supplier can answer for one kind and defer to the mechanisms for the rest", func(t *testing.T) {
		var called string
		selecting := NewSelectingInformerSupplier(
			supplierOf(apiWatch, &called),
			NewInformerMechanism(provisions[reList], supplierOf(reList, &called)),
		)
		bespoke := func(kind resource.Kind, clients resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
			if kind.Kind() == "Bespoke" {
				return supplierOf(eventBus, &called)(kind, clients, opts)
			}
			return selecting(kind, clients, opts)
		}

		for kindName, want := range map[string]string{"Bespoke": eventBus, "Ordinary": reList} {
			called = ""
			createTestApp(t, AppConfig{
				InformerConfig: AppInformerConfig{InformerSupplier: bespoke},
				ManagedKinds: []AppManagedKind{{
					Kind:       namedTestKind(kindName),
					Reconciler: &Reconciler{},
					ReconcileOptions: BasicReconcileOptions{
						WatchOptions: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
						UsePlain:     true,
					},
				}},
			})
			assert.Equal(t, want, called, "kind %s", kindName)
		}
	})

	t.Run("an informer which says nothing about what it provides is not checked", func(t *testing.T) {
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
					return &mockInformer{}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Sweep: &operator.WatchSweepOptions{}},
					UsePlain:     true,
				},
			}},
		})
	})

	t.Run("an informer which provides less than the kind needs is refused", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				// Advertised as the hydrating bus, but its informer says it only re-lists.
				InformerSupplier: NewSelectingInformerSupplier(
					nil,
					NewInformerMechanism(provisions[hydratingBus], supplierOf(reList, nil)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			"provides live=none sweep=full-object/all, but its reconciler or watcher needs live=full-object/all sweep=none")
	})

	t.Run("an opinionated handler refuses an informer which provides only identities", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: supplierOf(eventBus, nil),
			},
			UnmanagedKinds: []AppUnmanagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: UnmanagedKindReconcileOptions{
					WatchOptions: operator.WatchOptions{
						Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
					},
					UseOpinionated: true,
				},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "but its opinionated reconciler or watcher needs live=full-object/all")
	})
}

type optionedInformer struct {
	mockInformer
	options operator.WatchOptions
}

func (o *optionedInformer) WatchOptions() operator.WatchOptions {
	return o.options
}
