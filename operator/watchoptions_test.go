package operator

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestWatchOptions_SatisfiedBy(t *testing.T) {
	var (
		apiWatch       = WatchOptions{Live: &WatchLiveOptions{}}
		resyncingWatch = WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{}}
		// Bare identities live, whole objects on the re-list: a bus whose payload is the key alone.
		identityBus = WatchOptions{
			Live:   &WatchLiveOptions{Detail: WatchDetailIdentity},
			Resync: &WatchResyncOptions{},
		}
		// The same with the object's metadata announced alongside the key.
		metadataBus = WatchOptions{
			Live:   &WatchLiveOptions{Detail: WatchDetailMetadata},
			Resync: &WatchResyncOptions{},
		}
		// The same bus with a round-robin group, which shares the live channel only.
		sharedBus = WatchOptions{
			Live: &WatchLiveOptions{
				Detail:       WatchDetailIdentity,
				Distribution: WatchDistributionShared,
			},
			Resync: &WatchResyncOptions{},
		}
		// A keys-only lister, so even its resync carries identities.
		keysOnlyBus = WatchOptions{
			Live:   &WatchLiveOptions{Detail: WatchDetailIdentity},
			Resync: &WatchResyncOptions{Detail: WatchDetailIdentity},
		}
		reList = WatchOptions{Resync: &WatchResyncOptions{}}
	)

	tests := []struct {
		name     string
		needs    WatchOptions
		provided WatchOptions
		want     bool
	}{
		{
			name:     "a kind which declares no channel is satisfied by the API watch",
			provided: apiWatch,
			want:     true,
		},
		{
			name:     "a kind which declares no channel is not satisfied by identities on the live channel",
			provided: identityBus,
			want:     false,
		},
		{
			name:     "a kind which declares no channel is not satisfied by a re-list, which has no live channel",
			provided: reList,
			want:     false,
		},
		{
			name:     "an identity need on the live channel is satisfied by whole objects",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}},
			provided: apiWatch,
			want:     true,
		},
		{
			name:     "needing both channels is not satisfied by a watch which does not resync",
			needs:    WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{}},
			provided: apiWatch,
			want:     false,
		},
		{
			name:     "needing both channels is satisfied by a resyncing watch",
			needs:    WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{}},
			provided: resyncingWatch,
			want:     true,
		},
		{
			name:     "needing only a resync is satisfied by a re-list",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: reList,
			want:     true,
		},
		{
			name:     "needing only a resync is not satisfied by a watch which does not resync",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: apiWatch,
			want:     false,
		},
		{
			// Its identity-only live events still reach a handler which declared whole objects.
			name:     "needing only a whole-object resync is not satisfied by a bus whose live events carry identities",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: identityBus,
			want:     false,
		},
		{
			name:     "needing only an identity resync is satisfied by a bus whose live events carry identities",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Detail: WatchDetailIdentity}},
			provided: identityBus,
			want:     true,
		},
		{
			name:     "a whole-object live need is not satisfied by a watch whose resync carries identities",
			needs:    WatchOptions{Live: &WatchLiveOptions{}},
			provided: WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{Detail: WatchDetailIdentity}},
			want:     false,
		},
		{
			name:  "a sharded live need is not satisfied by a mechanism whose resync goes to every instance",
			needs: WatchOptions{Live: &WatchLiveOptions{Distribution: WatchDistributionSharded}},
			provided: WatchOptions{
				Live:   &WatchLiveOptions{Distribution: WatchDistributionSharded},
				Resync: &WatchResyncOptions{},
			},
			want: false,
		},
		{
			name:     "needing whole objects on a resync is not satisfied by a keys-only lister",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: keysOnlyBus,
			want:     false,
		},
		{
			name:     "a shared live channel does not serve a kind which says nothing about it",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}},
			provided: sharedBus,
			want:     false,
		},
		{
			name: "a shared live channel serves a kind which allows its live work to be divided",
			needs: WatchOptions{Live: &WatchLiveOptions{
				Detail:       WatchDetailIdentity,
				Distribution: WatchDistributionShared,
			}},
			provided: sharedBus,
			want:     true,
		},
		{
			name:     "a shared live channel's distribution is not compared for a kind which only resyncs",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Detail: WatchDetailIdentity}},
			provided: sharedBus,
			want:     true,
		},
		{
			name:     "a kind which needs its resync sharded is not served by a bus which lists everything",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionSharded}},
			provided: sharedBus,
			want:     false,
		},
		{
			name:     "a kind which only permits its resync to be divided is served by an undivided one",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Detail: WatchDetailIdentity, Distribution: WatchDistributionShared}},
			provided: sharedBus,
			want:     true,
		},
		{
			name:     "an unrecognized detail is needed in full",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetail("other")}},
			provided: identityBus,
			want:     false,
		},
		{
			name:     "an unrecognized advertisement satisfies nothing",
			provided: WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetail("identities")}},
			want:     false,
		},
		{
			name:     "a metadata need is satisfied by metadata",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailMetadata}},
			provided: metadataBus,
			want:     true,
		},
		{
			name:     "a metadata need is not satisfied by bare identities",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailMetadata}},
			provided: identityBus,
			want:     false,
		},
		{
			name:     "an identity need is satisfied by metadata",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}},
			provided: metadataBus,
			want:     true,
		},
		{
			name:     "a cadence and a page size are not compared",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Interval: time.Hour, ListPageSize: 500}},
			provided: reList,
			want:     true,
		},
		{
			name:     "Ignore is a request, so any resync serves it",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Ignore: AlreadyObserved(nil)}},
			provided: reList,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.needs.SatisfiedBy(tt.provided))
		})
	}
}

