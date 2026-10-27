package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func watchMetadata(md *ManifestVersionKindWatchMetadata) *ManifestVersionKindWatch {
	return &ManifestVersionKindWatch{Metadata: md}
}

// A platform registers from this map, so it is keyed by resource and unions a kind's versions: announcing a key no
// version reads costs payload size, while missing one a version does read is a correctness problem.
func TestManifestData_WatchMetadata(t *testing.T) {
	data := ManifestData{
		Group: "watchapp.ext.grafana.app",
		Versions: []ManifestVersion{
			{Name: "v1", Kinds: []ManifestVersionKind{
				{Kind: "Foo", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{
					Labels:     []string{"app"},
					Finalizers: []string{"watchapp.grafana.app/finalizer"},
					Generation: true,
				})},
				{Kind: "Undeclared"},
			}},
			{Name: "v2", Kinds: []ManifestVersionKind{
				{Kind: "Foo", Plural: "Foos", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{
					// "app" repeats across versions and must not be duplicated.
					Labels:            []string{"app", "tier"},
					Annotations:       []string{"watchapp.grafana.app/paused"},
					Finalizers:        []string{"watchapp.grafana.app/finalizer"},
					DeletionTimestamp: true,
				})},
			}},
		},
	}

	got := data.WatchMetadata()

	assert.Len(t, got, 1, "a kind which declares nothing is absent")
	assert.Equal(t, ManifestVersionKindWatchMetadata{
		Labels:            []string{"app", "tier"},
		Annotations:       []string{"watchapp.grafana.app/paused"},
		Finalizers:        []string{"watchapp.grafana.app/finalizer"},
		Generation:        true,
		DeletionTimestamp: true,
	}, got[schema.GroupResource{Group: "watchapp.ext.grafana.app", Resource: "foos"}])
}

// The resource name defaults to the lowercased kind plus "s", and an explicit plural is lowercased — a platform keys
// its registration on the resource, so this has to match what it serves.
func TestManifestData_WatchMetadataResourceNames(t *testing.T) {
	data := ManifestData{
		Group: "g",
		Versions: []ManifestVersion{{Name: "v1", Kinds: []ManifestVersionKind{
			{Kind: "Foo", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{Labels: []string{"a"}})},
			{Kind: "Entity", Plural: "Entities", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{Labels: []string{"b"}})},
		}}},
	}

	got := data.WatchMetadata()

	assert.Contains(t, got, schema.GroupResource{Group: "g", Resource: "foos"})
	assert.Contains(t, got, schema.GroupResource{Group: "g", Resource: "entities"})
}

func TestManifestData_WatchMetadataEmpty(t *testing.T) {
	assert.Empty(t, ManifestData{}.WatchMetadata())
	// A watch section with no metadata declares nothing to announce.
	assert.Empty(t, ManifestData{Versions: []ManifestVersion{{Kinds: []ManifestVersionKind{
		{Kind: "Foo", Watch: &ManifestVersionKindWatch{Needs: WatchNeedsSignal}},
	}}}}.WatchMetadata())
}

// One informer serves every version of a kind, so the strongest requirement across them wins.
func TestManifestData_WatchNeeds(t *testing.T) {
	data := ManifestData{
		Group: "g",
		Versions: []ManifestVersion{
			{Name: "v1", Kinds: []ManifestVersionKind{
				{Kind: "Cadenced", Watch: &ManifestVersionKindWatch{Needs: WatchNeedsCadence}},
				{Kind: "Signalled", Watch: &ManifestVersionKindWatch{Needs: WatchNeedsSignal}},
				{Kind: "Undeclared"},
			}},
			{Name: "v2", Kinds: []ManifestVersionKind{
				// v2's handlers read objects, so the kind's informer must provide them.
				{Kind: "Signalled", Watch: &ManifestVersionKindWatch{Needs: WatchNeedsObjects}},
				{Kind: "Cadenced", Watch: &ManifestVersionKindWatch{Needs: WatchNeedsCadence}},
			}},
		},
	}

	got := data.WatchNeeds()

	assert.Equal(t, WatchNeedsObjects, got["Signalled"], "the strongest requirement across versions wins")
	assert.Equal(t, WatchNeedsCadence, got["Cadenced"])
	assert.NotContains(t, got, "Undeclared", "a kind which declares nothing is absent, and read as needing objects")
}

// The resolution rule is shared by the app supplier and by wiring which builds informers directly, so it is tested
// where it lives. Every fallback is upward in capability.
func TestWatchNeedsResolve(t *testing.T) {
	all := []WatchInformer{WatchInformerWatch, WatchInformerBus, WatchInformerPeriodic}
	noBus := []WatchInformer{WatchInformerWatch, WatchInformerPeriodic}

	for _, tt := range []struct {
		needs       ManifestVersionKindWatchNeeds
		withAll     WatchInformer
		withoutABus WatchInformer
	}{
		{WatchNeedsObjects, WatchInformerWatch, WatchInformerWatch},
		{"", WatchInformerWatch, WatchInformerWatch},
		{WatchNeedsSignal, WatchInformerBus, WatchInformerWatch},
		{WatchNeedsCadence, WatchInformerPeriodic, WatchInformerPeriodic},
	} {
		t.Run(string(tt.needs), func(t *testing.T) {
			assert.Equal(t, tt.withAll, tt.needs.Resolve(all...))
			assert.Equal(t, tt.withoutABus, tt.needs.Resolve(noBus...), "a missing bus never fails, it falls back")
			assert.Equal(t, WatchInformerWatch, tt.needs.Resolve(),
				"nothing declared available falls back to the watch, which satisfies every need")
		})
	}

	// A cadence need is satisfied by anything which delivers changes, so it falls back rather than failing.
	assert.Equal(t, WatchInformerBus, WatchNeedsCadence.Resolve(WatchInformerBus))
}
