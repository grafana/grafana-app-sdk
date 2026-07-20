package resource

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ManagerProperties is used to identify the manager (the tool responsible for managing a resource).
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/manager.go#L3-L20
type ManagerProperties struct {
	// Kind is the kind of manager responsible for managing the resource.
	// Examples include "repo", "terraform", "kubectl", etc.
	Kind ManagerKind `json:"kind,omitempty"`

	// Identity refers to a specific instance of the manager.
	// The format & the value depend on the manager kind.
	Identity string `json:"id,omitempty"`

	// AllowsEdits indicates whether the manager allows edits to the resource.
	// If set to true, it means that other requesters can edit the resource.
	AllowsEdits bool `json:"allowEdits,omitempty"`

	// Suspended indicates whether the manager is suspended.
	// If set to true, then the manager skips updates to the resource.
	Suspended bool `json:"suspended,omitempty"`
}

// ManagerKind is the type of manager responsible for managing a resource.
// It can be a user, a tool, or a generic API client.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/manager.go#L25
type ManagerKind string

// Known values for ManagerKind.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/manager.go#L28-L39
const (
	ManagerKindUnknown   ManagerKind = ""
	ManagerKindRepo      ManagerKind = "repo"
	ManagerKindTerraform ManagerKind = "terraform"
	ManagerKindKubectl   ManagerKind = "kubectl"
	ManagerKindPlugin    ManagerKind = "plugin"
	ManagerKindGrafana   ManagerKind = "grafana"

	// ManagerKindClassicFP is a shim/migration path for legacy file provisioning.
	// Previously this was a "file:" prefix.
	//
	// Deprecated: this is used as a shim/migration path for legacy file provisioning.
	ManagerKindClassicFP ManagerKind = "classic-file-provisioning"
)

// ParseManagerKindString parses a string into a ManagerKind.
// For unknown values, it returns ManagerKindUnknown.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/manager.go#L44
func ParseManagerKindString(v string) ManagerKind {
	switch v {
	case string(ManagerKindRepo):
		return ManagerKindRepo
	case string(ManagerKindTerraform):
		return ManagerKindTerraform
	case string(ManagerKindKubectl):
		return ManagerKindKubectl
	case string(ManagerKindPlugin):
		return ManagerKindPlugin
	case string(ManagerKindGrafana):
		return ManagerKindGrafana
	case string(ManagerKindClassicFP): // nolint:staticcheck
		return ManagerKindClassicFP // nolint:staticcheck
	default:
		return ManagerKindUnknown
	}
}

// SourceProperties is used to identify the source of a provisioned resource.
// It is used by managers for reconciling data from a source to Grafana.
// Not all managers use these properties; some (like Terraform) don't have a concept of a source.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/68d9a5eadb9a6fa1a2512b0e137257edfb8fedce/pkg/apimachinery/utils/manager.go#L66-L78
type SourceProperties struct {
	// Path to the source of the resource. Can be a file path, a URL, etc.
	Path string `json:"path,omitempty"`

	// Checksum of the source of the resource. An example could be a git commit hash.
	Checksum string `json:"checksum,omitempty"`

	// TimestampMillis is the unix-millis timestamp of the source of the resource.
	// An example could be the file modification time.
	TimestampMillis int64 `json:"timestampMillis,omitempty"`
}

// GetFolder returns the UID of the folder the resource is placed in (AnnoKeyFolder), or "" if unset.
func GetFolder(obj metav1.Object) string {
	return getAnnotation(obj, AnnoKeyFolder)
}

// SetFolder sets the UID of the folder the resource is placed in (AnnoKeyFolder).
// Passing an empty uid removes the annotation.
func SetFolder(obj metav1.Object, uid string) {
	setAnnotation(obj, AnnoKeyFolder, uid)
}

// GetManagerProperties returns the identity of the tool responsible for managing the resource.
// If the identity is not known, the second return value is false.
//
// To match core semantics, the legacy "grafana.app/repoName" annotation is read as a fallback.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/meta.go#L661
func GetManagerProperties(obj metav1.Object) (ManagerProperties, bool) {
	res := ManagerProperties{
		Identity:    "",
		Kind:        ManagerKindUnknown,
		AllowsEdits: false,
		Suspended:   false,
	}

	annot := obj.GetAnnotations()

	id, ok := annot[AnnoKeyManagerIdentity]
	if !ok || id == "" {
		// Temporarily support the legacy repo name annotation.
		repo := annot[oldAnnoKeyRepoName]
		if repo != "" {
			return ManagerProperties{
				Kind:     ManagerKindRepo,
				Identity: repo,
			}, true
		}

		// If the identity is not set, ignore the other annotations and return the default values.
		// This prevents inadvertently marking resources as managed, which could block updates from
		// other sources.
		return res, false
	}
	res.Identity = id

	if v, ok := annot[AnnoKeyManagerKind]; ok {
		res.Kind = ParseManagerKindString(v)
	}

	if v, ok := annot[AnnoKeyManagerAllowsEdits]; ok {
		res.AllowsEdits = v == "true"
	}

	if v, ok := annot[AnnoKeyManagerSuspended]; ok {
		res.Suspended = v == "true"
	}

	return res, true
}