func TestWatchOptions_RefusedDeliveries(t *testing.T) {
	// A missing or divided channel takes nothing away from a handler with its own informer for it.
	assert.Empty(t, WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{}}.RefusedDeliveries(
		WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionShared}}))

	assert.Equal(t, []string{`Resync deliveries carrying less than "full-object"`},
		WatchOptions{}.RefusedDeliveries(WatchOptions{
			Live:   &WatchLiveOptions{},
			Resync: &WatchResyncOptions{Detail: WatchDetailIdentity},
		}))
	assert.Equal(t, []string{`Live deliveries which are not "sharded"`},
		WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionSharded}}.RefusedDeliveries(
			WatchOptions{Live: &WatchLiveOptions{}}))
}

func TestWatchOptions_String(t *testing.T) {
	assert.Equal(t, "live=full-object/all resync=none", WatchOptions{}.String())
	assert.Equal(t, "live=identity/shared resync=full-object/all", WatchOptions{
		Live: &WatchLiveOptions{
			Detail:       WatchDetailIdentity,
			Distribution: WatchDistributionShared,
		},
		Resync: &WatchResyncOptions{},
	}.String())
	assert.Equal(t, "live=none resync=full-object/all", WatchOptions{
		Resync: &WatchResyncOptions{Interval: time.Hour, ListPageSize: 500},
	}.String())
	assert.Equal(t, "live=none resync=full-object/all/ignore/ignore-unchanged", WatchOptions{
		Resync: &WatchResyncOptions{Ignore: AlreadyObserved(nil), IgnoreUnchanged: SameGeneration},
	}.String())
}

func TestWatchOptions_WithDefaults_keepsTheResyncChecks(t *testing.T) {
	declared := WatchOptions{Resync: &WatchResyncOptions{Ignore: AlreadyObserved(nil), IgnoreUnchanged: SameGeneration}}
	defaulted := declared.WithDefaults().Resync
	assert.NotNil(t, defaulted.Ignore)
	assert.NotNil(t, defaulted.IgnoreUnchanged)
}

func TestWatchDetail_SatisfiedBy(t *testing.T) {
	// Ordered: the whole object serves every need, metadata serves metadata and identity, identity serves only
	// identity.
	assert.True(t, WatchDetailFullObject.SatisfiedBy(WatchDetailFullObject))
	assert.True(t, WatchDetailFullObject.SatisfiedBy(""))
	assert.True(t, WatchDetailMetadata.SatisfiedBy(WatchDetailFullObject))
	assert.True(t, WatchDetailMetadata.SatisfiedBy(WatchDetailMetadata))
	assert.True(t, WatchDetailIdentity.SatisfiedBy(WatchDetailMetadata))
	assert.True(t, WatchDetailIdentity.SatisfiedBy(WatchDetailIdentity))
	assert.False(t, WatchDetailFullObject.SatisfiedBy(WatchDetailMetadata))
	assert.False(t, WatchDetailFullObject.SatisfiedBy(WatchDetailIdentity))
	assert.False(t, WatchDetailMetadata.SatisfiedBy(WatchDetailIdentity))
	// A mistyped advertisement must not pass for whole objects, but two sides naming the same unknown agree.
	assert.False(t, WatchDetailFullObject.SatisfiedBy(WatchDetail("metadata-only")))
	assert.False(t, WatchDetailMetadata.SatisfiedBy(WatchDetail("partial")))
	assert.True(t, WatchDetail("partial").SatisfiedBy(WatchDetail("partial")))
}

