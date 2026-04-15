package codegen

import (
	"iter"
	"slices"
	"strings"

	"cuelang.org/go/cue"
)

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Versions() []Version
	Properties() AppManifestProperties
}

type AppManifestProperties struct {
	AppName          string                                `json:"appName"`
	AppDisplayName   string                                `json:"appDisplayName"`
	Group            string                                `json:"group"`
	FullGroup        string                                `json:"fullGroup"`
	ExtraPermissions AppManifestPropertiesExtraPermissions `json:"extraPermissions"`
	OperatorURL      *string                               `json:"operatorURL,omitempty"`
	PreferredVersion string                                `json:"preferredVersion"`
	Roles            map[string]AppManifestPropertiesRole  `json:"roles"`
	RoleBindings     *AppManifestPropertiesRoleBindings    `json:"roleBindings"`
}

type AppManifestPropertiesExtraPermissions struct {
	AccessKinds []AppManifestKindPermission `json:"accessKinds,omitempty"`
}

type AppManifestKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

type AppManifestPropertiesRole struct {
	Title       string                          `json:"title"`
	Description string                          `json:"description"`
	Kinds       []AppManifestPropertiesRoleKind `json:"kinds"`
	Routes      []string                        `json:"routes"`
}

type AppManifestPropertiesRoleKind struct {
	Kind          string   `json:"kind"`
	Verbs         []string `json:"verbs"`
	PermissionSet *string  `json:"permissionSet"`
}

type AppManifestPropertiesRoleVersion struct {
	Kinds []string `json:"kinds" yaml:"kinds"`
}

type AppManifestPropertiesRoleBindings struct {
	// Viewer sets the role(s) granted to users in the "viewer" group
	Viewer []string `json:"viewer" yaml:"viewer"`
	// Editor sets the role(s) granted to users in the "editor" group
	Editor []string `json:"editor" yaml:"editor"`
	// Admin sets the role(s) granted to users in the "admin" group
	Admin []string `json:"admin" yaml:"admin"`
	// Additional is a map of additional group strings to their associated roles
	Additional map[string][]string `json:"additional" yaml:"additional"`
}

type SimpleManifest struct {
	AppManifestProperties
	AllVersions map[string]*SimpleVersion `json:"versions"`
}

func (m *SimpleManifest) Name() string {
	return m.AppName
}

func (m *SimpleManifest) Properties() AppManifestProperties {
	return m.AppManifestProperties
}

func (m *SimpleManifest) Kinds() []Kind {
	kinds := make(map[string]*AnyKind)
	for _, version := range m.AllVersions {
		for _, kind := range version.Kinds() {
			k, ok := kinds[kind.Kind]
			if !ok {
				k = &AnyKind{
					Props: KindProperties{
						Kind:                   kind.Kind,
						Group:                  m.FullGroup,
						ManifestGroup:          m.Group,
						MachineName:            kind.MachineName,
						PluralMachineName:      kind.PluralMachineName,
						PluralName:             kind.PluralName,
						Current:                m.PreferredVersion,
						Scope:                  kind.Scope,
						Validation:             kind.Validation,
						Mutation:               kind.Mutation,
						Conversion:             kind.Conversion,
						ConversionWebhookProps: kind.ConversionWebhookProps,
						Codegen:                kind.Codegen,
					},
					AllVersions: make([]KindVersion, 0),
				}
			}
			k.AllVersions = append(k.AllVersions, KindVersion{
				Version:                  version.Name(),
				Schema:                   kind.Schema,
				Codegen:                  kind.Codegen,
				Served:                   kind.Served,
				SelectableFields:         kind.SelectableFields,
				Validation:               kind.Validation,
				Mutation:                 kind.Mutation,
				AdditionalPrinterColumns: kind.AdditionalPrinterColumns,
				Routes:                   kind.Routes,
			})
			kinds[kind.Kind] = k
		}
	}
	ret := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		ret = append(ret, k)
	}
	return ret
}

