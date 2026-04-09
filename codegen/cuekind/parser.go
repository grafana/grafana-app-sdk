package cuekind

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
)

const DefaultManifestSelector = "manifest"

type Parser struct {
	root        cue.Value
	manifestDef cue.Value
}

type parser[T any] struct {
	parseFunc func(selectors ...string) ([]T, error)
}

// NewParser creates a new parser instance for the provided CUE value and config.
func NewParser(c *Cue, enableOperatorStatusGeneration bool) (*Parser, error) {
	manifestDef, err := getManifestDefinition(c.Defs, enableOperatorStatusGeneration)
	if err != nil {
		return nil, fmt.Errorf("could not load manifest definition: %w", err)
	}

	return &Parser{
		root:        c.Root,
		manifestDef: manifestDef,
	}, nil
}

func (p *parser[T]) Parse(selectors ...string) ([]T, error) {
	return p.parseFunc(selectors...)
}

func (p *Parser) ManifestParser() codegen.Parser[codegen.AppManifest] {
	return &parser[codegen.AppManifest]{
		parseFunc: func(s ...string) ([]codegen.AppManifest, error) {
			if len(s) == 0 {
				s = []string{DefaultManifestSelector}
			}
			manifests := make([]codegen.AppManifest, 0, len(s))
			for _, selector := range s {
				m, err := p.ParseManifest(selector)
				if err != nil {
					return nil, err
				}
				manifests = append(manifests, m)
			}
			return manifests, nil
		},
	}
}

// ParseManifest parses ManifestSelector (or the root object if no selector is provided) as a CUE app manifest,
// returning the parsed codegen.AppManifest object or an error.
func (p *Parser) ParseManifest(manifestSelector string) (codegen.AppManifest, error) {
	val := p.root
	if manifestSelector != "" {
		val = val.LookupPath(cue.MakePath(cue.Str(manifestSelector)))
	}

	val = val.Unify(p.manifestDef)
	if val.Err() != nil {
		return nil, val.Err()
	}
	manifest := &codegen.SimpleManifest{}
	if err := val.Decode(&manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// getManifestDefinition loads the manifest CUE definition
// revive complains about the usage of control flag, but it's not a problem here.
// nolint:revive
func getManifestDefinition(defs cue.Value, genOperatorState bool) (cue.Value, error) {
	var schemaDef cue.Value
	if genOperatorState {
		schemaDef = defs.LookupPath(cue.MakePath(cue.Str("SchemaWithOperatorState")))
	} else {
		schemaDef = defs.LookupPath(cue.MakePath(cue.Str("Schema")))
	}

	// Bake the schema constraint into Kind within defs so that
	// Manifest (which references it) inherits the correct schema.
	defs = defs.FillPath(cue.MakePath(cue.Str("Kind"), cue.Str("schema")), schemaDef)

	manifestDef := defs.LookupPath(cue.MakePath(cue.Str("Manifest")))

	return manifestDef,
		errors.Join(
			schemaDef.Err(),
			manifestDef.Err(),
		)
}
