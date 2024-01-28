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

	"github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen"
)

//go:embed def.cue cue.mod/module.cue
var overlayFS embed.FS

func NewParser() (*Parser, error) {
	return &Parser{}, nil
}

type Parser struct {
	kindDef   *cue.Value
	schemaDef *cue.Value
}

// Parse parses all CUE files in `files`, and reads all top-level selectors (or only `selectors` if provided)
// as kinds as defined by [def.cue]. It then returns a list of kinds parsed.
//
//nolint:funlen
func (p *Parser) Parse(files fs.FS, selectors ...string) ([]codegen.Kind, error) {
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
	vals := make([]cue.Value, 0)
	if len(selectors) > 0 {
		for _, s := range selectors {
			v := root.LookupPath(cue.MakePath(cue.Str(s)))
			if v.Err() != nil {
				return nil, v.Err()
			}
			vals = append(vals, v)
		}
	} else {
		i, err := root.Fields()
		if err != nil {
			return nil, err
		}
		for i.Next() {
			vals = append(vals, i.Value())
		}
	}

	// Load the kind definition (this function does this only once regardless of how many times the user calls Parse())
	kindDef, schemaDef, err := p.getKindDefinition()
	if err != nil {
		return nil, fmt.Errorf("could not load internal kind definition: %w", err)
	}

	// Unify the kinds we loaded from CUE with the kind definition,
	// then put together the kind struct from that
	kinds := make([]codegen.Kind, 0)
	for _, val := range vals {
		// Start by unifying the provided cue.Value with the cue.Value that contains our Kind definition.
		// This gives us default values for all fields that weren't filled out,
		// and will create errors for required fields that may be missing.
		val = val.Unify(kindDef)
		if val.Err() != nil {
			return nil, val.Err()
		}

		// Decode the unified value into our collection of properties.
		props := codegen.KindProperties{}
		err = val.Decode(&props)
		if err != nil {
			return nil, err
		}

		// We can't simply decode the version map, because we need to extract some values as types,
		// but leave the schema value as a cue.Value. So we tell cue to decode it into a map,
		// then still need to iterate through the map and adjust values
		someKind := &codegen.AnyKind{
			Props:       props,
			AllVersions: make([]codegen.KindVersion, 0),
		}
		goVers := make(map[string]codegen.KindVersion)
		vers := val.LookupPath(cue.MakePath(cue.Str("versions")))
		if vers.Err() != nil {
			return nil, vers.Err()
		}
		err = vers.Decode(&goVers)
		if err != nil {
			return nil, err
		}
		for k, v := range goVers {
			v.Schema = val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("schema")))
			if v.Schema.Err() != nil {
				return nil, v.Schema.Err()
			}
			if props.APIResource != nil {
				// Normally, we would use a conditional unify in the def.cue file of kindDef,
				// but there is a bug where the conditional evaluation creates a nil vertex somewhere
				// when loading with the CLI, so this is a faster fix (TODO: long-term fix)
				v.Schema = v.Schema.Unify(schemaDef)
				if v.Schema.Err() != nil {
					return nil, v.Schema.Err()
				}
			}
			someKind.AllVersions = append(someKind.AllVersions, v)
		}
		kinds = append(kinds, someKind)
	}
	return kinds, nil
}

func (p *Parser) getKindDefinition() (cue.Value, cue.Value, error) {
	if p.kindDef != nil {
		return *p.kindDef, *p.schemaDef, nil
	}

	kindOverlay := make(map[string]load.Source)
	err := ToOverlay("/github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen/cuekind", overlayFS, kindOverlay)
	if err != nil {
		return cue.Value{}, cue.Value{}, err
	}
	kindInstWithDef := load.Instances(nil, &load.Config{
		Overlay:    kindOverlay,
		ModuleRoot: filepath.FromSlash("/github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen/cuekind"),
		Module:     "github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen/cuekind",
		Dir:        filepath.FromSlash("/github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen/cuekind"),
	})[0]
	inst := cuecontext.New().BuildInstance(kindInstWithDef)
	if inst.Err() != nil {
		return cue.Value{}, cue.Value{}, inst.Err()
	}
	kindDef := inst.LookupPath(cue.MakePath(cue.Str("Kind")))
	if kindDef.Err() != nil {
		return cue.Value{}, cue.Value{}, kindDef.Err()
	}
	schemaDef := inst.LookupPath(cue.MakePath(cue.Str("Schema")))
	if schemaDef.Err() != nil {
		return cue.Value{}, cue.Value{}, schemaDef.Err()
	}
	p.kindDef = &kindDef
	p.schemaDef = &schemaDef
	return *p.kindDef, *p.schemaDef, nil
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
