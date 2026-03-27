package cuekind

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
)

const DefaultManifestSelector = "manifest"

type Parser struct {
	root                 cue.Value
	perKindVersion       bool
	loadedCUEDefinitions *cueDefinitions
}

type parser[T any] struct {
	parseFunc func(selectors ...string) ([]T, error)
}

type cueDefinitions struct {
	Manifest    cue.Value
	OldManifest cue.Value
}

// NewParser creates a new parser instance for the provided CUE value and config.
func NewParser(c *Cue, enableOperatorStatusGeneration, perKindVersion bool) (*Parser, error) {
	defs, err := getCUEDefinitions(c.Defs, enableOperatorStatusGeneration)
	if err != nil {
		return nil, fmt.Errorf("could not load internal kind definition: %w", err)
	}

	return &Parser{
		root:                 c.Root,
		perKindVersion:       perKindVersion,
		loadedCUEDefinitions: defs,
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

	if p.perKindVersion {
		val = val.Unify(p.loadedCUEDefinitions.OldManifest)
		if val.Err() != nil {
			return nil, val.Err()
		}
		manifest := OldManifest{}
		if err := val.Decode(&manifest); err != nil {
			return nil, err
		}
		return manifest.toSimpleManifest(), nil
	}

	val = val.Unify(p.loadedCUEDefinitions.Manifest)
	if val.Err() != nil {
		return nil, val.Err()
	}
	manifest := &codegen.SimpleManifest{}
	if err := val.Decode(&manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// getCUEDefinitions loads CUE definitions for various types if not yet loaded,
// and returns a cueDefinitions object with the CUE values for them.
// revive complains about the usage of control flag, but it's not a problem here.
// nolint:revive
func getCUEDefinitions(defs cue.Value, genOperatorState bool) (*cueDefinitions, error) {
	var schemaDef cue.Value
	if genOperatorState {
		schemaDef = defs.LookupPath(cue.MakePath(cue.Str("SchemaWithOperatorState")))
	} else {
		schemaDef = defs.LookupPath(cue.MakePath(cue.Str("Schema")))
	}

	// Bake the schema constraint into Kind and KindOld within defs so that
	// Manifest/ManifestOld (which reference them) inherit the correct schema.
	defs = defs.FillPath(cue.MakePath(cue.Str("Kind"), cue.Str("schema")), schemaDef)
	defs = defs.FillPath(cue.MakePath(cue.Str("KindOld"), cue.Str("versions"), cue.AnyString, cue.Str("schema")), schemaDef)

	manifestDef := defs.LookupPath(cue.MakePath(cue.Str("Manifest")))
	oldManifestDef := defs.LookupPath(cue.MakePath(cue.Str("ManifestOld")))

	return &cueDefinitions{
			Manifest:    manifestDef,
			OldManifest: oldManifestDef,
		}, errors.Join(
			schemaDef.Err(),
			manifestDef.Err(),
			oldManifestDef.Err(),
		)
}