func (m *SimpleManifest) Versions() []Version {
	versions := make([]Version, 0, len(m.AllVersions))
	for _, v := range m.AllVersions {
		versions = append(versions, v)
	}
	slices.SortFunc(versions, func(a, b Version) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return versions
}

type VersionProperties struct {
	Name    string                `json:"name"`
	Served  bool                  `json:"served"`
	Codegen KindCodegenProperties `json:"codegen"`
}

type VersionCustomRoutes struct {
	Namespaced map[string]map[string]CustomRoute `json:"namespaced"`
	Cluster    map[string]map[string]CustomRoute `json:"cluster"`
}

type Version interface {
	Name() string
	Properties() VersionProperties
	Kinds() []VersionedKind
	Routes() VersionCustomRoutes
}

type SimpleVersion struct {
	VersionProperties
	AllKinds     []VersionedKind      `json:"kinds"`
	CustomRoutes *VersionCustomRoutes `json:"routes,omitempty"`
}

func (v *SimpleVersion) Name() string {
	return v.VersionProperties.Name
}

func (v *SimpleVersion) Properties() VersionProperties {
	return v.VersionProperties
}

func (v *SimpleVersion) Kinds() []VersionedKind {
	return v.AllKinds
}

func (v *SimpleVersion) Routes() VersionCustomRoutes {
	if v.CustomRoutes == nil {
		return VersionCustomRoutes{}
	}
	return *v.CustomRoutes
}

type VersionedKind struct {
	// Kind is the unique-within-the-group name of the kind
	Kind string `json:"kind"`
	// MachineName is the machine version of the Kind, which follows the regex: /^[a-z]+[a-z0-9]*$/
	MachineName string `json:"machineName"`
	// PluralMachineName is the plural of the MachineName
	PluralMachineName string `json:"pluralMachineName"`
	// PluralName is the plural of the Kind
	PluralName             string                      `json:"pluralName"`
	Scope                  string                      `json:"scope"`
	Validation             KindAdmissionCapability     `json:"validation"`
	Mutation               KindAdmissionCapability     `json:"mutation"`
	Conversion             bool                        `json:"conversion"`
	ConversionWebhookProps ConversionWebhookProperties `json:"conversionWebhookProps"`
	// Codegen contains code-generation directives for the codegen pipeline
	Codegen                  KindCodegenProperties     `json:"codegen"`
	Served                   bool                      `json:"served"`
	SelectableFields         []string                  `json:"selectableFields"`
	AdditionalPrinterColumns []AdditionalPrinterColumn `json:"additionalPrinterColumns"`
	// Schema is the CUE schema for the version
	// This should eventually be changed to JSONSchema/OpenAPI(/AST?)
	Schema cue.Value                         `json:"schema"` // TODO: this should eventually be OpenAPI/JSONSchema (ast or bytes?)
	Routes map[string]map[string]CustomRoute `json:"routes,omitempty"`
}

// VersionedKinds returns a sequence of all VersionedKinds in version order.
// It can be used with the range operator to make the operation:
//
//	for _, version := range m.Versions() {
//	    for _, kind := range version.Kinds() {
//	        ...
//	    }
//	}
//
// simplified to:
//
//	for version, kind := range m.VersionedKinds() {
//	    ...
//	}
func VersionedKinds(manifest AppManifest) iter.Seq2[Version, VersionedKind] {
	return func(yield func(Version, VersionedKind) bool) {
		for _, version := range manifest.Versions() {
			for _, kind := range version.Kinds() {
				if !yield(version, kind) {
					return
				}
			}
		}
	}
}

// PreferredVersionKinds returns a sequence of all VersionedKinds for the preferred version.
// It can be used with the range operator to make the operation:
//
//	for version, kind := range m.PreferredVersionKinds() {
//	    ...
//	}
func PreferredVersionKinds(manifest AppManifest) iter.Seq2[Version, VersionedKind] {
	return func(yield func(Version, VersionedKind) bool) {
		for _, version := range manifest.Versions() {
			if version.Name() != manifest.Properties().PreferredVersion {
				continue
			}
			for _, kind := range version.Kinds() {
				if !yield(version, kind) {
					return
				}
			}
		}
	}
}