func TestWatchDistribution_SatisfiedBy(t *testing.T) {
	tests := []struct {
		needed   WatchDistribution
		provided WatchDistribution
		want     bool
	}{
		{needed: WatchDistributionAll, provided: WatchDistributionAll, want: true},
		{needed: WatchDistributionAll, provided: WatchDistributionShared, want: false},
		{needed: WatchDistributionAll, provided: WatchDistributionSharded, want: false},
		{needed: WatchDistributionShared, provided: WatchDistributionShared, want: true},
		{needed: WatchDistributionShared, provided: WatchDistributionSharded, want: true},
		// Shared permits division rather than requiring it, so an undivided channel serves it: a kind which asked
		// for its work to be done once is not harmed by every instance doing it, which is what it gets today.
		{needed: WatchDistributionShared, provided: WatchDistributionAll, want: true},
		{needed: WatchDistributionSharded, provided: WatchDistributionSharded, want: true},
		{needed: WatchDistributionSharded, provided: WatchDistributionShared, want: false},
		{needed: WatchDistributionSharded, provided: WatchDistributionAll, want: false},
		// Unset means all on either side, as WatchOptions.SatisfiedBy reads it.
		{needed: WatchDistributionAll, provided: "", want: true},
		{needed: "", provided: WatchDistributionAll, want: true},
		{needed: "", provided: WatchDistributionShared, want: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.needed)+" needed, "+string(tt.provided)+" provided", func(t *testing.T) {
			assert.Equal(t, tt.want, tt.needed.SatisfiedBy(tt.provided))
		})
	}
}

func TestInformerWatchOptions(t *testing.T) {
	t.Run("an informer which says nothing states no options", func(t *testing.T) {
		options, stated := InformerWatchOptions(&mockInformer{})
		assert.False(t, stated)
		assert.Equal(t, WatchOptions{}, options)
	})

	t.Run("an informer wrapping one which states options is unwrapped", func(t *testing.T) {
		want := WatchOptions{Resync: &WatchResyncOptions{}}
		options, stated := InformerWatchOptions(&wrappingInformer{wrapped: &optionedInformer{options: want}})
		assert.True(t, stated)
		assert.Equal(t, want, options)
	})

	t.Run("an informer wrapping one which says nothing says nothing", func(t *testing.T) {
		_, stated := InformerWatchOptions(&wrappingInformer{wrapped: &mockInformer{}})
		assert.False(t, stated)
	})

	t.Run("an informer which says so states its own options", func(t *testing.T) {
		want := WatchOptions{Resync: &WatchResyncOptions{}}
		options, stated := InformerWatchOptions(&optionedInformer{options: want})
		assert.True(t, stated)
		assert.Equal(t, want, options)
	})
}

type optionedInformer struct {
	mockInformer
	options WatchOptions
}

func (o *optionedInformer) WatchOptions() WatchOptions {
	return o.options
}

