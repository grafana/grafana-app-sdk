package cuekind

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/grafana/grafana-app-sdk/codegen"
)

//go:embed def.cue
var overlayFS embed.FS

type Parser struct {
	kindDef *cue.Value
}

var cueModRegex = regexp.MustCompile(`^module:[\s]+"([^"]+)"$`)

func (p *Parser) ParseFS(files fs.FS, selectors ...string) ([]codegen.Kind, error) {
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
	fmt.Println(filepath.FromSlash(filepath.Join("/", modPath)))
	inst := load.Instances(nil, &load.Config{
		Overlay:    overlay,
		ModuleRoot: filepath.FromSlash(filepath.Join("/", modPath)),
		Module:     modPath,
		Dir:        filepath.FromSlash(filepath.Join("/", modPath)),
	})
	if len(inst) < 0 {
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
			st := cueFmtState{}
			i.Value().Format(&st, 'v')
			os.WriteFile("./foo/fs-"+i.Label()+"-pre.cue", st.Bytes(), fs.ModePerm)
			vals = append(vals, i.Value())
		}
	}

	// Load the kind definition (do this only once)
	kindDef, err := p.getKindDefinition()
	if err != nil {
		return nil, fmt.Errorf("could not load internal kind definition: %w", err)
	}

	kinds := make([]codegen.Kind, 0)
	for _, val := range vals {
		val = val.Unify(kindDef)
		if val.Err() != nil {
			return nil, val.Err()
		}
		props := codegen.KindProperties{}
		val.Decode(&props)
		someKind := &codegen.AnyKind{
			Props:       props,
			AllVersions: make([]codegen.KindVersion, 0),
		}
		goVers := make(map[string]codegen.KindVersion)
		vers := val.LookupPath(cue.MakePath(cue.Str("versions")))
		vers.Decode(&goVers)
		for k, v := range goVers {
			v.Schema = val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("schema")))
			someKind.AllVersions = append(someKind.AllVersions, v)
		}
		kinds = append(kinds, someKind)
	}

	return kinds, nil
}

func (p *Parser) Parse(directory string, selectors ...string) ([]codegen.Kind, error) {
	kindDef, _ := p.getKindDefinition()
	inst := load.Instances(nil, &load.Config{
		Dir: directory,
	})
	if len(inst) < 0 {
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

	kinds := make([]codegen.Kind, 0)
	for _, val := range vals {
		st := cueFmtState{}
		val.Format(&st, 'v')
		os.WriteFile("./foo/val.cue", st.Bytes(), fs.ModePerm)
		val = val.Unify(kindDef)
		st = cueFmtState{}
		val.Format(&st, 'v')
		os.WriteFile("./foo/val-unified.cue", st.Bytes(), fs.ModePerm)
		if val.Err() != nil {
			return nil, val.Err()
		}
		props := codegen.KindProperties{}
		val.Decode(&props)
		someKind := &codegen.AnyKind{
			Props:       props,
			AllVersions: make([]codegen.KindVersion, 0),
		}
		goVers := make(map[string]codegen.KindVersion)
		vers := val.LookupPath(cue.MakePath(cue.Str("versions")))
		vers.Decode(&goVers)
		for k, v := range goVers {
			v.Schema = val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("schema")))
			someKind.AllVersions = append(someKind.AllVersions, v)
		}
		kinds = append(kinds, someKind)
	}

	return kinds, nil
}

func (p *Parser) getKindDefinition() (cue.Value, error) {
	if p.kindDef != nil {
		return *p.kindDef, nil
	}

	kindOverlay := make(map[string]load.Source)
	err := ToOverlay(filepath.Join("/github.com/grafana/grafana-app-sdk/codegen/cuekind"), overlayFS, kindOverlay)
	if err != nil {
		return cue.Value{}, err
	}
	kindInstWithDef := load.Instances(nil, &load.Config{
		Overlay: kindOverlay,
	})[0]
	kindDef := cuecontext.New().BuildInstance(kindInstWithDef).LookupPath(cue.MakePath(cue.Str("Kind")))
	p.kindDef = &kindDef
	return *p.kindDef, nil
}

func (p *Parser) Parse2() error {
	overlay := make(map[string]load.Source)
	ToOverlay(filepath.Join("/github.com/grafana/grafana-app-sdk/codegen/cuekind"), overlayFS, overlay)
	fmt.Println(overlay)
	instWithDef := load.Instances(nil, &load.Config{
		Overlay: overlay,
	})[0]
	kindDef := cuecontext.New().BuildInstance(instWithDef).LookupPath(cue.MakePath(cue.Str("Kind")))
	//fmt.Println(kindDef)

	inst := load.Instances(nil, &load.Config{
		Dir: "./testing/cue",
	})
	if len(inst) < 0 {
		return fmt.Errorf("no data")
	}
	//fmt.Println(len(inst))
	root := cuecontext.New().BuildInstance(inst[0])
	//fmt.Println(root)
	val := root.LookupPath(cue.MakePath(cue.Str("myKind")))
	val = val.Unify(kindDef)
	if val.Err() != nil {
		fmt.Println(val.Err())
		panic(val.Err())
	}
	fmt.Println(val)
	props := codegen.KindProperties{}
	val.Decode(&props)
	someKind := codegen.AnyKind{
		Props:       props,
		AllVersions: make([]codegen.KindVersion, 0),
	}
	goVers := make(map[string]codegen.KindVersion)
	vers := val.LookupPath(cue.MakePath(cue.Str("versions")))
	fmt.Println(vers)
	vers.Decode(&goVers)
	fmt.Println(goVers)
	for k, v := range goVers {
		v.Schema = val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("schema")))
		someKind.AllVersions = append(someKind.AllVersions, v)
	}
	//someKind.Schema = val.LookupPath(cue.MakePath(cue.Str("schema")))
	fmt.Println(someKind)
	fmt.Println(someKind.AllVersions[0].Schema)
	/*def := cuecontext.New().CompileString(FullDef)
	val = val.Unify(def)
	fmt.Println(val.Err())*/
	return nil
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

// cueFmtState wraps a bytes.Buffer with the extra methods required to implement fmt.State.
// it will return false when queried about any flags.
type cueFmtState struct {
	bytes.Buffer
}

// Width returns the value of the width option and whether it has been set. It will always return 0, false.
func (*cueFmtState) Width() (wid int, ok bool) {
	return 0, false
}

// Precision returns the value of the precision option and whether it has been set. It will always return 0, false.
func (*cueFmtState) Precision() (prec int, ok bool) {
	return 0, false
}

// Flag returns whether the specified flag has been set. It will always return false.
func (*cueFmtState) Flag(flag int) bool {
	return flag == '#' || flag == '+'
}
