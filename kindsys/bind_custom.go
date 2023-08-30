package kindsys

import (
	"github.com/grafana/thema"
)

// genericCustom is a general representation of a parsed and validated Custom kind.
type genericCustom struct {
	def Def[CustomProperties]
	lin thema.Lineage
}

var _ Custom = genericCustom{}

// Props returns the generic SomeKindProperties
func (k genericCustom) Props() SomeKindProperties {
	return k.def.Properties
}

// Name returns the Name property
func (k genericCustom) Name() string {
	return k.def.Properties.Name
}

// MachineName returns the MachineName property
func (k genericCustom) MachineName() string {
	return k.def.Properties.MachineName
}

// Maturity returns the Maturity property
func (k genericCustom) Maturity() Maturity {
	return k.def.Properties.Maturity
}

// Def returns a Def with the type of ExtendedProperties, containing the bound ExtendedProperties
func (k genericCustom) Def() Def[CustomProperties] {
	return k.def
}

// Lineage returns the underlying bound Lineage
func (k genericCustom) Lineage() thema.Lineage {
	return k.lin
}

// BindCustom creates a Custom-implementing type from a def, runtime, and opts
//
//nolint:lll
func BindCustom(rt *thema.Runtime, def Def[CustomProperties], opts ...thema.BindOption) (Custom, error) {
	lin, err := def.Some().BindKindLineage(rt, opts...)
	if err != nil {
		return nil, err
	}

	return genericCustom{
		def: def,
		lin: lin,
	}, nil
}