// SetManagerProperties sets the identity of the tool responsible for managing the resource.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/meta.go#L705
func SetManagerProperties(obj metav1.Object, v ManagerProperties) {
	annot := obj.GetAnnotations()
	if annot == nil {
		annot = make(map[string]string, 4)
	}

	if v.Identity != "" {
		annot[AnnoKeyManagerIdentity] = v.Identity
	} else {
		delete(annot, AnnoKeyManagerIdentity)
	}

	if string(v.Kind) != "" {
		annot[AnnoKeyManagerKind] = string(v.Kind)
	} else {
		delete(annot, AnnoKeyManagerKind)
	}

	if v.AllowsEdits {
		annot[AnnoKeyManagerAllowsEdits] = strconv.FormatBool(v.AllowsEdits)
	} else {
		delete(annot, AnnoKeyManagerAllowsEdits)
	}

	if v.Suspended {
		annot[AnnoKeyManagerSuspended] = strconv.FormatBool(v.Suspended)
	} else {
		delete(annot, AnnoKeyManagerSuspended)
	}

	// Clean up legacy annotation.
	delete(annot, oldAnnoKeyRepoName)

	obj.SetAnnotations(annot)
}

// GetSourceProperties returns the source properties of a provisioned resource.
// If no source properties are set, the second return value is false.
//
// To match core semantics, the legacy "grafana.app/repoPath", "grafana.app/repoHash", and
// "grafana.app/repoTimestamp" annotations are read as fallbacks.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/meta.go#L740
func GetSourceProperties(obj metav1.Object) (SourceProperties, bool) {
	var (
		res   SourceProperties
		found bool
	)

	annot := obj.GetAnnotations()
	if annot == nil {
		return res, false
	}

	if path, ok := annot[AnnoKeySourcePath]; ok && path != "" {
		res.Path = path
		found = true
	} else if path, ok := annot[oldAnnoKeyRepoPath]; ok && path != "" {
		res.Path = path
		found = true
	}

	if hash, ok := annot[AnnoKeySourceChecksum]; ok && hash != "" {
		res.Checksum = hash
		found = true
	} else if hash, ok := annot[oldAnnoKeyRepoHash]; ok && hash != "" {
		res.Checksum = hash
		found = true
	}

	t, ok := annot[AnnoKeySourceTimestamp]
	if !ok {
		t, ok = annot[oldAnnoKeyRepoTimestamp]
	}
	if ok && t != "" {
		if ts, err := strconv.ParseInt(t, 10, 64); err == nil {
			res.TimestampMillis = ts
			found = true
		}
	}

	return res, found
}

// SetSourceProperties sets the source properties of a provisioned resource.
//
// Mirrors grafana/grafana:
// https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/meta.go#L782
func SetSourceProperties(obj metav1.Object, v SourceProperties) {
	annot := obj.GetAnnotations()
	if annot == nil {
		annot = make(map[string]string, 3)
	}

	if v.Path != "" {
		annot[AnnoKeySourcePath] = v.Path
	} else {
		delete(annot, AnnoKeySourcePath)
	}

	if v.Checksum != "" {
		annot[AnnoKeySourceChecksum] = v.Checksum
	} else {
		delete(annot, AnnoKeySourceChecksum)
	}

	if v.TimestampMillis > 0 {
		annot[AnnoKeySourceTimestamp] = strconv.FormatInt(v.TimestampMillis, 10)
	} else {
		delete(annot, AnnoKeySourceTimestamp)
	}

	obj.SetAnnotations(annot)
}

// getAnnotation reads a single annotation key off obj, returning "" if unset.
func getAnnotation(obj metav1.Object, key string) string {
	annot := obj.GetAnnotations()
	if annot == nil {
		return ""
	}
	return annot[key]
}

// setAnnotation writes a single annotation key on obj. An empty value removes the key, and an empty
// annotation map is set back to nil so the field is dropped from the serialized object.
func setAnnotation(obj metav1.Object, key, val string) {
	annot := obj.GetAnnotations()
	if val == "" {
		if annot != nil {
			delete(annot, key)
			if len(annot) == 0 {
				annot = nil
			}
		}
	} else {
		if annot == nil {
			annot = make(map[string]string)
		}
		annot[key] = val
	}
	obj.SetAnnotations(annot)
}
