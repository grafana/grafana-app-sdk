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
		metadataBus    = "metadata-bus"
		resyncingWatch = "resyncing-watch"
	)

	provisions := map[string]operator.WatchOptions{
		apiWatch:       {Live: &operator.WatchLiveOptions{}},
		resyncingWatch: {Live: &operator.WatchLiveOptions{}, Resync: &operator.WatchResyncOptions{}},
		eventBus: {
			Live:   &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
			Resync: &operator.WatchResyncOptions{},
		},
		hydratingBus: {Live: &operator.WatchLiveOptions{}, Resync: &operator.WatchResyncOptions{}},
		reList:       {Resync: &operator.WatchResyncOptions{}},
		// Announces the object's metadata alongside the key, which is enough for an opinionated wrapper.
		metadataBus: {
			Live:   &operator.WatchLiveOptions{Detail: operator.WatchDetailMetadata},
			Resync: &operator.WatchResyncOptions{},
		},
		// A keys-only lister, so even its resync carries bare identities.
		keysOnlyBus: {
			Live:   &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
			Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailIdentity},
		},
		// A round-robin group shares the live channel; the re-list still lists the whole kind.
		sharedEventBus: {
			Live: &operator.WatchLiveOptions{
				Detail:       operator.WatchDetailIdentity,
				Distribution: operator.WatchDistributionShared,
			},
			Resync: &operator.WatchResyncOptions{},
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
			needs:  operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name:   "a kind needing whole objects on a schedule passes over a keys-only bus offered first",
			needs:  operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
			offers: []string{keysOnlyBus, reList},
			plain:  true,
			want:   reList,
		},
		{
			// The bus's re-list reads whole objects, but its identity-only live events would reach the reconciler too.
			name:   "a kind needing whole objects on a schedule passes over a bus whose live events carry identities",
			needs:  operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
			offers: []string{eventBus, reList},
			plain:  true,
			want:   reList,
		},
		{
			name:   "an opinionated kind needing a resync passes over a bus whose live events carry identities",
			needs:  operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
			offers: []string{eventBus, reList},
			want:   reList,
		},
		{
			name: "a kind needing only a resync of identities takes whichever is offered first",
			needs: operator.WatchOptions{
				Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailIdentity},
			},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name: "a kind needing each change and a resync passes over the re-list and the API watch",
			needs: operator.WatchOptions{
				Live:   &operator.WatchLiveOptions{},
				Resync: &operator.WatchResyncOptions{},
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
			name: "a kind which lets its live work be divided takes the divided bus listed first",
			needs: operator.WatchOptions{
				Live: &operator.WatchLiveOptions{
					Detail:       operator.WatchDetailIdentity,
					Distribution: operator.WatchDistributionShared,
				},
			},
			offers: []string{sharedEventBus, eventBus},
			plain:  true,
			want:   sharedEventBus,
		},
		{
			name: "permitting division does not require it, so an undivided bus listed first serves the kind",
			needs: operator.WatchOptions{
				Live: &operator.WatchLiveOptions{
					Detail:       operator.WatchDetailIdentity,
					Distribution: operator.WatchDistributionShared,
				},
			},
			offers: []string{eventBus, sharedEventBus},
			plain:  true,
			want:   eventBus,
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
			mechanisms := make([]operator.InformerMechanism, 0, len(tt.offers)+1)
			for _, mechanism := range tt.offers {
				mechanisms = append(mechanisms, operator.NewInformerMechanism(provisions[mechanism], supplierOf(mechanism, called)))
			}
			// The watch listed last, as a deployment which wants it checked like the rest would.
			mechanisms = append(mechanisms, operator.NewInformerMechanism(provisions[apiWatch], supplierOf(apiWatch, called)))
			return AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier, mechanisms...),
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
						WatchNeeds: tt.needs,
						UsePlain:   tt.plain,
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
						WatchNeeds:     tt.needs,
						UseOpinionated: !tt.plain,
					},
				}},
			})
			assert.Equal(t, tt.want, called)
		})
	}

	t.Run("each kind's cadence and page size reach the mechanism through its needs", func(t *testing.T) {
		type resyncConfig struct {
			interval time.Duration
			pageSize int64
		}
		resolved := map[string]resyncConfig{}
		asGiven := map[string]resyncConfig{}
		needs := map[string]operator.WatchOptions{}
		record := func(kind resource.Kind, _ resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
			// What any mechanism resolves, and what the App left the app-wide fields as.
			r := opts.ResyncOptions()
			resolved[kind.Kind()] = resyncConfig{r.Interval, r.ListPageSize}
			asGiven[kind.Kind()] = resyncConfig{opts.CacheResyncInterval, opts.WatchListPageSize}
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
					WatchNeeds: operator.WatchOptions{
						Resync: &operator.WatchResyncOptions{
							Interval:     time.Hour,
							ListPageSize: 5000,
						},
					},
					UsePlain: true,
				},
			}, {
				Kind:       namedTestKind("AppWide"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
					UsePlain:   true,
				},
			}},
		})

		assert.Equal(t, map[string]resyncConfig{
			"Hourly":  {interval: time.Hour, pageSize: 5000},
			"AppWide": {interval: time.Minute, pageSize: 100},
		}, resolved)
		// The app-wide fields arrive untouched: the kind's figures are resolved from its needs, not written over
		// the configuration the deployment set.
		appWide := resyncConfig{interval: time.Minute, pageSize: 100}
		assert.Equal(t, map[string]resyncConfig{"Hourly": appWide, "AppWide": appWide}, asGiven)
		// And they reach the mechanism as declared, so one which configures itself does not read them back out of
		// the options above.
		assert.Equal(t, &operator.WatchResyncOptions{
			Detail:       operator.WatchDetailFullObject,
			Distribution: operator.WatchDistributionAll,
			Interval:     time.Hour,
			ListPageSize: 5000,
		}, needs["Hourly"].Resync)
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
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(provisions[reList], record(reList)),
					operator.NewInformerMechanism(provisions[sharedEventBus], record(sharedEventBus)),
					operator.NewInformerMechanism(provisions[resyncingWatch], record(resyncingWatch)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       namedTestKind("MyKind"),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{
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
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
					UsePlain:   true,
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
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					nil,
					operator.NewInformerMechanism(provisions[hydratingBus], supplierOf(hydratingBus, &called)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
					UsePlain:   true,
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
					WatchNeeds: operator.WatchOptions{
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
		// Only the re-list is offered, which has no live channel at all, so the one mechanism cannot satisfy the
		// kind and the recording fallback standing in for the API watch is the only thing which can have built it.
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(supplierOf(apiWatch, &called),
					operator.NewInformerMechanism(provisions[reList], supplierOf(reList, &called)),
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
		selecting := operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
			operator.NewInformerMechanism(provisions[reList], supplierOf(reList, &called)),
			operator.NewInformerMechanism(provisions[apiWatch], supplierOf(apiWatch, &called)),
		)
		bespoke := func(kind resource.Kind, clients resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
			if kind.Kind() == "Bespoke" {
				return supplierOf(hydratingBus, &called)(kind, clients, opts)
			}
			return selecting(kind, clients, opts)
		}

		for kindName, want := range map[string]string{"Bespoke": hydratingBus, "Ordinary": reList} {
			called = ""
			createTestApp(t, AppConfig{
				InformerConfig: AppInformerConfig{InformerSupplier: bespoke},
				ManagedKinds: []AppManagedKind{{
					Kind:       namedTestKind(kindName),
					Reconciler: &Reconciler{},
					ReconcileOptions: BasicReconcileOptions{
						WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
						UsePlain:   true,
					},
				}},
			})
			assert.Equal(t, want, called, "kind %s", kindName)
		}
	})

	t.Run("an opinionated handler is served by a mechanism carrying metadata", func(t *testing.T) {
		// The deletion timestamp, the finalizer list and the generation are all metadata, so a mechanism which
		// carries it serves the wrapper without the spec or the status.
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(provisions[metadataBus], supplierOf(metadataBus, &called)),
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{
						Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailMetadata},
					},
				},
			}},
		})
		assert.Equal(t, metadataBus, called)
	})

	t.Run("an opinionated handler declaring bare identities is refused", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{
						Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
					},
				},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "declares Live deliveries carrying less than the object's metadata")
		assert.Contains(t, err.Error(), "use a plain reconciler or watcher")
	})

	t.Run("two entries watching one kind are checked against each other's informers", func(t *testing.T) {
		// The informer controller keys by GroupVersionKind, so the bus selected for the first entry also delivers
		// to the second entry's reconciler, which takes the whole object.
		bus := operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
			operator.NewInformerMechanism(provisions[eventBus], supplierOf(eventBus, nil)),
		)
		identities := UnmanagedKindReconcileOptions{
			WatchNeeds: operator.WatchOptions{
				Live:   &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
				Resync: &operator.WatchResyncOptions{},
			},
		}

		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{InformerSupplier: bus},
			UnmanagedKinds: []AppUnmanagedKind{
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: identities},
				{Kind: testKind(), Reconciler: &Reconciler{}},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is watched by more than one entry")
		// The direction which matters: the first entry's bus carries identities live, and the second entry's
		// reconciler, which took the watch, reads whole objects.
		assert.Contains(t, err.Error(), `Live deliveries carrying less than "full-object"`)

		// Two entries the one mechanism serves are fine.
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{InformerSupplier: bus},
			UnmanagedKinds: []AppUnmanagedKind{
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: identities},
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: identities},
			},
		})
	})

	t.Run("entries whose informers have complementary channels are accepted", func(t *testing.T) {
		// The re-list has no live channel and the watch no resync, so neither informer provides what the other
		// entry asked for — but each entry has its own informer for that, and what they do deliver to one
		// another's handlers carries whole objects.
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(provisions[reList], supplierOf(reList, nil)),
				),
			},
			UnmanagedKinds: []AppUnmanagedKind{
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: UnmanagedKindReconcileOptions{
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{Interval: time.Hour}},
				}},
				{Kind: testKind(), Reconciler: &Reconciler{}},
			},
		})
	})

	t.Run("a mechanism carrying less on a channel the kind did not ask for is skipped for the fallback", func(t *testing.T) {
		// The kind only asks for a resync, which the bus's re-list satisfies, but its identity-only live events
		// would still reach the opinionated wrapper, so selection moves on to the fallback.
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(provisions[eventBus], supplierOf(eventBus, &called)),
				),
				InformerOptions: operator.InformerOptions{CacheResyncInterval: time.Minute},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}}},
			}},
		})
		assert.Empty(t, called)
	})

	t.Run("an informer carrying less on a channel the kind did not ask for is refused", func(t *testing.T) {
		// A whole-object live watch whose resync carries only identities, built directly rather than selected.
		lister := func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			return &optionedInformer{options: operator.WatchOptions{
				Live:   &operator.WatchLiveOptions{},
				Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailIdentity},
			}}, nil
		}
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{InformerSupplier: lister},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `makes Resync deliveries carrying less than "full-object"`)
		assert.Contains(t, err.Error(), "whether the kind asked for that channel or not")
	})

	t.Run("entries sharing a watch are refused unsharded deliveries to a handler requiring sharding", func(t *testing.T) {
		sharded := operator.WatchOptions{Live: &operator.WatchLiveOptions{Distribution: operator.WatchDistributionSharded}}
		shardedBus := func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			return &optionedInformer{options: sharded}, nil
		}
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(sharded, shardedBus),
				),
			},
			UnmanagedKinds: []AppUnmanagedKind{
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: UnmanagedKindReconcileOptions{WatchNeeds: sharded}},
				{Kind: testKind(), Reconciler: &Reconciler{}},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is watched by more than one entry")
		assert.Contains(t, err.Error(), `Live deliveries which are not "sharded"`)
	})

	t.Run("an opinionated watcher declaring a resync is refused", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{CacheResyncInterval: time.Minute},
			},
			UnmanagedKinds: []AppUnmanagedKind{{
				Kind:    testKind(),
				Watcher: &Watcher{},
				ReconcileOptions: UnmanagedKindReconcileOptions{
					UseOpinionated: true,
					WatchNeeds:     operator.WatchOptions{Live: &operator.WatchLiveOptions{}, Resync: &operator.WatchResyncOptions{}},
				},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "declares a resync, which its opinionated watcher would never act on")

		// The opinionated reconciler passes resyncs on, so the same declaration is served for one.
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{CacheResyncInterval: time.Minute},
			},
			UnmanagedKinds: []AppUnmanagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: UnmanagedKindReconcileOptions{
					UseOpinionated: true,
					WatchNeeds:     operator.WatchOptions{Live: &operator.WatchLiveOptions{}, Resync: &operator.WatchResyncOptions{}},
				},
			}},
		})
	})

	t.Run("a kind needing a resync with no cadence anywhere is refused, naming both knobs", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
					UsePlain:   true,
				},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resync=none")
		assert.Contains(t, err.Error(), "WatchNeeds.Resync.Interval")
		assert.Contains(t, err.Error(), "CacheResyncInterval")
	})

	t.Run("a kind asking for a page size the mechanism ignores is still served", func(t *testing.T) {
		// ListPageSize takes no part in SatisfiedBy, so a mechanism ignoring it is not a failure.
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{CacheResyncInterval: time.Minute},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{ListPageSize: 5000}},
					UsePlain:   true,
				},
			}},
		})
	})

	t.Run("a mechanism's advertisement is checked even when its informer says nothing", func(t *testing.T) {
		// The first entry takes the bus, whose informer describes nothing; its identity-only live events would
		// otherwise reach the second entry's reconciler unchecked.
		silent := func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			return &mockInformer{}, nil
		}
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					nil,
					operator.NewInformerMechanism(provisions[eventBus], silent),
				),
			},
			UnmanagedKinds: []AppUnmanagedKind{
				{Kind: testKind(), Reconciler: &Reconciler{}, ReconcileOptions: UnmanagedKindReconcileOptions{
					WatchNeeds: operator.WatchOptions{Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity}},
				}},
				{Kind: testKind(), Reconciler: &Reconciler{}},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Live deliveries carrying less than "full-object"`)
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
					WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}},
					UsePlain:   true,
				},
			}},
		})
	})

	t.Run("an informer which provides less than the kind needs is refused", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				// Advertised as the hydrating bus, but its informer says it only re-lists.
				InformerSupplier: operator.NewSelectingInformerSupplier(DefaultInformerSupplier,
					operator.NewInformerMechanism(provisions[hydratingBus], supplierOf(reList, nil)),
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
			"provides live=none resync=full-object/all, but its reconciler or watcher needs live=full-object/all resync=none")
	})

	t.Run("an opinionated handler is refused an informer which carries only bare identities", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{InformerSupplier: supplierOf(eventBus, nil)},
			UnmanagedKinds: []AppUnmanagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: UnmanagedKindReconcileOptions{UseOpinionated: true},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "carries less than the object's metadata on its Live channel")
	})
}

type optionedInformer struct {
	mockInformer
	options operator.WatchOptions
}

func (o *optionedInformer) WatchOptions() operator.WatchOptions {
	return o.options
}

// An opinionated kind may set resync checks, and they keep objects without its finalizer.
func TestApp_OpinionatedKindMayIgnoreObjects(t *testing.T) {
	var received operator.InformerOptions
	supplier := func(_ resource.Kind, _ resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		received = options
		return &optionedInformer{options: operator.WatchOptions{Resync: &operator.WatchResyncOptions{}}}, nil
	}
	createTestApp(t, AppConfig{
		InformerConfig: AppInformerConfig{
			InformerSupplier: supplier,
			InformerOptions:  operator.InformerOptions{CacheResyncInterval: time.Minute},
		},
		ManagedKinds: []AppManagedKind{{
			Kind:       testKind(),
			Reconciler: &Reconciler{},
			ReconcileOptions: BasicReconcileOptions{WatchNeeds: operator.WatchOptions{Resync: &operator.WatchResyncOptions{
				Ignore: func(resource.Object) bool { return true },
			}}},
		}},
	})
	require.NotNil(t, received.WatchNeeds.Resync)
	unfinished := &resource.UntypedObject{}
	assert.False(t, received.WatchNeeds.Resync.Ignore(unfinished), "an object without the reconciler's finalizer is kept")
}
