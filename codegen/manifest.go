package codegen

import "cuelang.org/go/cue"

type AppManifest interface {
	Name() string
	Kinds() []Kind
	Versions() []Version
	Properties() AppManifestProperties
}

type AppManifestProperties struct {
	AppName          string                                `json:"appName"`
	Group            string                                `json:"group"`
	FullGroup        string                                `json:"fullGroup"`
	ExtraPermissions AppManifestPropertiesExtraPermissions `json:"extraPermissions"`
	OperatorURL      *string                               `json:"operatorURL,omitempty"`
	PreferredVersion string                                `json:"preferredVersion"`
}

type AppManifestPropertiesExtraPermissions struct {
	AccessKinds []AppManifestKindPermission `json:"accessKinds,omitempty"`
}

type AppManifestKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

type SimpleManifest struct {
	Props       AppManifestProperties
	AllVersions []Version
}

func (m *SimpleManifest) Name() string {
	return m.Props.AppName
}

func (m *SimpleManifest) Properties() AppManifestProperties {
	return m.Props
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
						Group:                  m.Props.FullGroup,
						ManifestGroup:          m.Props.Group,
						MachineName:            kind.MachineName,
						PluralMachineName:      kind.PluralMachineName,
						PluralName:             kind.PluralName,
						Current:                m.Props.PreferredVersion,
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
	return m.AllVersions
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
	Props        VersionProperties
	AllKinds     []VersionedKind
	CustomRoutes VersionCustomRoutes
}

func (v *SimpleVersion) Name() string {
	return v.Props.Name
}

func (v *SimpleVersion) Properties() VersionProperties {
	return v.Props
}

func (v *SimpleVersion) Kinds() []VersionedKind {
	return v.AllKinds
}

func (v *SimpleVersion) Routes() VersionCustomRoutes {
	return v.CustomRoutes
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
	Routes map[string]map[string]CustomRoute `json:"routes"`
}
