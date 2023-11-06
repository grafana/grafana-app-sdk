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

type KindProperties struct {
	Kind              string                 `json:"kind"`
	Group             string                 `json:"group"`
	MachineName       string                 `json:"machineName"`
	PluralMachineName string                 `json:"pluralMachineName"`
	PluralName        string                 `json:"pluralName"`
	Current           string                 `json:"current"`
	APIResource       *APIResourceProperties `json:"apiResource"`
	Codegen           KindCodegenProperties  `json:"codegen"`
}

type APIResourceProperties struct {
	Group string `json:"group"`
	Scope string `json:"scope"`
}

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
