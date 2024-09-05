package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/thema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
	themagen "github.com/grafana/grafana-app-sdk/codegen/thema"
	"github.com/grafana/grafana-app-sdk/kindsys"
)

const (
	targetResource    = "resource"
	targetModel       = "model"
	kindGroupingGroup = "group"
	kindGroupingKind  = "kind"
)

var generateCmd = &cobra.Command{
	Use:  "generate",
	RunE: generateCmdFunc,
}

func setupGenerateCmd() {
	generateCmd.PersistentFlags().StringP("gogenpath", "g", "pkg/generated/",
		"Path to directory where generated go code will reside")
	generateCmd.PersistentFlags().StringP("tsgenpath", "t", "plugin/src/generated/",
		"Path to directory where generated TypeScript code will reside")
	generateCmd.Flags().String("type", "kubernetes", `Storage layer type. 
Currently only allowed value is 'kubernetes', which will generate Custom Resource Definition files for each selector.`)
	generateCmd.Flags().String("crdencoding", "json", `Encoding for Custom Resource Definition 
files. Allowed values are 'json', 'yaml', and 'none'. Use 'none' to turn off CRD generation.`)
	generateCmd.Flags().String("crdpath", "definitions", `Path where Custom Resource 
Definitions will be created. Only applicable if type=kubernetes`)
	generateCmd.Flags().String("kindgrouping", kindGroupingKind, `Kind go package grouping.
Allowed values are 'group' and 'kind'. Dictates the packaging of go kinds, where 'group' places all kinds with the same group in the same package, and 'kind' creates separate packages per kind (packaging will always end with the version)`)
	generateCmd.Flags().Bool("postprocess", false, "Whether to run post-processing on the generated files after they are written to disk. Post-processing includes code generation based on +k8s comments on types. Post-processing will fail if the dependencies required by the generated code are absent from go.mod.")
	generateCmd.Flags().Lookup("postprocess").NoOptDefVal = "true"

	// Don't show "usage" information when an error is returned form the command,
	// because our errors are not command-usage-based
	generateCmd.SilenceUsage = true
}