func TestInformerOptions_ResyncOptions(t *testing.T) {
	tests := []struct {
		name    string
		options InformerOptions
		want    *WatchResyncOptions
	}{
		{
			name: "nothing asked for anything",
		},
		{
			name:    "a page size alone does not make a resync",
			options: InformerOptions{WatchListPageSize: 500},
		},
		{
			name:    "the app-wide interval with no needs stated",
			options: InformerOptions{CacheResyncInterval: time.Minute, WatchListPageSize: 100},
			want:    &WatchResyncOptions{Interval: time.Minute, ListPageSize: 100},
		},
		{
			name: "a nil Resync does not switch the app-wide interval off",
			options: InformerOptions{
				CacheResyncInterval: time.Minute,
				WatchNeeds:          WatchOptions{Live: &WatchLiveOptions{}},
			},
			want: &WatchResyncOptions{Interval: time.Minute},
		},
		{
			name: "what the kind asked for wins",
			options: InformerOptions{
				CacheResyncInterval: time.Minute,
				WatchListPageSize:   100,
				WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{
					Detail:       WatchDetailIdentity,
					Interval:     time.Hour,
					ListPageSize: 5000,
				}},
			},
			want: &WatchResyncOptions{
				Detail:       WatchDetailIdentity,
				Interval:     time.Hour,
				ListPageSize: 5000,
			},
		},
		{
			name: "each field falls back on its own",
			options: InformerOptions{
				CacheResyncInterval: time.Minute,
				WatchListPageSize:   100,
				WatchNeeds:          WatchOptions{Resync: &WatchResyncOptions{ListPageSize: 5000}},
			},
			want: &WatchResyncOptions{Interval: time.Minute, ListPageSize: 5000},
		},
		{
			name:    "a resync asked for with no cadence anywhere resolves to no interval",
			options: InformerOptions{WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{}}},
			want:    &WatchResyncOptions{},
		},
		{
			name:    "a negative interval is passed through for its reader to clamp",
			options: InformerOptions{CacheResyncInterval: -time.Minute},
			want:    &WatchResyncOptions{Interval: -time.Minute},
		},
		{
			// A negative is a figure the kind stated, so the app-wide one does not replace it, and no cadence is
			// what it resolves to.
			name: "a negative figure stated by the kind does not fall back to the app-wide one",
			options: InformerOptions{
				CacheResyncInterval: time.Minute,
				WatchListPageSize:   100,
				WatchNeeds: WatchOptions{
					Resync: &WatchResyncOptions{Interval: -time.Hour, ListPageSize: -5},
				},
			},
			want: &WatchResyncOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.options.ResyncOptions())
		})
	}
}

func TestNewKubernetesBasedInformer_resyncResolution(t *testing.T) {
	hourly := InformerOptions{WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{Interval: time.Hour}}}

	t.Run("the cadence a kind asked for is what it advertises", func(t *testing.T) {
		inf, err := NewKubernetesBasedInformer(untypedKind, &mockListWatchClient{}, hourly)
		require.NoError(t, err)
		require.NotNil(t, inf.WatchOptions().Resync)
		assert.Equal(t, time.Hour, inf.WatchOptions().Resync.Interval)
	})

	t.Run("the event timeout clamps against the kind's cadence, not the app-wide one", func(t *testing.T) {
		options := hourly
		options.CacheResyncInterval = time.Minute
		options.EventTimeout = 5 * time.Minute
		inf, err := NewKubernetesBasedInformer(untypedKind, &mockListWatchClient{}, options)
		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute, inf.eventTimeout)
	})

	t.Run("a cadence shorter than the event timeout still clamps it", func(t *testing.T) {
		options := InformerOptions{
			WatchNeeds:   WatchOptions{Resync: &WatchResyncOptions{Interval: 30 * time.Second}},
			EventTimeout: 5 * time.Minute,
		}
		inf, err := NewKubernetesBasedInformer(untypedKind, &mockListWatchClient{}, options)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, inf.eventTimeout)
	})
}

func TestCustomCacheInformer_WatchOptions(t *testing.T) {
	build := func(opts CustomCacheInformerOptions) *CustomCacheInformer {
		return NewCustomCacheInformer(cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc), nil, untypedKind, opts)
	}

	t.Run("no cadence anywhere means no resync channel", func(t *testing.T) {
		assert.Nil(t, build(CustomCacheInformerOptions{}).WatchOptions().Resync)
	})

	t.Run("a memcached store which does not track keys has no resync to advertise", func(t *testing.T) {
		informer, err := NewMemcachedInformer(untypedKind, &mockListWatchClient{}, MemcachedInformerOptions{
			ServerAddrs: []string{"127.0.0.1:11211"},
			CustomCacheInformerOptions: CustomCacheInformerOptions{
				InformerOptions: InformerOptions{CacheResyncInterval: time.Minute},
			},
		})
		require.NoError(t, err)
		assert.Nil(t, informer.WatchOptions().Resync, "a resync enqueues the store's keys, and this store lists none")
	})

	t.Run("the app-wide interval is advertised", func(t *testing.T) {
		inf := build(CustomCacheInformerOptions{InformerOptions: InformerOptions{CacheResyncInterval: time.Minute}})
		require.NotNil(t, inf.WatchOptions().Resync)
		assert.Equal(t, time.Minute, inf.WatchOptions().Resync.Interval)
	})

	t.Run("the kind's cadence and page size win over the app-wide ones", func(t *testing.T) {
		inf := build(CustomCacheInformerOptions{InformerOptions: InformerOptions{
			CacheResyncInterval: time.Minute,
			WatchListPageSize:   100,
			WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{
				Interval:     time.Hour,
				ListPageSize: 5000,
			}},
		}})
		require.NotNil(t, inf.WatchOptions().Resync)
		assert.Equal(t, time.Hour, inf.WatchOptions().Resync.Interval)
		assert.Equal(t, int64(5000), inf.listPageSize)
	})
}

