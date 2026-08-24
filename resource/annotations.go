package resource

// This file mirrors the Grafana annotation keys defined in grafana/grafana so apps built on the SDK
// (including ones outside the grafana/grafana repo) can use constants instead of hard-coding strings.
//
// The keys are defined in grafana/grafana's pkg/apimachinery/utils/meta.go and manager.go. The test in
// annotations_test.go checks the values here against core, so we catch it if those keys ever change.
//
// Source (pinned): https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go

// CanonicalAnnotationPrefix is the prefix grafana/grafana uses for app platform metadata annotation
// keys. Every AnnoKey* constant in this package uses it.
//
// The older SDK prefix is AnnotationPrefix ("grafana.com/"). It's still around for backward compat (see
// the deprecated AnnotationCreatedBy/AnnotationUpdatedBy/AnnotationUpdateTimestamp constants in
// untypedobject.go). We write the new prefix and fall back to the old one on read.
const CanonicalAnnotationPrefix = "grafana.app/"

// Grafana platform annotation keys. These mirror grafana/grafana, with a test guarding the values.
// Use these constants instead of hard-coding the strings.
const (
	// AnnoKeyCreatedBy is the identity that created the resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L41
	AnnoKeyCreatedBy = CanonicalAnnotationPrefix + "createdBy"

	// AnnoKeyUpdatedBy is the identity that last updated the resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L43
	AnnoKeyUpdatedBy = CanonicalAnnotationPrefix + "updatedBy"

	// AnnoKeyUpdatedTimestamp is the RFC3339 timestamp of the last update to the resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L42
	AnnoKeyUpdatedTimestamp = CanonicalAnnotationPrefix + "updatedTimestamp"

	// AnnoKeyFolder is the UID of the folder the resource is placed in.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L44
	AnnoKeyFolder = CanonicalAnnotationPrefix + "folder"

	// AnnoKeyMessage is a human-readable message associated with the change to the resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L46
	AnnoKeyMessage = CanonicalAnnotationPrefix + "message"

	// AnnoKeyGrantPermissions allows users to explicitly grant themselves permissions when creating
	// resources in the "root" folder. This annotation is not saved and is invalid for update.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L31
	AnnoKeyGrantPermissions = CanonicalAnnotationPrefix + "grant-permissions"

	// AnnoKeyManagerKind identifies the kind of tool managing the resource (see ManagerKind).
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L57
	AnnoKeyManagerKind = CanonicalAnnotationPrefix + "managedBy"

	// AnnoKeyManagerIdentity identifies the specific instance of the manager that manages the resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L58
	AnnoKeyManagerIdentity = CanonicalAnnotationPrefix + "managerId"

	// AnnoKeyManagerAllowsEdits indicates whether the manager allows edits to the resource by others.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L59
	AnnoKeyManagerAllowsEdits = CanonicalAnnotationPrefix + "managerAllowsEdits"

	// AnnoKeyManagerSuspended indicates whether the manager is suspended (skips updates to the resource).
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L60
	AnnoKeyManagerSuspended = CanonicalAnnotationPrefix + "managerSuspended"

	// AnnoKeySourcePath is the path to the source of a provisioned resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L64
	AnnoKeySourcePath = CanonicalAnnotationPrefix + "sourcePath"

	// AnnoKeySourceChecksum is the checksum of the source of a provisioned resource (e.g. a git commit hash).
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L65
	AnnoKeySourceChecksum = CanonicalAnnotationPrefix + "sourceChecksum"

	// AnnoKeySourceTimestamp is the unix-millis timestamp of the source of a provisioned resource.
	// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L66
	AnnoKeySourceTimestamp = CanonicalAnnotationPrefix + "sourceTimestamp"
)

// AnnoGrantPermissionsDefault is the value that should be sent with AnnoKeyGrantPermissions.
// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L34
const AnnoGrantPermissionsDefault = "default"

// Legacy annotation keys, retained only as read-fallbacks inside the manager/source accessors to match
// core's GetManagerProperties()/GetSourceProperties() semantics. These are intentionally unexported;
// new code should write the AnnoKeySource*/AnnoKeyManager* keys above.
//
// Defined in grafana/grafana: https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/meta.go#L50-L53
const (
	oldAnnoKeyRepoName      = CanonicalAnnotationPrefix + "repoName"
	oldAnnoKeyRepoPath      = CanonicalAnnotationPrefix + "repoPath"
	oldAnnoKeyRepoHash      = CanonicalAnnotationPrefix + "repoHash"
	oldAnnoKeyRepoTimestamp = CanonicalAnnotationPrefix + "repoTimestamp"
)

// firstNonEmptyAnnotation returns the value of the first key in keys that is present and non-empty in
// annotations, or "" if none match. It is used to read a canonical key while falling back to its legacy
// equivalent(s).
func firstNonEmptyAnnotation(annotations map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := annotations[k]; ok && v != "" {
			return v
		}
	}
	return ""
}
