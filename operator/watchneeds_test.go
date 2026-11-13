package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWatchNeeds_Resolve(t *testing.T) {
	bus := func(announced WatchMetadata) InformerOffer {
		return InformerOffer{Kind: InformerKindBus, Announces: &announced}
	}
	watch := InformerOffer{Kind: InformerKindWatch}
	periodic := InformerOffer{Kind: InformerKindPeriodic}

	tests := []struct {
		name           string
		needs          WatchNeeds
		available      []InformerOffer
		want           InformerKind
		wantPassedOver bool
	}{
		{
			name:      "unset needs resolve to a watch",
			available: []InformerOffer{watch, bus(WatchMetadata{}), periodic},
			want:      InformerKindWatch,
		},
		{
			name:      "objects resolve to a watch even where everything is available",
			needs:     WatchNeeds{Stream: WatchStreamObjects},
			available: []InformerOffer{watch, bus(WatchMetadata{}), periodic},
			want:      InformerKindWatch,
		},
		{
			name:      "signal prefers the bus",
			needs:     WatchNeeds{Stream: WatchStreamSignal},
			available: []InformerOffer{watch, bus(WatchMetadata{}), periodic},
			want:      InformerKindBus,
		},
		{
			name:      "signal falls back to a watch where no bus is available",
			needs:     WatchNeeds{Stream: WatchStreamSignal},
			available: []InformerOffer{watch, periodic},
			want:      InformerKindWatch,
		},
		{
			name:      "cadence prefers the periodic informer",
			needs:     WatchNeeds{Stream: WatchStreamCadence},
			available: []InformerOffer{watch, bus(WatchMetadata{}), periodic},
			want:      InformerKindPeriodic,
		},
		{
			name:      "cadence falls back to the bus before the watch",
			needs:     WatchNeeds{Stream: WatchStreamCadence},
			available: []InformerOffer{watch, bus(WatchMetadata{})},
			want:      InformerKindBus,
		},
		{
			name:  "nothing available resolves to a watch",
			needs: WatchNeeds{Stream: WatchStreamSignal},
			want:  InformerKindWatch,
		},
		{
			name: "a bus which announces what the handlers read serves them",
			needs: WatchNeeds{
				Stream:   WatchStreamSignal,
				Metadata: WatchMetadata{Labels: []string{"app"}, Generation: true},
			},
			available: []InformerOffer{watch, bus(WatchMetadata{Labels: []string{"app", "tier"}, Generation: true})},
			want:      InformerKindBus,
		},
		{
			name: "a bus which announces less than the handlers read is passed over",
			needs: WatchNeeds{
				Stream:   WatchStreamSignal,
				Metadata: WatchMetadata{Finalizers: []string{"myapp/finalizer"}, DeletionTimestamp: true},
			},
			available:      []InformerOffer{watch, bus(WatchMetadata{Generation: true})},
			want:           InformerKindWatch,
			wantPassedOver: true,
		},
		{
			name: "a bus announcing nothing still serves a need which reads nothing",
			needs: WatchNeeds{
				Stream: WatchStreamSignal,
			},
			available: []InformerOffer{watch, bus(WatchMetadata{})},
			want:      InformerKindBus,
		},
		{
			name: "an informer which carries whole objects satisfies any metadata need",
			needs: WatchNeeds{
				Stream:   WatchStreamCadence,
				Metadata: WatchMetadata{Labels: []string{"app"}, DeletionTimestamp: true},
			},
			available: []InformerOffer{watch, periodic},
			want:      InformerKindPeriodic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, passedOver := tt.needs.Resolve(tt.available...)
			assert.Equal(t, tt.want, got)
			if tt.wantPassedOver {
				assert.NotEmpty(t, passedOver)
			} else {
				assert.Empty(t, passedOver)
			}
		})
	}
}

func TestWatchMetadata_Missing(t *testing.T) {
	needs := WatchMetadata{
		Labels:            []string{"app", "tier"},
		Annotations:       []string{"myapp/paused"},
		Finalizers:        []string{"myapp/finalizer"},
		Generation:        true,
		CreationTimestamp: true,
		DeletionTimestamp: true,
	}
	announced := WatchMetadata{
		Labels:     []string{"app"},
		Finalizers: []string{"myapp/finalizer"},
		Generation: true,
	}

	missing := needs.Missing(announced)

	assert.Equal(t, WatchMetadata{
		Labels:            []string{"tier"},
		Annotations:       []string{"myapp/paused"},
		CreationTimestamp: true,
		DeletionTimestamp: true,
	}, missing)
	assert.False(t, needs.SatisfiedBy(announced))
	assert.True(t, needs.SatisfiedBy(needs))
	assert.Equal(t, "labels.tier, annotations.myapp/paused, creationTimestamp, deletionTimestamp", missing.String())
}

func TestWatchMetadata_IsZero(t *testing.T) {
	assert.True(t, WatchMetadata{}.IsZero())
	assert.False(t, WatchMetadata{Labels: []string{"app"}}.IsZero())
	assert.False(t, WatchMetadata{Generation: true}.IsZero())
}
