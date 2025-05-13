package cuekind

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/grafana/grafana-app-sdk/codegen"
)

const DefaultManifestSelector = "manifest"

//go:embed def.cue cue.mod/module.cue
var overlayFS embed.FS

func NewParser() (*Parser, error) {
	return &Parser{}, nil
}

type Parser struct {
	kindDef     *cue.Value
	schemaDef   *cue.Value
	manifestDef *cue.Value
}

type parser[T any] struct {
	parseFunc func(fs.FS, ...string) ([]T, error)
}

func (p *parser[T]) Parse(f fs.FS, args ...string) ([]T, error) {
	return p.parseFunc(f, args...)
}

func (p *Parser) ManifestParser(genOperatorState bool) codegen.Parser[codegen.AppManifest] {
	return &parser[codegen.AppManifest]{
		parseFunc: func(f fs.FS, s ...string) ([]codegen.AppManifest, error) {
			if len(s) == 0 {
				s = []string{"manifest"}
			}
			manifests := make([]codegen.AppManifest, 0, len(s))
			for _, selector := range s {
				m, err := p.ParseManifest(f, selector, genOperatorState)
				if err != nil {
					return nil, err
				}
				manifests = append(manifests, m)
			}
			return manifests, nil
		},
	}
}

// KindParser returns a Parser that returns a list of codegen.Kind.
// If useManifest is true, it will load kinds from a manifest provided by the selector(s) in Parse (or DefaultManifestSelector if no selectors are present),
// rather than loading the selector(s) as kinds.
//
//nolint:revive
func (p *Parser) KindParser(useManifest bool, genOperatorState bool) codegen.Parser[codegen.Kind] {
	return &parser[codegen.Kind]{
		parseFunc: func(f fs.FS, s ...string) ([]codegen.Kind, error) {
			if useManifest {
				if len(s) == 0 {
					s = []string{"manifest"}
				}
				kinds := make([]codegen.Kind, 0)
				for _, selector := range s {
					m, err := p.ParseManifest(f, selector, genOperatorState)
					if err != nil {
						return nil, err
					}
					kinds = append(kinds, m.Kinds()...)
				}
				return kinds, nil
			}
			return nil, fmt.Errorf("parsing kinds without manifest no longer supported")
		},
	}
}

