//nolint:lll
package thema

import (
	"fmt"
	"io/fs"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"
	"github.com/grafana/thema"
	"github.com/grafana/thema/load"
	"github.com/hashicorp/go-multierror"

	"github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/kindsys"
)

func NewParser(rt *thema.Runtime) (*Parser, error) {
	return &Parser{
		rt: rt,
	}, nil
}

type Parser struct {
	rt *thema.Runtime
}

func (p *Parser) Parse(modFS fs.FS, selectors ...string) ([]kindsys.Custom, error) {
	// Load in the cue from the modFS using thema
	inst, err := load.InstanceWithThema(modFS, ".")
	if err != nil {
		return nil, err
	}

	root := p.rt.Context().BuildInstance(inst)
	if root.Err() != nil {
		return nil, root.Err()
	}
	if !root.Exists() {
		return nil, fmt.Errorf("unable to load Instance")
	}

	// Get the relevant selectors
	allDefs := make([]kindsys.Custom, 0)
	if len(selectors) == 0 {
		i, err := root.Fields()
		if err != nil {
			return nil, err
		}
		for i.Next() {
			def, err := kindsys.ToDef[kindsys.CustomProperties](i.Value())
			if err != nil {
				return nil, err
			}
			custom, err := kindsys.BindCustom(p.rt, def)
			if err != nil {
				return nil, err
			}
			allDefs = append(allDefs, custom)
		}
	} else {
		for _, s := range selectors {
			v := root.LookupPath(cue.ParsePath(s))
			if !v.Exists() {
				return nil, fmt.Errorf("%s: selector does not exist", s)
			}
			if v.Err() != nil {
				return nil, fmt.Errorf("%s: %w", s, v.Err())
			}

			def, err := kindsys.ToDef[kindsys.CustomProperties](v)
			if err != nil {
				return nil, err
			}
			custom, err := kindsys.BindCustom(p.rt, def)
			if err != nil {
				return nil, err
			}
			allDefs = append(allDefs, custom)
		}
	}

	return allDefs, nil
}

// Generator is an interface which describes any code generator that can be passed to a Parser, such as CustomKindParser.
type Generator interface {
	Generate(objs ...kindsys.Custom) (codejen.Files, error)
}

// FilteredGenerator allows a Generator to express an opinion on whether it should be used for a particular
// Custom-implementing type. When Allowed returns false, the Generator may return an error on Generate.
type FilteredGenerator interface {
	Generator
	// Allowed returns true if the Generator can be used for this particular object
	Allowed(kindsys.Custom) bool
}

// Filter wraps a Generator to create a FilteredGenerator, using the supplied filterFunc to create the
// Allowed and Filter functions of the FilteredGenerator.
func Filter(g Generator, filterFunc func(kindsys.Custom) bool) FilteredGenerator {
	return &genericFilteredGenerator{
		Generator:  g,
		filterFunc: filterFunc,
	}
}

type genericFilteredGenerator struct {
	Generator
	filterFunc func(kindsys.Custom) bool
}

func (g *genericFilteredGenerator) Allowed(c kindsys.Custom) bool {
	return g.filterFunc(c)
}

func kindsysNamerFunc(d kindsys.Custom) string {
	if d == nil {
		return "nil"
	}
	if d.Lineage() == nil {
		return "nil Lineage"
	}
	return d.Lineage().Name()
}

// CustomKindParser allows for parsing of github.com/grafana/thema cue Lineages, encapsulated as grafana CustomStructured
// kinds. The parser can validate inputs for correctness, and can generate codegen metadata objects from the
// provided CUE and pass them off to a Generator.
//
// The CustomKindParser currently understands only grafana CustomStructured CUE types,
// with plans to support Thema #CRD types in the future.
type CustomKindParser struct {
	rt        *thema.Runtime
	root      cue.Value
	allFields []cue.Value // For caching
}

// NewCustomKindParser creates a new CustomKindParser from the provided thema.Runtime and fs.FS.
// The fs should contain the cue.mod directory as well as any cue files expected to be parsed by the CustomKindParser.
// If thema cannot parse the provided fs.FS, an error will be returned.
func NewCustomKindParser(rt *thema.Runtime, modFS fs.FS) (*CustomKindParser, error) {
	inst, err := load.InstanceWithThema(modFS, ".")
	if err != nil {
		return nil, err
	}

	val := rt.Context().BuildInstance(inst)
	if val.Err() != nil {
		return nil, val.Err()
	}
	if !val.Exists() {
		return nil, fmt.Errorf("unable to load Instance")
	}

	return &CustomKindParser{
		rt:   rt,
		root: val,
	}, nil
}

