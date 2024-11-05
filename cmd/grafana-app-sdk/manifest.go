package main

import (
	"fmt"
	"io"
	"io/fs"
	"regexp"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
)

func loadManifestCUE(modFS fs.FS, selector string) (codegen.AppManifest, error) {
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	return parser.ParseManifest(modFS)
}

func addKindToManifestBytesCUE(manifestBytes []byte, kindFieldName string) ([]byte, error) {
	contents := string(manifestBytes)
	expr := regexp.MustCompile(`(?m)^(\s*kinds\s*:)(.*)$`)
	fmt.Println(expr.FindStringSubmatch(contents))
	matches := expr.FindStringSubmatch(contents)
	if len(matches) < 3 {
		return nil, fmt.Errorf("could not find kinds field in manifest.cue")
	}
	kindsStr := matches[2]
	if regexp.MustCompile(`^\s*\[`).MatchString(kindsStr) {
		// Direct array, we can prepend our field
		// Check if there's anything in the array
		if regexp.MustCompile(`^\s\[\s*]`).MatchString(kindsStr) {
			// Empty, just replace with our field
			contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"]")
		} else {
			kindsStr = regexp.MustCompile(`^\s*\[`).ReplaceAllString(kindsStr, " ["+kindFieldName+", ")
			contents = expr.ReplaceAllString(contents, matches[1]+kindsStr)
		}
	} else {
		// Not a simple list, prepend `[<fieldname>] + `
		contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"] + "+matches[2])
	}
	return []byte(contents), nil
}

func addKindToManifestCUE(modFS fs.FS, selector string, kindFieldName string) ([]byte, error) {
	// Rather than attempt to load and modify in-CUE (as this is complex and will also change the CUE the user has written)
	// We will just modify the file at <kindpath>/manifest.cue and stick kindFieldName at the beginning of the `kinds` array
	file, err := modFS.Open("manifest.cue")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contentsBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	contents := string(contentsBytes)
	expr := regexp.MustCompile(`(?m)^(\s*kinds\s*:)(.*)$`)
	fmt.Println(expr.FindStringSubmatch(contents))
	matches := expr.FindStringSubmatch(contents)
	if len(matches) < 3 {
		return nil, fmt.Errorf("could not find kinds field in manifest.cue")
	}
	kindsStr := matches[2]
	if regexp.MustCompile(`^\s*\[`).MatchString(kindsStr) {
		// Direct array, we can prepend our field
		// Check if there's anything in the array
		if regexp.MustCompile(`^\s\[\s*]`).MatchString(kindsStr) {
			// Empty, just replace with our field
			contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"]")
		} else {
			kindsStr = regexp.MustCompile(`^\s*\[`).ReplaceAllString(kindsStr, " ["+kindFieldName+", ")
			contents = expr.ReplaceAllString(contents, matches[1]+kindsStr)
		}
	} else {
		// Not a simple list, prepend `[<fieldname>] + `
		contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"] + "+matches[2])
	}
	return []byte(contents), nil
	// This really depends on the manifest file being the one we gen'd

	/*modFile, err := modFS.Open("cue.mod/module.cue")
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
	err = cuekind.ToOverlay(filepath.Join("/", modPath), modFS, overlay)
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
	var val cue.Value = root
	if selector != "" {
		val = root.LookupPath(cue.MakePath(cue.Str(selector)))
	}

	val = val.FillPath(cue.MakePath(cue.Str("kinds")), load.FromBytes([]byte("["+kindFieldName+"]")))*/
	insts := load.Instances([]string{"./kinds"}, nil)
	ctx := cuecontext.New()
	v := ctx.BuildInstance(insts[0])
	val := v.LookupPath(cue.MakePath(cue.Str("manifest")))

	ctx.CompileBytes([]byte("{kinds: [foo]}"))

	fmt.Println(val)

	//val = val.Unify(ctx.CompileBytes([]byte("{kinds: [\"foo\"]}")))
	// If we lookup the `kinds` path before we Format, it'll resolve the field imports.
	// Instead, print it and grab the kinds as-is, which will be as-written in the CUE file
	st := cueFmtState{}
	val.Format(&st, 'v')
	fmt.Println(string(st.Bytes()))
	return nil, fmt.Errorf("NO")
}
