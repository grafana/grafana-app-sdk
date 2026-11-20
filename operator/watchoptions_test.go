package operator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"
)

func TestWatchOptions_SatisfiedBy(t *testing.T) {
	var (
		apiWatch       = WatchOptions{Live: &WatchLiveOptions{}}
		resyncingWatch = WatchOptions{Live: &WatchLiveOptions{}, Resync: &WatchResyncOptions{}}
		// Identities live, whole objects on the re-list, which is what a bus with a full-object lister does.
		identityBus = WatchOptions{
			Live:   &WatchLiveOptions{Detail: WatchDetailIdentity},
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
			name:     "needing only a resync is satisfied by a bus whose re-list reads whole objects",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: identityBus,
			want:     true,
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
			name:     "a shared live channel is not compared for a kind which only resyncs",
			needs:    WatchOptions{Resync: &WatchResyncOptions{}},
			provided: sharedBus,
			want:     true,
		},
		{
			name:     "a kind which needs its resync divided is not served by a bus which lists everything",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Distribution: WatchDistributionShared}},
			provided: sharedBus,
			want:     false,
		},
		{
			name:     "an unrecognized detail is needed in full",
			needs:    WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetail("other")}},
			provided: identityBus,
			want:     false,
		},
		{
			name:     "a cadence and a page size are not compared",
			needs:    WatchOptions{Resync: &WatchResyncOptions{Interval: time.Hour, PageSize: 500}},
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
		Resync: &WatchResyncOptions{Interval: time.Hour, PageSize: 500},
	}.String())
}

func TestWatchDetail_SatisfiedBy(t *testing.T) {
	assert.True(t, WatchDetailIdentity.SatisfiedBy(WatchDetailFullObject))
	assert.True(t, WatchDetailIdentity.SatisfiedBy(WatchDetailIdentity))
	assert.True(t, WatchDetailFullObject.SatisfiedBy(WatchDetailFullObject))
	assert.False(t, WatchDetailFullObject.SatisfiedBy(WatchDetailIdentity))
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
		{needed: WatchDistributionShared, provided: WatchDistributionAll, want: false},
		{needed: WatchDistributionSharded, provided: WatchDistributionSharded, want: true},
		{needed: WatchDistributionSharded, provided: WatchDistributionShared, want: false},
		{needed: WatchDistributionSharded, provided: WatchDistributionAll, want: false},
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
			want:    &WatchResyncOptions{Interval: time.Minute, PageSize: 100},
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
					Detail:   WatchDetailIdentity,
					Interval: time.Hour,
					PageSize: 5000,
				}},
			},
			want: &WatchResyncOptions{
				Detail:   WatchDetailIdentity,
				Interval: time.Hour,
				PageSize: 5000,
			},
		},
		{
			name: "each field falls back on its own",
			options: InformerOptions{
				CacheResyncInterval: time.Minute,
				WatchListPageSize:   100,
				WatchNeeds:          WatchOptions{Resync: &WatchResyncOptions{PageSize: 5000}},
			},
			want: &WatchResyncOptions{Interval: time.Minute, PageSize: 5000},
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
				Interval: time.Hour,
				PageSize: 5000,
			}},
		}})
		require.NotNil(t, inf.WatchOptions().Resync)
		assert.Equal(t, time.Hour, inf.WatchOptions().Resync.Interval)
		assert.Equal(t, int64(5000), inf.resyncPageSize)
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
