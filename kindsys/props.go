package kindsys

import "github.com/grafana/thema"

// CommonProperties contains the metadata common to all categories of kinds.
type CommonProperties struct {
	Name              string   `json:"name"`
	PluralName        string   `json:"pluralName"`
	MachineName       string   `json:"machineName"`
	PluralMachineName string   `json:"pluralMachineName"`
	LineageIsGroup    bool     `json:"lineageIsGroup"`
	Maturity          Maturity `json:"maturity"`
	Description       string   `json:"description,omitempty"`
}

// CustomProperties represents the static properties in the definition of a
// Custom kind that are representable with basic Go types. This
// excludes Thema schemas.
type CustomProperties struct {
	CommonProperties
	CurrentVersion thema.SyntacticVersion `json:"currentVersion"`
	IsCRD          bool                   `json:"isCRD"`
	Group          string                 `json:"group"`
	CRD            struct {
		Group         string  `json:"group"`
		Scope         string  `json:"scope"`
		GroupOverride *string `json:"groupOverride"`
	} `json:"crd"`
	Codegen struct {
		Frontend bool `json:"frontend"`
		Backend  bool `json:"backend"`
	} `json:"codegen"`
}

func (m CustomProperties) Common() CommonProperties {
	return m.CommonProperties
}

// SomeKindProperties is an interface type to abstract over the different kind
// property struct types: [CoreProperties], [CustomProperties]
//
// It is the traditional interface counterpart to the generic type constraint
// KindProperties.
type SomeKindProperties interface {
	Common() CommonProperties
}

// KindProperties is a type parameter that comprises the base possible set of
// kind metadata configurations.
type KindProperties interface {
	CustomProperties
}