// ParseManifest parses ManifestSelector (or the root object if no selector is provided) as a CUE app manifest,
// returning the parsed codegen.AppManifest object or an error.
//
//nolint:funlen
func (p *Parser) ParseManifest(files fs.FS, manifestSelector string, genOperatorState bool) (codegen.AppManifest, error) {
	// Load the FS
	// Get the module from cue.mod/module.cue
	modFile, err := files.Open("cue.mod/module.cue")
	if err != nil {
		return nil, fmt.Errorf("provided fs.FS is not a valid CUE module: error opening cue.mod/module.cue: %w", err)
	}
	defer modFile.Close()
	modFileContents, err := io.ReadAll(modFile)
	if err != nil {
		return nil, fmt.Errorf("error reading contents of cue.mod/module.cue")
	}
	cueMod := cuecontext.New().CompileString(string(modFileContents))
	if cueMod.Err() != nil {
		return nil, cueMod.Err()
	}
	modPath, _ := cueMod.LookupPath(cue.MakePath(cue.Str("module"))).String()

	overlay := make(map[string]load.Source)
	err = ToOverlay(filepath.Join("/", modPath), files, overlay)
	if err != nil {
		return nil, err
	}
	inst := load.Instances(nil, &load.Config{
		Overlay:    overlay,
		ModuleRoot: filepath.FromSlash(filepath.Join("/", modPath)),
		Module:     modPath,
		Dir:        filepath.FromSlash(filepath.Join("/", modPath)),
	})
	if len(inst) == 0 {
		return nil, fmt.Errorf("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return nil, root.Err()
	}
	var val cue.Value = root
	if manifestSelector != "" {
		val = root.LookupPath(cue.MakePath(cue.Str(manifestSelector)))
	}

	// Load the kind definition (this function does this only once regardless of how many times the user calls Parse())
	kindDef, schemaDef, manifestDef, err := p.getKindDefinition(genOperatorState)
	if err != nil {
		return nil, fmt.Errorf("could not load internal kind definition: %w", err)
	}

	val = val.Unify(manifestDef)
	if val.Err() != nil {
		return nil, val.Err()
	}

	// Decode
	manifestProps := codegen.AppManifestProperties{}
	err = val.Decode(&manifestProps)
	if err != nil {
		return nil, err
	}

	manifest := &codegen.SimpleManifest{
		Props: manifestProps,
	}

	manifest.AllVersions = make([]codegen.Version, 0)
	versionsVal := val.LookupPath(cue.MakePath(cue.Str("versions")))
	if versionsVal.Err() != nil {
		return nil, versionsVal.Err()
	}
	it, err := versionsVal.Fields()
	if err != nil {
		return nil, err
	}
	for it.Next() {
		ver := it.Value()
		vProps := codegen.VersionProperties{}
		err = ver.Decode(&vProps)
		if err != nil {
			return nil, err
		}
		version := &codegen.SimpleVersion{
			Props:    vProps,
			AllKinds: make([]codegen.VersionedKind, 0),
		}
		kinds := ver.LookupPath(cue.MakePath(cue.Str("kinds")))
		if kinds.Err() != nil {
			return nil, kinds.Err()
		}
		kit, err := kinds.List()
		if err != nil {
			return nil, err
		}
		for kit.Next() {
			kind, err := p.parseKind(kit.Value(), kindDef, schemaDef)
			if err != nil {
				return nil, err
			}
			version.AllKinds = append(version.AllKinds, *kind)
		}
		manifest.AllVersions = append(manifest.AllVersions, version)
	}

	return manifest, nil
}

func (*Parser) parseKind(val cue.Value, kindDef, schemaDef cue.Value) (*codegen.VersionedKind, error) {
	// Start by unifying the provided cue.Value with the cue.Value that contains our Kind definition.
	// This gives us default values for all fields that weren't filled out,
	// and will create errors for required fields that may be missing.
	val = val.Unify(kindDef)
	if val.Err() != nil {
		return nil, val.Err()
	}

	// We can't simply decode the version map, because we need to extract some values as types,
	// but leave the schema value as a cue.Value. So we tell cue to decode it into a map,
	// then still need to iterate through the map and adjust values
	someKind := &codegen.VersionedKind{}
	err := val.Decode(someKind)
	if err != nil {
		return nil, err
	}

	someKind.Schema = val.LookupPath(cue.MakePath(cue.Str("schema")))

	// Normally, we would use a conditional unify in the def.cue file of kindDef,
	// but there is a bug where the conditional evaluation creates a nil vertex somewhere
	// when loading with the CLI, so this is a faster fix (TODO: long-term fix)
	someKind.Schema = someKind.Schema.Unify(schemaDef)
	if someKind.Schema.Err() != nil {
		return nil, someKind.Schema.Err()
	}

	return someKind, nil
}

// revive complains about the usage of control flag, but it's not a problem here.
// nolint:revive
func (p *Parser) getKindDefinition(genOperatorState bool) (cue.Value, cue.Value, cue.Value, error) {
	if p.kindDef != nil {
		return *p.kindDef, *p.schemaDef, *p.manifestDef, nil
	}

	kindOverlay := make(map[string]load.Source)
	err := ToOverlay("/github.com/grafana/grafana-app-sdk/codegen/cuekind", overlayFS, kindOverlay)
	if err != nil {
		return cue.Value{}, cue.Value{}, cue.Value{}, err
	}
	kindInstWithDef := load.Instances(nil, &load.Config{
		Overlay:    kindOverlay,
		ModuleRoot: filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
		Module:     "github.com/grafana/grafana-app-sdk/codegen/cuekind",
		Dir:        filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
	})[0]
	inst := cuecontext.New().BuildInstance(kindInstWithDef)
	if inst.Err() != nil {
		return cue.Value{}, cue.Value{}, cue.Value{}, inst.Err()
	}
	kindDef := inst.LookupPath(cue.MakePath(cue.Str("Kind")))
	if kindDef.Err() != nil {
		return cue.Value{}, cue.Value{}, cue.Value{}, kindDef.Err()
	}

	var schemaDef cue.Value
	if genOperatorState {
		schemaDef = inst.LookupPath(cue.MakePath(cue.Str("SchemaWithOperatorState")))
		if schemaDef.Err() != nil {
			return cue.Value{}, cue.Value{}, cue.Value{}, schemaDef.Err()
		}
	} else {
		schemaDef = inst.LookupPath(cue.MakePath(cue.Str("Schema")))
		if schemaDef.Err() != nil {
			return cue.Value{}, cue.Value{}, cue.Value{}, schemaDef.Err()
		}
	}

	manifestDef := inst.LookupPath(cue.MakePath(cue.Str("Manifest")))
	if manifestDef.Err() != nil {
		return cue.Value{}, cue.Value{}, cue.Value{}, manifestDef.Err()
	}

	p.kindDef = &kindDef
	p.schemaDef = &schemaDef
	p.manifestDef = &manifestDef

	return *p.kindDef, *p.schemaDef, *p.manifestDef, nil
}

func ToOverlay(prefix string, vfs fs.FS, overlay map[string]load.Source) error {
	// TODO why not just stick the prefix on automatically...?
	if !filepath.IsAbs(prefix) {
		return fmt.Errorf("must provide absolute path prefix when generating cue overlay, got %q", prefix)
	}
	err := fs.WalkDir(vfs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		f, err := vfs.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() // nolint: errcheck

		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		overlay[filepath.Join(prefix, path)] = load.FromBytes(b)
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
