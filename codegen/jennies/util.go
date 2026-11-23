package jennies

import (
	"path/filepath"
	"regexp"
	"strings"

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
// Deprecated: Use GetGeneratedGoTypePath instead.
//
//nolint:revive,staticcheck
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

// GetGeneratedGoTypePath returns the correct codegen path based on the kind, version, and whether the
// generated code should be grouped by kind or by GroupVersion.
// When groupByKind is true, the path will be <kind>/<version>.
// When groupByKind is false, the path will be <shortGroup>/<version>.
//
//nolint:revive
func GetGeneratedGoTypePath(groupByKind bool, shortGroup string, version string, kindMachineName string) string {
	if groupByKind {
		return filepath.Join(ToPackageName(kindMachineName), ToPackageName(version))
	}
	return filepath.Join(ToPackageName(shortGroup), ToPackageName(version))
}

// kindResource returns the API resource name for the kind, which comes from the kind's definition.
// It falls back to the lowercased PluralName for a VersionedKind which was not built from a definition.
func kindResource(kind codegen.VersionedKind) string {
	if kind.Resource != "" {
		return kind.Resource
	}
	return strings.ToLower(kind.PluralName)
}

func versionedKindToKindProperties(kind codegen.VersionedKind, appManifest codegen.AppManifest) codegen.KindProperties {
	return codegen.KindProperties{
		Kind:                   kind.Kind,
		Group:                  appManifest.Properties().FullGroup,
		ManifestGroup:          appManifest.Properties().Group,
		MachineName:            kind.MachineName,
		PluralMachineName:      kind.PluralMachineName,
		PluralName:             kind.PluralName,
		Resource:               kind.Resource,
		Current:                appManifest.Properties().PreferredVersion,
		Scope:                  kind.Scope,
		UserReadable:           kind.UserReadable,
		FolderScoped:           kind.FolderScoped,
		Validation:             kind.Validation,
		Mutation:               kind.Mutation,
		Conversion:             kind.Conversion,
		ConversionWebhookProps: kind.ConversionWebhookProps,
		Codegen:                kind.Codegen,
	}
}
