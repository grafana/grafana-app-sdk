package codegen

import (
	"cuelang.org/go/cue"
)

// Kind is a common interface declaration for code generation.
// Any type parser should be able to parse a kind into this definition to supply
// to various common Jennies in the codegen package.
type Kind interface {
	Name() string
	Properties() KindProperties
	Versions() []KindVersion
	Version(version string) *KindVersion
}

// KindProperties is the collection of properties for a Kind which are used for code generation
type KindProperties struct {
	// Kind is the unique-within-the-group name of the kind
	Kind string `json:"kind"`
	// Group is the group the Kind is a part of
	Group string `json:"group"`
	// ManifestGroup is the group shortname used by the AppManifest this Kind belongs to
	ManifestGroup string `json:"manifestGroup"`
	// MachineName is the machine version of the Kind, which follows the regex: /^[a-z]+[a-z0-9]*$/
	MachineName string `json:"machineName"`
	// PluralMachineName is the plural of the MachineName
	PluralMachineName string `json:"pluralMachineName"`
	// PluralName is the plural of the Kind
	PluralName string `json:"pluralName"`
	// Current is the version string of the version considered to be "current".
	// This does not have to be the latest, but determines preference when generating code.
	Current                string                      `json:"current"`
	Scope                  string                      `json:"scope"`
	Validation             KindAdmissionCapability     `json:"validation"`
	Mutation               KindAdmissionCapability     `json:"mutation"`
	Conversion             bool                        `json:"conversion"`
	ConversionWebhookProps ConversionWebhookProperties `json:"conversionWebhookProps"`
	// Codegen contains code-generation directives for the codegen pipeline
	Codegen KindCodegenProperties `json:"codegen"`
}

type ConversionWebhookProperties struct {
	URL string `json:"url"`
}

type KindAdmissionCapabilityOperation string

const (
	AdmissionCapabilityOperationCreate  KindAdmissionCapabilityOperation = "CREATE"
	AdmissionCapabilityOperationUpdate  KindAdmissionCapabilityOperation = "UPDATE"
	AdmissionCapabilityOperationDelete  KindAdmissionCapabilityOperation = "DELETE"
	AdmissionCapabilityOperationConnect KindAdmissionCapabilityOperation = "CONNECT"
	AdmissionCapabilityOperationAny     KindAdmissionCapabilityOperation = "*"
)

type KindAdmissionCapability struct {
	Operations []KindAdmissionCapabilityOperation `json:"operations"`
}

// KindCodegenProperties contains code generation directives for a Kind or KindVersion
type KindCodegenProperties struct {
	TS KindCodegenLanguageProperties[KindCodegenTSConfig] `json:"ts"`
	Go KindCodegenLanguageProperties[KindCodegenGoConfig] `json:"go"`
}

type KindCodegenLanguageProperties[T any] struct {
	Enabled bool `json:"enabled"`
	Config  T    `json:"config"`
}

// KindCodegenTSConfig is the TypeScript configuration options for codegen,
// modeled after the cog TS codegen options.
type KindCodegenTSConfig struct {
	ImportsMap        map[string]string `json:"importsMap"`
	EnumsAsUnionTypes bool              `json:"enumsAsUnionTypes"`
}

// KindCodegenGoConfig is the Go configuration options for codegen,
// modeled after the cog Go codegen options.
type KindCodegenGoConfig struct {
}

type AdditionalPrinterColumn struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Format      *string `json:"format,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *int32  `json:"priority"`
	JSONPath    string  `json:"jsonPath"`
}

// CustomRouteRequest represents the request part of a custom route definition.
type CustomRouteRequest struct {
	Query cue.Value `json:"query,omitempty"`
	Body  cue.Value `json:"body,omitempty"`
}

// CustomRouteResponse represents the response part of a custom route definition.
type CustomRouteResponse struct {
	Schema   cue.Value                   `json:"schema,omitempty"`
	Metadata CustomRouteResponseMetadata `json:"metadata,omitempty"`
}

type CustomRouteResponseMetadata struct {
	TypeMeta   bool `json:"typeMeta"`
	ListMeta   bool `json:"listMeta"`
	ObjectMeta bool `json:"objectMeta"`
}

// CustomRoute represents a single custom route definition for a specific HTTP method.
type CustomRoute struct {
	Name       string              `json:"name"`
	Request    CustomRouteRequest  `json:"request"`
	Response   CustomRouteResponse `json:"response"`
	Extensions map[string]any      `json:"extensions,omitempty"`
}

type KindVersion struct {
	Version string `json:"version"`
	// Schema is the CUE schema for the version
	// This should eventually be changed to JSONSchema/OpenAPI(/AST?)
	Schema                   cue.Value                         `json:"schema"` // TODO: this should eventually be OpenAPI/JSONSchema (ast or bytes?)
	Codegen                  KindCodegenProperties             `json:"codegen"`
	Served                   bool                              `json:"served"`
	SelectableFields         []string                          `json:"selectableFields"`
	Validation               KindAdmissionCapability           `json:"validation"`
	Mutation                 KindAdmissionCapability           `json:"mutation"`
	AdditionalPrinterColumns []AdditionalPrinterColumn         `json:"additionalPrinterColumns"`
	Routes                   map[string]map[string]CustomRoute `json:"routes"`
}

// AnyKind is a simple implementation of Kind
type AnyKind struct {
	Props       KindProperties
	AllVersions []KindVersion
}

func (a *AnyKind) Name() string {
	return a.Props.Kind
}

func (a *AnyKind) Properties() KindProperties {
	return a.Props
}

func (a *AnyKind) Versions() []KindVersion {
	return a.AllVersions
}

func (a *AnyKind) Version(v string) *KindVersion {
	for i := 0; i < len(a.AllVersions); i++ {
		if v == a.AllVersions[i].Version {
			return &a.AllVersions[i]
		}
	}
	return nil
}
