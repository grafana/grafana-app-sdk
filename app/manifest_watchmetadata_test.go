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
				{Kind: "Foo", Plural: "Foos", Resource: "foos", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{
					Labels:     []string{"app"},
					Finalizers: []string{"watchapp.grafana.app/finalizer"},
					Generation: true,
				})},
				{Kind: "Undeclared", Plural: "Undeclareds", Resource: "undeclareds"},
			}},
			{Name: "v2", Kinds: []ManifestVersionKind{
				{Kind: "Foo", Plural: "Foos", Resource: "foos", Watch: watchMetadata(&ManifestVersionKindWatchMetadata{
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

// A platform keys its registration on the resource the API serves the kind as, which the kind declares rather than
// the manifest deriving it from the kind name.
func TestManifestData_WatchMetadataResourceNames(t *testing.T) {
	data := ManifestData{
		Group: "g",
		Versions: []ManifestVersion{{Name: "v1", Kinds: []ManifestVersionKind{
			{Kind: "Foo", Plural: "Foos", Resource: "foos",
				Watch: watchMetadata(&ManifestVersionKindWatchMetadata{Labels: []string{"a"}})},
			{Kind: "Entity", Plural: "Entities", Resource: "entities",
				Watch: watchMetadata(&ManifestVersionKindWatchMetadata{Labels: []string{"b"}})},
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
		{Kind: "Foo", Watch: &ManifestVersionKindWatch{}},
	}}}}.WatchMetadata())
}
