package codegen

import "cuelang.org/go/cue"

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
	// MachineName is the machine version of the Kind, which follows the regex: /^[a-z]+[a-z0-9]*$/
	MachineName string `json:"machineName"`
	// PluralMachineName is the plural of the MachineName
	PluralMachineName string `json:"pluralMachineName"`
	// PluralName is the plural of the Kind
	PluralName string `json:"pluralName"`
	// Current is the version string of the version considered to be "current".
	// This does not have to be the latest, but determines preference when generating code.
	Current string `json:"current"`
	// APIResource is an optional field which, if present, indicates that the Kind is expressible as a kubernetes API resource,
	// and contains attributes which allow identification as such.
	APIResource *APIResourceProperties `json:"apiResource"`
	// Codegen contains code-generation directives for the codegen pipeline
	Codegen KindCodegenProperties `json:"codegen"`
}

// APIResourceProperties contains information about a Kind expressible as a kubernetes API resource
type APIResourceProperties struct {
	Group string `json:"group"`
	Scope string `json:"scope"`
}

// KindCodegenProperties contains code generation directives for a Kind or KindVersion
type KindCodegenProperties struct {
	Frontend bool `json:"frontend"`
	Backend  bool `json:"backend"`
}

type KindVersion struct {
	Version string `json:"version"`
	// Schema is the CUE schema for the version
	// This should eventually be changed to JSONSchema/OpenAPI(/AST?)
	Schema  cue.Value             `json:"schema"` // TODO: this should eventually be OpenAPI/JSONSchema (ast or bytes?)
	Codegen KindCodegenProperties `json:"codegen"`
	Served  bool                  `json:"served"`
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
