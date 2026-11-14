package operator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWatchOptions_SatisfiedBy(t *testing.T) {
	var (
		apiWatch       = WatchOptions{Live: &WatchLiveOptions{}}
		resyncingWatch = WatchOptions{Live: &WatchLiveOptions{}, Sweep: &WatchSweepOptions{}}
		// Identities live, whole objects on the re-list, which is what a bus with a full-object lister does.
		identityBus = WatchOptions{
			Live:  &WatchLiveOptions{Detail: WatchDetailIdentity},
			Sweep: &WatchSweepOptions{},
		}
		// The same bus with a round-robin group, which shares the live channel only.
		sharedBus = WatchOptions{
			Live: &WatchLiveOptions{
				Detail:       WatchDetailIdentity,
				Distribution: WatchDistributionShared,
			},
			Sweep: &WatchSweepOptions{},
		}
		// A keys-only lister, so even its sweep carries identities.
		keysOnlyBus = WatchOptions{
			Live:  &WatchLiveOptions{Detail: WatchDetailIdentity},
			Sweep: &WatchSweepOptions{Detail: WatchDetailIdentity},
		}
		reList = WatchOptions{Sweep: &WatchSweepOptions{}}
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
			needs:    WatchOptions{Live: &WatchLiveOptions{}, Sweep: &WatchSweepOptions{}},
			provided: apiWatch,
			want:     false,
		},
		{
			name:     "needing both channels is satisfied by a resyncing watch",
			needs:    WatchOptions{Live: &WatchLiveOptions{}, Sweep: &WatchSweepOptions{}},
			provided: resyncingWatch,
			want:     true,
		},
		{
			name:     "needing only a sweep is satisfied by a re-list",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{}},
			provided: reList,
			want:     true,
		},
		{
			name:     "needing only a sweep is not satisfied by a watch which does not resync",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{}},
			provided: apiWatch,
			want:     false,
		},
		{
			name:     "needing only a sweep is satisfied by a bus whose re-list reads whole objects",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{}},
			provided: identityBus,
			want:     true,
		},
		{
			name:     "needing whole objects on a sweep is not satisfied by a keys-only lister",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{}},
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
			name:     "a shared live channel is not compared for a kind which only sweeps",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{}},
			provided: sharedBus,
			want:     true,
		},
		{
			name:     "a kind which needs its sweep divided is not served by a bus which lists everything",
			needs:    WatchOptions{Sweep: &WatchSweepOptions{Distribution: WatchDistributionShared}},
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
			needs:    WatchOptions{Sweep: &WatchSweepOptions{Every: time.Hour, PageSize: 500}},
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
	assert.Equal(t, "live=full-object/all sweep=none", WatchOptions{}.String())
	assert.Equal(t, "live=identity/shared sweep=full-object/all", WatchOptions{
		Live: &WatchLiveOptions{
			Detail:       WatchDetailIdentity,
			Distribution: WatchDistributionShared,
		},
		Sweep: &WatchSweepOptions{},
	}.String())
	assert.Equal(t, "live=none sweep=full-object/all", WatchOptions{
		Sweep: &WatchSweepOptions{Every: time.Hour, PageSize: 500},
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
		want := WatchOptions{Sweep: &WatchSweepOptions{}}
		options, stated := InformerWatchOptions(&wrappingInformer{wrapped: &optionedInformer{options: want}})
		assert.True(t, stated)
		assert.Equal(t, want, options)
	})

	t.Run("an informer wrapping one which says nothing says nothing", func(t *testing.T) {
		_, stated := InformerWatchOptions(&wrappingInformer{wrapped: &mockInformer{}})
		assert.False(t, stated)
	})

	t.Run("an informer which says so states its own options", func(t *testing.T) {
		want := WatchOptions{Sweep: &WatchSweepOptions{}}
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

func TestKubernetesBasedInformer_WatchOptions(t *testing.T) {
	live := &WatchLiveOptions{Detail: WatchDetailFullObject, Distribution: WatchDistributionAll}

	t.Run("a watch with no resync has only the live channel", func(t *testing.T) {
		inf := &KubernetesBasedInformer{}
		assert.Equal(t, WatchOptions{Live: live}, inf.WatchOptions())
	})

	t.Run("a watch with a resync sweeps as well", func(t *testing.T) {
		inf := &KubernetesBasedInformer{resyncInterval: time.Hour}
		assert.Equal(t, WatchOptions{
			Live: live,
			Sweep: &WatchSweepOptions{
				Detail:       WatchDetailFullObject,
				Distribution: WatchDistributionAll,
				Every:        time.Hour,
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
