package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestAnnotationKeyValues checks that every exported annotation key matches the value grafana/grafana
// uses (pkg/apimachinery/utils/meta.go), so the constants don't silently drift from core.
func TestAnnotationKeyValues(t *testing.T) {
	assert.Equal(t, "grafana.app/", CanonicalAnnotationPrefix)

	cases := map[string]string{
		AnnoKeyCreatedBy:          "grafana.app/createdBy",
		AnnoKeyUpdatedBy:          "grafana.app/updatedBy",
		AnnoKeyUpdatedTimestamp:   "grafana.app/updatedTimestamp",
		AnnoKeyFolder:             "grafana.app/folder",
		AnnoKeyMessage:            "grafana.app/message",
		AnnoKeyGrantPermissions:   "grafana.app/grant-permissions",
		AnnoKeyManagerKind:        "grafana.app/managedBy",
		AnnoKeyManagerIdentity:    "grafana.app/managerId",
		AnnoKeyManagerAllowsEdits: "grafana.app/managerAllowsEdits",
		AnnoKeyManagerSuspended:   "grafana.app/managerSuspended",
		AnnoKeySourcePath:         "grafana.app/sourcePath",
		AnnoKeySourceChecksum:     "grafana.app/sourceChecksum",
		AnnoKeySourceTimestamp:    "grafana.app/sourceTimestamp",
	}
	for got, want := range cases {
		assert.Equal(t, want, got)
	}

	assert.Equal(t, "default", AnnoGrantPermissionsDefault)

	// Legacy keys remain on the deprecated "grafana.com/" prefix for backward compatibility.
	assert.Equal(t, "grafana.com/createdBy", AnnotationCreatedBy)
	assert.Equal(t, "grafana.com/updatedBy", AnnotationUpdatedBy)
	assert.Equal(t, "grafana.com/updateTimestamp", AnnotationUpdateTimestamp)
}

func TestFolderAccessors(t *testing.T) {
	obj := &metav1.ObjectMeta{}

	assert.Equal(t, "", GetFolder(obj))

	SetFolder(obj, "team-a-dashboards")
	assert.Equal(t, "team-a-dashboards", obj.Annotations[AnnoKeyFolder])
	assert.Equal(t, "team-a-dashboards", GetFolder(obj))

	// Setting an empty folder removes the annotation (and drops the empty map).
	SetFolder(obj, "")
	assert.Equal(t, "", GetFolder(obj))
	assert.Nil(t, obj.Annotations)
}

func TestManagerProperties_RoundTrip(t *testing.T) {
	obj := &metav1.ObjectMeta{}

	// Missing annotations -> not found.
	_, ok := GetManagerProperties(obj)
	assert.False(t, ok)

	in := ManagerProperties{
		Kind:        ManagerKindRepo,
		Identity:    "my-repo",
		AllowsEdits: true,
		Suspended:   false,
	}
	SetManagerProperties(obj, in)

	out, ok := GetManagerProperties(obj)
	require.True(t, ok)
	assert.Equal(t, in, out)

	// Suspended=false and AllowsEdits unset should not leave stale annotations.
	assert.Equal(t, "repo", obj.Annotations[AnnoKeyManagerKind])
	assert.Equal(t, "my-repo", obj.Annotations[AnnoKeyManagerIdentity])
	assert.Equal(t, "true", obj.Annotations[AnnoKeyManagerAllowsEdits])
	_, hasSuspended := obj.Annotations[AnnoKeyManagerSuspended]
	assert.False(t, hasSuspended)
}

func TestManagerProperties_LegacyRepoFallback(t *testing.T) {
	obj := &metav1.ObjectMeta{
		Annotations: map[string]string{
			oldAnnoKeyRepoName: "legacy-repo",
		},
	}

	out, ok := GetManagerProperties(obj)
	require.True(t, ok)
	assert.Equal(t, ManagerKindRepo, out.Kind)
	assert.Equal(t, "legacy-repo", out.Identity)
}

func TestSourceProperties_RoundTrip(t *testing.T) {
	obj := &metav1.ObjectMeta{}

	_, ok := GetSourceProperties(obj)
	assert.False(t, ok)

	in := SourceProperties{
		Path:            "dashboards/foo.json",
		Checksum:        "abc123",
		TimestampMillis: 1718000000000,
	}
	SetSourceProperties(obj, in)

	out, ok := GetSourceProperties(obj)
	require.True(t, ok)
	assert.Equal(t, in, out)
}

func TestSourceProperties_LegacyFallback(t *testing.T) {
	obj := &metav1.ObjectMeta{
		Annotations: map[string]string{
			oldAnnoKeyRepoPath:      "legacy/path.json",
			oldAnnoKeyRepoHash:      "deadbeef",
			oldAnnoKeyRepoTimestamp: "1700000000000",
		},
	}

	out, ok := GetSourceProperties(obj)
	require.True(t, ok)
	assert.Equal(t, "legacy/path.json", out.Path)
	assert.Equal(t, "deadbeef", out.Checksum)
	assert.Equal(t, int64(1700000000000), out.TimestampMillis)
}

func TestParseManagerKindString(t *testing.T) {
	cases := map[string]ManagerKind{
		"repo":                      ManagerKindRepo,
		"terraform":                 ManagerKindTerraform,
		"kubectl":                   ManagerKindKubectl,
		"plugin":                    ManagerKindPlugin,
		"grafana":                   ManagerKindGrafana,
		"classic-file-provisioning": ManagerKindClassicFP,
		"":                          ManagerKindUnknown,
		"something-else":            ManagerKindUnknown,
	}
	for in, want := range cases {
		assert.Equal(t, want, ParseManagerKindString(in), "input %q", in)
	}
}

// TestCommonMetadataLegacyAndCanonicalRead checks that GetCommonMetadata reads both the old
// "grafana.com/" keys (written by older SDK versions) and the "grafana.app/" keys (written by core),
// and that it prefers the "grafana.app/" key when both are set.
func TestCommonMetadataLegacyAndCanonicalRead(t *testing.T) {
	t.Run("legacy keys still read", func(t *testing.T) {
		obj := &UntypedObject{}
		obj.Annotations = map[string]string{
			AnnotationCreatedBy: "legacy-creator",
			AnnotationUpdatedBy: "legacy-updater",
		}
		cm := obj.GetCommonMetadata()
		assert.Equal(t, "legacy-creator", cm.CreatedBy)
		assert.Equal(t, "legacy-updater", cm.UpdatedBy)
	})

	t.Run("canonical keys read", func(t *testing.T) {
		obj := &UntypedObject{}
		obj.Annotations = map[string]string{
			AnnoKeyCreatedBy: "core-creator",
			AnnoKeyUpdatedBy: "core-updater",
		}
		cm := obj.GetCommonMetadata()
		assert.Equal(t, "core-creator", cm.CreatedBy)
		assert.Equal(t, "core-updater", cm.UpdatedBy)
	})

	t.Run("canonical preferred over legacy", func(t *testing.T) {
		obj := &UntypedObject{}
		obj.Annotations = map[string]string{
			AnnoKeyCreatedBy:    "core-creator",
			AnnotationCreatedBy: "legacy-creator",
		}
		cm := obj.GetCommonMetadata()
		assert.Equal(t, "core-creator", cm.CreatedBy)
	})
}
