package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/grafana/codejen"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/config"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
)

const (
	targetResource = "resource"
)

var generateCmd = &cobra.Command{
	Use:  "generate",
	RunE: generateCmdFunc,
}

//nolint:goconst
func setupGenerateCmd() {
	// Don't show "usage" information when an error is returned form the command,
	// because our errors are not command-usage-based
	generateCmd.SilenceUsage = true
}

//nolint:funlen,revive,gocyclo
func generateCmdFunc(cmd *cobra.Command, _ []string) error {
	// Global flags
	sourcePath, err := cmd.Flags().GetString(sourceFlag)
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString(formatFlag)
	if err != nil {
		return err
	}
	configSelector, err := cmd.Flags().GetString(configFlag)
	if err != nil {
		return err
	}

	var genSrc any

	switch format {
	case FormatCUE:
		genSrc, err = cuekind.LoadCue(os.DirFS(sourcePath))
		if err != nil {
			return err
		}
	case FormatNone:
	default:
		return fmt.Errorf("unknown format '%s'", format)
	}

	// Load config
	cfg, err := config.Load(genSrc, configSelector)
	if err != nil {
		return err
	}

	switch v := genSrc.(type) {
	case *cuekind.Cue:
		parser, err := cuekind.NewParser(v,
			cfg.Codegen.EnableOperatorStatusGeneration,
		)
		if err != nil {
			return err
		}
		files, err := generateKindsCue(parser, cfg)
		if err != nil {
			return err
		}

		// Fail fast (before writing) if codegen produced Go that would not
		// compile due to duplicate top-level declarations. See grafana/grafana-app-sdk#1043.
		if err = validateGeneratedGoDecls(files); err != nil {
			return err
		}

		for _, f := range files {
			err = writeFile(f.RelativePath, f.Data)
			if err != nil {
				return err
			}
		}

		// Jennies that need to be run post-file-write
		if cfg.Codegen.EnableK8sPostProcessing {
			files, err = postGenerateFilesCue(parser, cfg)
			if err != nil {
				return err
			}
			for _, f := range files {
				err = writeFile(f.RelativePath, f.Data)
				if err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported source type '%T'", v)
	}

	return nil
}

//nolint:funlen,goconst
func generateKindsCue(parser *cuekind.Parser, cfg *config.Config) (codejen.Files, error) {
	generatorForManifest, err := codegen.NewGenerator(parser.ManifestParser())
	if err != nil {
		return nil, err
	}

	goModule := cfg.Codegen.GoModule
	if goModule == "" {
		goModule, err = getGoModule("go.mod")
		if err != nil {
			return nil, fmt.Errorf("unable to load go module from ./go.mod: %w. Set config.codegen.goModule with a value", err)
		}
	}

	goModGenPath := cfg.Codegen.GoModGenPath
	if goModGenPath == "" {
		goModGenPath = cfg.Codegen.GoGenPath
	}

	// Resource
	resourceFiles, err := generatorForManifest.Generate(cuekind.ResourceGenerator(goModule, goModGenPath, cfg.GroupKinds()), cfg.ManifestSelectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(cfg.Codegen.GoGenPath, f.RelativePath)
	}
	tsResourceFiles, err := generatorForManifest.Generate(cuekind.TypeScriptResourceGenerator(), cfg.ManifestSelectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range tsResourceFiles {
		tsResourceFiles[i].RelativePath = filepath.Join(cfg.Codegen.TsGenPath, f.RelativePath)
	}
	// CRD
	var crdFiles codejen.Files
	if cfg.Definitions.GenCRDs {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.Definitions.Encoding == "yaml" {
			encFunc = yaml.Marshal
		}
		crdFiles, err = generatorForManifest.Generate(cuekind.CRDGenerator(encFunc, cfg.Definitions.Encoding), cfg.ManifestSelectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range crdFiles {
			crdFiles[i].RelativePath = filepath.Join(cfg.Definitions.Path, f.RelativePath)
		}
	}

	// Backwards-compatibility for manifests written to the base generated path
	manifestPath := "manifestdata"
	if m, _ := filepath.Glob(filepath.Join(goModGenPath, "*_manifest.go")); len(m) > 0 {
		manifestPath = ""
	}

	manifestPkg := filepath.Base(manifestPath)
	if manifestPath == "" {
		manifestPkg = filepath.Base(goModGenPath)
	}

	// Manifest
	goManifestFiles, err := generatorForManifest.Generate(cuekind.ManifestGoGenerator(cuekind.ManifestGoGeneratorConfig{
		Package:            manifestPkg,
		IncludeSchemas:     cfg.Definitions.ManifestSchemas,
		ProjectRepo:        goModule,
		GoGenPath:          goModGenPath,
		ManifestGoFilePath: manifestPath,
		GroupKinds:         cfg.GroupKinds(),
	}), cfg.ManifestSelectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range goManifestFiles {
		goManifestFiles[i].RelativePath = filepath.Join(cfg.Codegen.GoGenPath, f.RelativePath)
	}

	// Manifest CRD
	var manifestFiles codejen.Files
	if cfg.Definitions.GenManifest {
		manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(
			cfg.Definitions.Encoding,
			cfg.Definitions.ManifestSchemas,
			cfg.Definitions.ManifestVersion),
			cfg.ManifestSelectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.Definitions.Path, f.RelativePath)
		}
	}

	allFiles := append(make(codejen.Files, 0), resourceFiles...)
	allFiles = append(allFiles, tsResourceFiles...)
	allFiles = append(allFiles, crdFiles...)
	allFiles = append(allFiles, manifestFiles...)
	allFiles = append(allFiles, goManifestFiles...)
	return allFiles, nil
}

// collidableDeclNames returns top-level names (type/func/const/var) sharing Go's
// package namespace. Blank "_" and init funcs (legal repeats) are excluded.
func collidableDeclNames(file *ast.File) []string {
	var names []string
	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl: // type / const / var
			for _, spec := range decl.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names = append(names, n.Name)
					}
				default:
				}
			}
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name.Name != "init" { // top-level func, not a method or init
				names = append(names, decl.Name.Name)
			}
		default:
		}
	}
	return slices.DeleteFunc(names, func(n string) bool { return n == "_" })
}