func TestKubernetesBasedInformer_WatchOptions(t *testing.T) {
	live := &WatchLiveOptions{Detail: WatchDetailFullObject, Distribution: WatchDistributionAll}

	t.Run("a watch with no resync has only the live channel", func(t *testing.T) {
		inf := &KubernetesBasedInformer{}
		assert.Equal(t, WatchOptions{Live: live}, inf.WatchOptions())
	})

	t.Run("a watch with a resync resyncs as well", func(t *testing.T) {
		inf := &KubernetesBasedInformer{resyncInterval: time.Hour}
		assert.Equal(t, WatchOptions{
			Live: live,
			Resync: &WatchResyncOptions{
				Detail:       WatchDetailFullObject,
				Distribution: WatchDistributionAll,
				Interval:     time.Hour,
			},
		}, inf.WatchOptions())
	})
}

type wrappingInformer struct {
	mockInformer
	wrapped Informer
}

func (w *wrappingInformer) Unwrap() Informer {
	return w.wrapped
}

func TestInformerWithWatchOptions(t *testing.T) {
	provides := WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}}

	t.Run("an informer which says what it provides is returned unchanged", func(t *testing.T) {
		stated := &optionedInformer{options: WatchOptions{Resync: &WatchResyncOptions{}}}
		assert.Same(t, stated, InformerWithWatchOptions(stated, provides))
	})

	t.Run("an informer which says nothing is given its mechanism's advertisement", func(t *testing.T) {
		options, stated := InformerWatchOptions(InformerWithWatchOptions(&mockInformer{}, provides))
		assert.True(t, stated)
		assert.Equal(t, provides, options)
	})

	t.Run("wrapping keeps the informer's metrics and health checks", func(t *testing.T) {
		inner := &instrumentedInformer{}
		wrapped := InformerWithWatchOptions(inner, provides)

		provider, ok := wrapped.(metrics.Provider)
		require.True(t, ok)
		assert.Equal(t, inner.PrometheusCollectors(), provider.PrometheusCollectors())

		checker, ok := wrapped.(health.Checker)
		require.True(t, ok)
		assert.Equal(t, []health.Check{inner}, checker.HealthChecks())

		// And adds no check of its own for an informer which has none.
		bare := InformerWithWatchOptions(&mockInformer{}, provides)
		_, isCheck := bare.(health.Check)
		assert.False(t, isCheck, "the wrapper must not register a health check the informer does not have")
		assert.Empty(t, bare.(health.Checker).HealthChecks())
	})

	t.Run("a nil informer stays nil", func(t *testing.T) {
		assert.Nil(t, InformerWithWatchOptions(nil, provides))
	})
}

// instrumentedInformer is an informer which reports metrics and its own health, as the SDK's do.
type instrumentedInformer struct {
	mockInformer
	collector prometheus.Collector
}

func (i *instrumentedInformer) PrometheusCollectors() []prometheus.Collector {
	if i.collector == nil {
		i.collector = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_total"})
	}
	return []prometheus.Collector{i.collector}
}

func (i *instrumentedInformer) HealthCheck(context.Context) error { return nil }

func (i *instrumentedInformer) HealthCheckName() string { return "instrumented" }

func TestNewSelectingInformerSupplier_nilFallback(t *testing.T) {
	supplier := NewSelectingInformerSupplier(nil, NewInformerMechanism(
		WatchOptions{Resync: &WatchResyncOptions{}},
		func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return &mockInformer{}, nil
		},
	))

	// A kind the mechanism serves is built; one it does not is an error rather than a nil func call.
	served, err := supplier(untypedKind, nil, InformerOptions{
		WatchNeeds: WatchOptions{Resync: &WatchResyncOptions{}},
	})
	require.NoError(t, err)
	assert.NotNil(t, served)

	_, err = supplier(untypedKind, nil, InformerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no fallback supplier")
}