//nolint:funlen
func generateCmdFunc(cmd *cobra.Command, _ []string) error {
	cuePath, err := cmd.Flags().GetString("cuepath")
	if err != nil {
		return err
	}

	goGenPath, err := cmd.Flags().GetString("gogenpath")
	if err != nil {
		return err
	}

	tsGenPath, err := cmd.Flags().GetString("tsgenpath")
	if err != nil {
		return err
	}

	selectors, err := cmd.Flags().GetStringSlice("selectors")
	if err != nil {
		return err
	}

	storageType, err := cmd.Flags().GetString("type")
	if err != nil {
		return err
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	encType, err := cmd.Flags().GetString("crdencoding")
	if err != nil {
		return err
	}

	crdPath, err := cmd.Flags().GetString("crdpath")
	if err != nil {
		return err
	}

	grouping, err := cmd.Flags().GetString("kindgrouping")
	if err != nil {
		return err
	}
	if grouping != kindGroupingGroup && grouping != kindGroupingKind {
		return fmt.Errorf("--kindgrouping must be one of 'group'|'kind'")
	}
	postProcess, err := cmd.Flags().GetBool("postprocess")
	if err != nil {
		return err
	}

	var files codejen.Files
	switch format {
	case FormatThema:
		files, err = generateKindsThema(os.DirFS(cuePath), kindGenConfig{
			GoGenBasePath: goGenPath,
			TSGenBasePath: tsGenPath,
			StorageType:   storageType,
			CRDEncoding:   encType,
			CRDPath:       crdPath,
		}, selectors...)
		if err != nil {
			return err
		}
	case FormatCUE:
		files, err = generateKindsCue(os.DirFS(cuePath), kindGenConfig{
			GoGenBasePath: goGenPath,
			TSGenBasePath: tsGenPath,
			StorageType:   storageType,
			CRDEncoding:   encType,
			CRDPath:       crdPath,
			GroupKinds:    grouping == kindGroupingGroup,
		}, selectors...)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown kind format '%s'", format)
	}

	for _, f := range files {
		err = writeFile(f.RelativePath, f.Data)
		if err != nil {
			return err
		}
	}

	// Jennies that need to be run post-file-write
	if postProcess {
		if format == FormatCUE {
			files, err = postGenerateFilesCue(os.DirFS(cuePath), kindGenConfig{
				GoGenBasePath: goGenPath,
				TSGenBasePath: tsGenPath,
				StorageType:   storageType,
				CRDEncoding:   encType,
				CRDPath:       crdPath,
				GroupKinds:    grouping == kindGroupingGroup,
			}, selectors...)
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
	}

	return nil
}

type kindGenConfig struct {
	GoGenBasePath string
	TSGenBasePath string
	StorageType   string
	CRDEncoding   string
	CRDPath       string
	GroupKinds    bool
}

func generateKindsThema(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	parser, err := themagen.NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
	if err != nil {
		return nil, err
	}

	// We want to run all the codegen before writing any files, to avoid partial generation in the event of errors
	allFiles := make(codejen.Files, 0)

	// Let's start by generating all back-end code
	// resource-target back-end code
	files, err := generateBackendResourcesThema(parser, filepath.Join(cfg.GoGenBasePath, targetResource), selectors)
	if err != nil {
		return nil, err
	}
	allFiles = append(allFiles, files...)

	// models-target back-end code
	files, err = generateBackendModelsThema(parser, filepath.Join(cfg.GoGenBasePath, targetModel+"s"), selectors)
	if err != nil {
		return nil, err
	}
	allFiles = append(allFiles, files...)

	// Front-end codegen
	files, err = generateFrontendModelsThema(parser, cfg.TSGenBasePath, selectors)
	if err != nil {
		return nil, err
	}
	allFiles = append(allFiles, files...)

	// Schema definition generation (CRD-only currently)
	if cfg.CRDEncoding != "none" {
		switch cfg.StorageType {
		case "kubernetes":
			files, err = generateCRDsThema(parser, cfg.CRDPath, cfg.CRDEncoding, selectors)
			if err != nil {
				return nil, err
			}
			allFiles = append(allFiles, files...)
		default:
			return nil, fmt.Errorf("unknown storage type '%s'", cfg.StorageType)
		}
	}
	return allFiles, nil
}

func generateBackendResourcesThema(parser *themagen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(themagen.Filter(themagen.ResourceGenerator(),
		func(c kindsys.Custom) bool {
			// Only run this generator on definitions with target="resource" and backend=true
			return c.Def().Properties.IsCRD && c.Def().Properties.Codegen.Backend
		}), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range files {
		files[i].RelativePath = filepath.Join(genPath, f.RelativePath)
	}
	return files, nil
}

func generateBackendModelsThema(parser *themagen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(themagen.Filter(themagen.ModelsGenerator(),
		func(c kindsys.Custom) bool {
			// Only run this generator on definitions with target="model" and backend=true
			return !c.Def().Properties.IsCRD && c.Def().Properties.Codegen.Backend
		}), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range files {
		files[i].RelativePath = filepath.Join(genPath, f.RelativePath)
	}
	return files, nil
}

func generateFrontendModelsThema(parser *themagen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(themagen.Filter(themagen.TypeScriptModelsGenerator(),
		func(c kindsys.Custom) bool {
			// Only run this generator on definitions with target="resource" and backend=true
			return c.Def().Properties.Codegen.Frontend
		}), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range files {
		files[i].RelativePath = filepath.Join(genPath, f.RelativePath)
	}
	return files, nil
}

func generateCRDsThema(parser *themagen.CustomKindParser, genPath string, encoding string, selectors []string) (codejen.Files, error) {
	var ms themagen.Generator
	if encoding == "yaml" {
		ms = themagen.CRDGenerator(yaml.Marshal, "yaml")
	} else {
		// Assume JSON
		ms = themagen.CRDGenerator(json.Marshal, "json")
	}
	files, err := parser.FilteredGenerate(themagen.Filter(ms, func(c kindsys.Custom) bool {
		return c.Def().Properties.IsCRD
	}), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range files {
		files[i].RelativePath = filepath.Join(genPath, f.RelativePath)
	}
	return files, nil
}

//nolint:funlen
func generateKindsCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	generator, err := codegen.NewGenerator[codegen.Kind](parser, modFS)
	if err != nil {
		return nil, err
	}
	// Resource
	resourceFiles, err := generator.FilteredGenerate(cuekind.ResourceGenerator(true, cfg.GroupKinds), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource != nil
	}, selectors...)
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenBasePath
	if !cfg.GroupKinds {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(relativePath, f.RelativePath)
	}
	// Model
	modelFiles, err := generator.FilteredGenerate(cuekind.ModelsGenerator(true, cfg.GroupKinds), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource == nil
	}, selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range modelFiles {
		modelFiles[i].RelativePath = filepath.Join(filepath.Join(cfg.GoGenBasePath, targetModel+"s"), f.RelativePath)
	}
	// TypeScript
	tsModelFiles, err := generator.FilteredGenerate(cuekind.TypeScriptModelsGenerator(true), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource == nil
	}, selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range tsModelFiles {
		tsModelFiles[i].RelativePath = filepath.Join(cfg.TSGenBasePath, f.RelativePath)
	}
	tsResourceFiles, err := generator.FilteredGenerate(cuekind.TypeScriptResourceGenerator(true), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource != nil
	}, selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range tsResourceFiles {
		tsResourceFiles[i].RelativePath = filepath.Join(cfg.TSGenBasePath, f.RelativePath)
	}
	// CRD
	var crdFiles codejen.Files
	if cfg.CRDEncoding != "none" {
		encFunc := json.Marshal
		if cfg.CRDEncoding == "yaml" {
			encFunc = yaml.Marshal
		}
		crdFiles, err = generator.FilteredGenerate(cuekind.CRDGenerator(encFunc, cfg.CRDEncoding), func(kind codegen.Kind) bool {
			return kind.Properties().APIResource != nil
		}, selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range crdFiles {
			crdFiles[i].RelativePath = filepath.Join(cfg.CRDPath, f.RelativePath)
		}
	}
	// Manifest
	var manifestFiles codejen.Files
	if cfg.CRDEncoding != "none" {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.CRDEncoding == "yaml" {
			encFunc = yaml.Marshal
		}
		manifestFiles, err = generator.FilteredGenerate(cuekind.ManifestGenerator(encFunc, cfg.CRDEncoding, ""), func(kind codegen.Kind) bool {
			return kind.Properties().APIResource != nil
		}, selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.CRDPath, f.RelativePath)
		}
	}

	goManifestFiles, err := generator.FilteredGenerate(cuekind.ManifestGoGenerator(filepath.Base(cfg.GoGenBasePath), ""), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource != nil
	})
	if err != nil {
		return nil, err
	}
	for i, f := range goManifestFiles {
		goManifestFiles[i].RelativePath = filepath.Join(cfg.GoGenBasePath, f.RelativePath)
	}

	allFiles := append(make(codejen.Files, 0), resourceFiles...)
	allFiles = append(allFiles, modelFiles...)
	allFiles = append(allFiles, tsModelFiles...)
	allFiles = append(allFiles, tsResourceFiles...)
	allFiles = append(allFiles, crdFiles...)
	allFiles = append(allFiles, manifestFiles...)
	allFiles = append(allFiles, goManifestFiles...)
	return allFiles, nil
}

func postGenerateFilesCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	// Get the repo from the go.mod file
	repo, err := getGoModule(filepath.Join("", "go.mod"))
	if err != nil {
		return nil, err
	}
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	generator, err := codegen.NewGenerator[codegen.Kind](parser, modFS)
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenBasePath
	if !cfg.GroupKinds {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.FilteredGenerate(cuekind.PostResourceGenerationGenerator(repo, relativePath, true, cfg.GroupKinds), func(kind codegen.Kind) bool {
		return kind.Properties().APIResource != nil
	}, selectors...)
}