// validateGeneratedGoDecls errors if a generated Go package declares the same
// top-level name in two files ("redeclared in this block"). See grafana-app-sdk#1043.
func validateGeneratedGoDecls(files codejen.Files) error {
	type ref struct{ dir, name string }
	// (dir, name) -> files declaring it (with repeats); >=2 occurrences collide.
	decls := make(map[ref][]string)
	fset := token.NewFileSet()
	for _, f := range files {
		if !strings.HasSuffix(f.RelativePath, ".go") {
			continue
		}
		parsed, err := goparser.ParseFile(fset, f.RelativePath, f.Data, 0)
		if err != nil {
			return fmt.Errorf("generated file %s does not parse: %w", f.RelativePath, err)
		}
		dir, base := filepath.Dir(f.RelativePath), filepath.Base(f.RelativePath)
		for _, name := range collidableDeclNames(parsed) {
			r := ref{dir: dir, name: name}
			decls[r] = append(decls[r], base)
		}
	}

	var collisions []string
	for r, where := range decls {
		if len(where) < 2 {
			continue
		}
		slices.Sort(where)
		where = slices.Compact(where) // distinct files for the message
		collisions = append(collisions, fmt.Sprintf("  - %q redeclared in %s (%s)", r.name, r.dir, strings.Join(where, ", ")))
	}
	if len(collisions) == 0 {
		return nil
	}
	slices.Sort(collisions)
	return fmt.Errorf("generated Go will not compile (duplicate declarations); a schema field or definition is likely named after a generated type such as Spec, Status, or JSONCodec:\n%s", strings.Join(collisions, "\n"))
}

func postGenerateFilesCue(parser *cuekind.Parser, cfg *config.Config) (codejen.Files, error) {
	repo, err := getGoModule(cfg.Codegen.GoGenPath)
	if err != nil {
		return nil, err
	}
	generator, err := codegen.NewGenerator[codegen.AppManifest](parser.ManifestParser())
	if err != nil {
		return nil, err
	}
	relativePath := cfg.Codegen.GoGenPath
	if !cfg.GroupKinds() {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.Generate(cuekind.PostResourceGenerationGenerator(repo, relativePath, cfg.GroupKinds()), cfg.ManifestSelectors...)
}
