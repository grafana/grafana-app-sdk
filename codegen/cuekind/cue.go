package cuekind

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

//go:embed def.cue cue.mod/module.cue
var overlayFS embed.FS

type Cue struct {
	Root cue.Value
	Defs cue.Value
}

func LoadCue(files fs.FS) (*Cue, error) {
	root, err := loadRoot(files)
	if err != nil {
		return nil, err
	}
	defs, err := loadDefs()
	if err != nil {
		return nil, err
	}

	return &Cue{
		Root: root,
		Defs: defs,
	}, nil
}

func loadRoot(files fs.FS) (cue.Value, error) {
	// Load the FS
	// Get the module from cue.mod/module.cue
	modFile, err := files.Open("cue.mod/module.cue")
	if err != nil {
		return cue.Value{}, fmt.Errorf("provided fs.FS is not a valid CUE module: error opening cue.mod/module.cue: %w", err)
	}
	defer modFile.Close()
	modFileContents, err := io.ReadAll(modFile)
	if err != nil {
		return cue.Value{}, errors.New("error reading contents of cue.mod/module.cue")
	}
	cueMod := cuecontext.New().CompileString(string(modFileContents))
	if cueMod.Err() != nil {
		return cue.Value{}, cueMod.Err()
	}
	modPath, _ := cueMod.LookupPath(cue.MakePath(cue.Str("module"))).String()

	overlay := make(map[string]load.Source)
	err = toOverlay(filepath.Join("/", modPath), files, overlay)
	if err != nil {
		return cue.Value{}, err
	}
	inst := load.Instances(nil, &load.Config{
		Overlay:    overlay,
		ModuleRoot: filepath.FromSlash(filepath.Join("/", modPath)),
		Module:     modPath,
		Dir:        filepath.FromSlash(filepath.Join("/", modPath)),
	})
	if len(inst) != 1 {
		return cue.Value{}, errors.New("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return cue.Value{}, root.Err()
	}

	return root, nil
}

func loadDefs() (cue.Value, error) {
	kindOverlay := make(map[string]load.Source)
	err := toOverlay("/github.com/grafana/grafana-app-sdk/codegen/cuekind", overlayFS, kindOverlay)
	if err != nil {
		return cue.Value{}, err
	}
	kindInstWithDef := load.Instances(nil, &load.Config{
		Overlay:    kindOverlay,
		ModuleRoot: filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
		Module:     "github.com/grafana/grafana-app-sdk/codegen/cuekind",
		Dir:        filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
	})[0]
	inst := cuecontext.New().BuildInstance(kindInstWithDef)
	if inst.Err() != nil {
		return cue.Value{}, inst.Err()
	}

	return inst, nil
}

func toOverlay(prefix string, vfs fs.FS, overlay map[string]load.Source) error {
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
