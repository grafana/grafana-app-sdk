package jennies

import (
	"path/filepath"
	"regexp"

	"github.com/grafana/grafana-app-sdk/codegen"
)

// ToPackageName sanitizes an input into a deterministic allowed go package name.
// It is used to turn kind names or versions into package names when performing go code generation.
func ToPackageName(input string) string {
	return regexp.MustCompile(`([^A-Za-z0-9_])`).ReplaceAllString(input, "_")
}

// GetGeneratedPath returns the correct codegen path based on the kind, version, and whether or not the
// generated code should be grouped by kind or by GroupVersion.
// When groupByKind is true, the path will be <kind>/<version>.
// When groupByKind is false, the path will be <group>/<version>.
//
//nolint:revive
func GetGeneratedPath(groupByKind bool, kind codegen.Kind, version string) string {
	if groupByKind {
		return filepath.Join(ToPackageName(kind.Properties().MachineName), ToPackageName(version))
	}
	grp := kind.Properties().ManifestGroup
	if grp == "" {
		grp = kind.Properties().Group
	}
	return filepath.Join(ToPackageName(grp), ToPackageName(version))
}

// getGeneratedPathForKind uses the same logic as GetGeneratedPath, but uses a codegen.VersionedKind instead of codegen.Kind
//
//nolint:revive
func getGeneratedPathForKind(groupByKind bool, group string, kind codegen.VersionedKind, version string) string {
	if groupByKind {
		return filepath.Join(ToPackageName(kind.MachineName), ToPackageName(version))
	}
	return filepath.Join(ToPackageName(group), ToPackageName(version))
}
