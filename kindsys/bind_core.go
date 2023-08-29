package kindsys

import (
	"github.com/grafana/thema"
)

// genericCore is a general representation of a parsed and validated Core kind.
type genericCore struct {
	def Def[CoreProperties]
	lin thema.Lineage
}

var _ Core = genericCore{}

func (k genericCore) Props() SomeKindProperties {
	return k.def.Properties
}

func (k genericCore) Name() string {
	return k.def.Properties.Name
}

func (k genericCore) MachineName() string {
	return k.def.Properties.MachineName
}

func (k genericCore) Maturity() Maturity {
	return k.def.Properties.Maturity
}

func (k genericCore) Def() Def[CoreProperties] {
	return k.def
}

func (k genericCore) Lineage() thema.Lineage {
	return k.lin
}

// TODO docs
func BindCore(rt *thema.Runtime, def Def[CoreProperties], opts ...thema.BindOption) (Core, error) {
	lin, err := def.Some().BindKindLineage(rt, opts...)
	if err != nil {
		return nil, err
	}

	return genericCore{
		def: def,
		lin: lin,
	}, nil
}