func TestInformerWithWatchOptions_wrappingAStockInformer(t *testing.T) {
	// A mechanism which builds on top of a stock informer: the one underneath says it carries whole objects, the
	// mechanism says its own deliveries carry identities. The mechanism is what was selected on, so it wins.
	stock, err := NewKubernetesBasedInformer(untypedKind, &mockListWatchClient{}, InformerOptions{})
	require.NoError(t, err)
	require.Equal(t, WatchDetailFullObject, stock.WatchOptions().Live.Detail)

	provides := WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity}}
	built := InformerWithWatchOptions(&wrappingInformer{wrapped: stock}, provides)

	options, stated := InformerWatchOptions(built)
	require.True(t, stated)
	assert.Equal(t, WatchDetailIdentity, options.Live.Detail)
}

// comparingInformer is an informer that compares against previous deliveries.
type comparingInformer struct {
	mockInformer
}

func (*comparingInformer) ComparesPrevious() bool { return true }

// comparingMechanism is a mechanism which builds comparingInformers.
type comparingMechanism struct {
	InformerMechanism
}

func (comparingMechanism) ComparesPrevious() bool { return true }

func TestPreviousComparer(t *testing.T) {
	resync := WatchOptions{Resync: &WatchResyncOptions{}}
	needsPrevious := WatchOptions{Resync: &WatchResyncOptions{IgnoreUnchanged: SameGeneration}}
	plain := NewInformerMechanism(resync, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
		return &mockInformer{}, nil
	})
	comparing := comparingMechanism{NewInformerMechanism(resync, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
		return &comparingInformer{}, nil
	})}
	build := func(needs WatchOptions, supplier InformerSupplier) (Informer, error) {
		return BuildInformer(InformerRequest{Kind: untypedKind, Supplier: supplier, Options: InformerOptions{WatchNeeds: needs}})
	}

	t.Run("selection skips a mechanism which does not compare, for a kind which sets IgnoreUnchanged", func(t *testing.T) {
		inf, err := build(needsPrevious, NewSelectingInformerSupplier(nil, plain, comparing))
		require.NoError(t, err)
		assert.True(t, comparesPrevious(inf), "the comparing mechanism's informer, through the advertising wrapper")
	})

	t.Run("a kind which does not set it takes the first mechanism", func(t *testing.T) {
		inf, err := build(resync, NewSelectingInformerSupplier(nil, plain, comparing))
		require.NoError(t, err)
		assert.False(t, comparesPrevious(inf))
	})

	t.Run("an informer which does not compare serves the kind, re-delivering every object", func(t *testing.T) {
		inf, err := build(needsPrevious, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return &mockInformer{}, nil
		})
		require.NoError(t, err)
		assert.False(t, comparesPrevious(inf))
	})

	t.Run("with no comparing mechanism, a mechanism which does not compare is taken before the fallback", func(t *testing.T) {
		fellBack := false
		inf, err := build(needsPrevious, NewSelectingInformerSupplier(
			func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
				fellBack = true
				return &mockInformer{}, nil
			},
			plain,
		))
		require.NoError(t, err)
		assert.False(t, fellBack)
		assert.False(t, comparesPrevious(inf))
	})

	t.Run("with no mechanism at all, the fallback, as the API watch is, takes over", func(t *testing.T) {
		fellBack := false
		_, err := build(needsPrevious, NewSelectingInformerSupplier(
			func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
				fellBack = true
				return &mockInformer{}, nil
			},
			nil, // the NATS mechanism, switched off
		))
		require.NoError(t, err)
		assert.True(t, fellBack)
	})

	t.Run("a comparer dividing its resync with no affinity is not selected, and refused when built", func(t *testing.T) {
		shared := WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionShared}}
		sharedComparing := comparingMechanism{NewInformerMechanism(shared, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return &comparingInformer{}, nil
		})}
		needsShared := WatchOptions{Resync: &WatchResyncOptions{
			IgnoreUnchanged: SameGeneration, Distribution: WatchDistributionShared,
		}}

		inf, err := build(needsShared, NewSelectingInformerSupplier(nil, sharedComparing, plain))
		require.NoError(t, err)
		assert.False(t, comparesPrevious(inf), "the shared comparer would report objects on other instances as gone")

		_, err = build(needsShared, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return InformerWithWatchOptions(&comparingInformer{}, shared), nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "an object listed to another instance would be taken for one which is gone")
	})

	t.Run("so is a comparer whose resync distribution this build does not recognize", func(t *testing.T) {
		roundRobin := WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistribution("round-robin")}}
		custom := comparingMechanism{NewInformerMechanism(roundRobin, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return &comparingInformer{}, nil
		})}
		needsShared := WatchOptions{Resync: &WatchResyncOptions{
			IgnoreUnchanged: SameGeneration, Distribution: WatchDistributionShared,
		}}

		inf, err := build(needsShared, NewSelectingInformerSupplier(nil, custom, plain))
		require.NoError(t, err)
		assert.False(t, comparesPrevious(inf))

		_, err = build(needsShared, func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
			return InformerWithWatchOptions(&comparingInformer{}, roundRobin), nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"round-robin"`)
	})

	t.Run("a sharded comparer serves such a kind", func(t *testing.T) {
		sharded := WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionSharded}}
		_, err := build(WatchOptions{Resync: &WatchResyncOptions{IgnoreUnchanged: SameGeneration, Distribution: WatchDistributionShared}},
			func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
				return InformerWithWatchOptions(&comparingInformer{}, sharded), nil
			})
		require.NoError(t, err)
	})

	t.Run("a comparer serves a kind which does not set IgnoreUnchanged, shared or not", func(t *testing.T) {
		_, err := build(WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionShared}},
			func(resource.Kind, resource.ClientGenerator, InformerOptions) (Informer, error) {
				return InformerWithWatchOptions(&comparingInformer{},
					WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionShared}}), nil
			})
		require.NoError(t, err)
	})
}

func TestBuildInformer_keepsObjectsAnOpinionatedHandlerHasNotFinished(t *testing.T) {
	var received InformerOptions
	supplier := func(_ resource.Kind, _ resource.ClientGenerator, options InformerOptions) (Informer, error) {
		received = options
		return &mockInformer{}, nil
	}
	always := WatchOptions{Resync: &WatchResyncOptions{
		Ignore:          func(resource.Object) bool { return true },
		IgnoreUnchanged: func(resource.Object, resource.Object) bool { return true },
	}}
	object := func(finalizers ...string) resource.Object {
		obj := &resource.UntypedObject{}
		obj.SetFinalizers(finalizers)
		return obj
	}

	t.Run("without a Finalizer, an opinionated kind setting either check is refused", func(t *testing.T) {
		for _, needs := range []WatchOptions{
			{Resync: &WatchResyncOptions{Ignore: always.Resync.Ignore}},
			{Resync: &WatchResyncOptions{IgnoreUnchanged: always.Resync.IgnoreUnchanged}},
		} {
			_, err := BuildInformer(InformerRequest{
				Kind: untypedKind, Supplier: supplier, Options: InformerOptions{WatchNeeds: needs}, Opinionated: true,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "names no Finalizer")
		}
	})

	t.Run("with one, both checks keep an object without it", func(t *testing.T) {
		_, err := BuildInformer(InformerRequest{
			Kind: untypedKind, Supplier: supplier, Options: InformerOptions{WatchNeeds: always},
			Opinionated: true, Finalizer: "done",
		})
		require.NoError(t, err)
		resync := received.WatchNeeds.Resync
		assert.False(t, resync.Ignore(object("wip")), "an add the handler has not finished")
		assert.True(t, resync.Ignore(object("done")))
		assert.False(t, resync.IgnoreUnchanged(object(), object("wip")))
		assert.True(t, resync.IgnoreUnchanged(object("done"), object("done")))
		assert.True(t, always.Resync.Ignore(object()), "the declaration itself is left as it was")
	})

	t.Run("a plain handler's checks are passed on unchanged", func(t *testing.T) {
		_, err := BuildInformer(InformerRequest{Kind: untypedKind, Supplier: supplier, Options: InformerOptions{WatchNeeds: always}})
		require.NoError(t, err)
		assert.True(t, received.WatchNeeds.Resync.Ignore(object()))
	})
}