// ListAllMainSelectors returns a list of string selectors that are top-level declarations in the CUE runtime.
// These selectors are the ones which will be considered by default in calls to Validate, Generate, and
// FilteredGenerate if no selectors are supplied.
func (g *CustomKindParser) ListAllMainSelectors() ([]string, error) {
	vals, err := g.loadAllCueValues()
	if err != nil {
		return nil, err
	}
	selectors := make([]string, 0)
	for _, v := range vals {
		selectors = append(selectors, v.Path().String())
	}
	return selectors, nil
}

// Validate validates the provided selectors, returning a map of <selector> -> <validation error list>.
// If no validation errors are found, len(map) will be 0.
// If no selectors are provided, all top-level declarations will be validated.
func (g *CustomKindParser) Validate(selectors ...string) (map[string]multierror.Error, error) {
	errs := make(map[string]multierror.Error, 0)
	if len(selectors) == 0 {
		var err error
		selectors, err = g.ListAllMainSelectors()
		if err != nil {
			return nil, err
		}
	}
	for _, s := range selectors {
		v := g.root.LookupPath(g.getPathFromString(s))
		if !v.Exists() {
			errs[s] = multierror.Error{
				Errors: []error{fmt.Errorf("selector does not exist")},
			}
			continue
		}
		if v.Err() != nil {
			errs[s] = multierror.Error{
				Errors: []error{v.Err()},
			}
			continue
		}

		def, err := kindsys.ToDef[kindsys.CustomProperties](v)
		if err != nil {
			errs[s] = multierror.Error{
				Errors: []error{err},
			}
			continue
		}
		_, err = kindsys.BindCustom(g.rt, def)
		if err != nil {
			errs[s] = multierror.Error{
				Errors: []error{err},
			}
			continue
		}
	}

	return errs, nil
}

// Generate parses the provided selectors and passes the parsed values to the Generator,
// returning the files created from each step. If no selectors are passed, all top-level declarations will be used.
func (g *CustomKindParser) Generate(generator Generator, selectors ...string) (codejen.Files, error) {
	if generator == nil {
		return nil, fmt.Errorf("generator must be non-nil")
	}

	return g.FilteredGenerate(Filter(generator, func(kindsys.Custom) bool {
		return true
	}), selectors...)
}

// FilteredGenerate parses the provided selectors, checks the parsed object against generator.Allowed(),
// and passes the resulting list of filtered objects to the FilteredGenerator,
// returning the files created from each step. If no selectors are passed, all supported selectors will be used.
func (g *CustomKindParser) FilteredGenerate(generator FilteredGenerator, selectors ...string) (codejen.Files, error) {
	if generator == nil {
		return nil, fmt.Errorf("generator must be non-nil")
	}

	if len(selectors) == 0 {
		var err error
		selectors, err = g.ListAllMainSelectors()
		if err != nil {
			return nil, err
		}
	}

	allDefs := make([]kindsys.Custom, 0)

	for _, s := range selectors {
		v := g.root.LookupPath(g.getPathFromString(s))
		if !v.Exists() {
			return nil, fmt.Errorf("%s: selector does not exist", s)
		}
		if v.Err() != nil {
			return nil, fmt.Errorf("%s: %w", s, v.Err())
		}

		def, err := kindsys.ToDef[kindsys.CustomProperties](v)
		if err != nil {
			return nil, err
		}
		custom, err := kindsys.BindCustom(g.rt, def)
		if err != nil {
			return nil, err
		}

		// Check if this generator will run for this object
		if !generator.Allowed(custom) {
			continue
		}

		allDefs = append(allDefs, custom)
	}

	return generator.Generate(allDefs...)
}

func (g *CustomKindParser) loadAllCueValues() ([]cue.Value, error) {
	if g.allFields != nil {
		return g.allFields, nil
	}
	i, err := g.root.Fields()
	if err != nil {
		return nil, err
	}
	vals := make([]cue.Value, 0)
	for i.Next() {
		vals = append(vals, i.Value())
	}
	g.allFields = vals
	return vals, nil
}

func (*CustomKindParser) getPathFromString(selector string) cue.Path {
	parts := strings.Split(selector, ".")
	selectors := make([]cue.Selector, 0)
	for _, p := range parts {
		selectors = append(selectors, cue.Str(p))
	}
	return cue.MakePath(selectors...)
}

func versionString(version thema.SyntacticVersion) string {
	return fmt.Sprintf("v%d-%d", version[0], version[1])
}
