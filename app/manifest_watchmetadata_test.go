package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// A platform registers from this map, so it is keyed by resource and unions a kind's versions: announcing a key no
// version reads costs payload size, while missing one a version does read is a correctness problem.
func TestManifestData_WatchMetadata(t *testing.T) {
	data := ManifestData{
		Group: "watchapp.ext.grafana.app",
		Versions: []ManifestVersion{
			{
				Name: "v1",
				Kinds: []ManifestVersionKind{
					{
						Kind: "Foo",
						WatchMetadata: &ManifestVersionKindWatchMetadata{
							Labels:     []string{"app"},
							Finalizers: []string{"watchapp.grafana.app/finalizer"},
						},
					},
					{Kind: "Undeclared"},
				},
			},
			{
				Name: "v2",
				Kinds: []ManifestVersionKind{
					{
						Kind:   "Foo",
						Plural: "Foos",
						WatchMetadata: &ManifestVersionKindWatchMetadata{
							// "app" repeats across versions and must not be duplicated.
							Labels:      []string{"app", "tier"},
							Annotations: []string{"watchapp.grafana.app/paused"},
							Finalizers:  []string{"watchapp.grafana.app/finalizer"},
						},
					},
				},
			},
		},
	}

	got := data.WatchMetadata()

	assert.Len(t, got, 1, "a kind which declares nothing is absent")
	assert.Equal(t, ManifestVersionKindWatchMetadata{
		Labels:      []string{"app", "tier"},
		Annotations: []string{"watchapp.grafana.app/paused"},
		Finalizers:  []string{"watchapp.grafana.app/finalizer"},
	}, got[schema.GroupResource{Group: "watchapp.ext.grafana.app", Resource: "foos"}])
}

// The resource name defaults to the lowercased kind plus "s", as the manifest documents, and an explicit plural is
// lowercased — a platform keys its registration on the resource, so this has to match what it serves.
func TestManifestData_WatchMetadataResourceNames(t *testing.T) {
	data := ManifestData{
		Group: "g",
		Versions: []ManifestVersion{{Name: "v1", Kinds: []ManifestVersionKind{
			{Kind: "Foo", WatchMetadata: &ManifestVersionKindWatchMetadata{Labels: []string{"a"}}},
			{Kind: "Entity", Plural: "Entities", WatchMetadata: &ManifestVersionKindWatchMetadata{Labels: []string{"b"}}},
		}}},
	}

	got := data.WatchMetadata()

	assert.Contains(t, got, schema.GroupResource{Group: "g", Resource: "foos"})
	assert.Contains(t, got, schema.GroupResource{Group: "g", Resource: "entities"})
}

func TestManifestData_WatchMetadataEmpty(t *testing.T) {
	assert.Empty(t, ManifestData{}.WatchMetadata())
}
