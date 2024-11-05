package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/grafana/codejen"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
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

//nolint:goconst
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
	generateCmd.Flags().Bool("nomanifest", false, "Whether to disable generating the app manifest")
	generateCmd.Flags().Lookup("nomanifest").NoOptDefVal = "true"
	generateCmd.Flags().Bool("notypeinpath", false, "Whether to remove the 'resource' or 'models' in the generated go path (this does nothing for --kindgrouping=group)")
	generateCmd.Flags().Lookup("notypeinpath").NoOptDefVal = "true"

	// Don't show "usage" information when an error is returned form the command,
	// because our errors are not command-usage-based
	generateCmd.SilenceUsage = true
}

//nolint:funlen,revive
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
	noManifest, err := cmd.Flags().GetBool("nomanifest")
	if err != nil {
		return err
	}
	noTypeInPath, err := cmd.Flags().GetBool("notypeinpath")
	if err != nil {
		return err
	}

	var files codejen.Files
	switch format {
	case FormatCUE:
		files, err = generateKindsCue(os.DirFS(cuePath), kindGenConfig{
			GoGenBasePath:      goGenPath,
			TSGenBasePath:      tsGenPath,
			StorageType:        storageType,
			CRDEncoding:        encType,
			CRDPath:            crdPath,
			GroupKinds:         grouping == kindGroupingGroup,
			GenerateManifest:   !noManifest,
			PrefixPathWithType: !noTypeInPath,
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
	GoGenBasePath      string
	TSGenBasePath      string
	StorageType        string
	CRDEncoding        string
	CRDPath            string
	GroupKinds         bool
	GenerateManifest   bool
	PrefixPathWithType bool
}

//nolint:funlen,goconst
func generateKindsCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	parser.ManifestSelector = "manifest"
	// Slightly hacky multiple generators as an intermediary while we move to a better system.
	// Both still source from a Manifest, but generatorForKinds supplies []Kind to jennies, vs AppManifest
	generatorForKinds, err := codegen.NewGenerator[codegen.Kind](parser.KindParser(true), modFS)
	if err != nil {
		return nil, err
	}
	generatorForManifest, err := codegen.NewGenerator[codegen.AppManifest](parser.ManifestParser(), modFS)
	if err != nil {
		return nil, err
	}
	// Resource
	resourceFiles, err := generatorForKinds.Generate(cuekind.ResourceGenerator(cfg.GroupKinds))
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenBasePath
	if !cfg.GroupKinds && cfg.PrefixPathWithType {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(relativePath, f.RelativePath)
	}
	tsResourceFiles, err := generatorForKinds.FilteredGenerate(cuekind.TypeScriptResourceGenerator(), func(kind codegen.Kind) bool {
		return true
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
		crdFiles, err = generatorForKinds.FilteredGenerate(cuekind.CRDGenerator(encFunc, cfg.CRDEncoding), func(kind codegen.Kind) bool {
			return true
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
	var goManifestFiles codejen.Files
	if cfg.GenerateManifest {
		if cfg.CRDEncoding != "none" {
			encFunc := func(v any) ([]byte, error) {
				return json.MarshalIndent(v, "", "    ")
			}
			if cfg.CRDEncoding == "yaml" {
				encFunc = yaml.Marshal
			}
			manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(encFunc, cfg.CRDEncoding, ""))
			if err != nil {
				return nil, err
			}
			for i, f := range manifestFiles {
				manifestFiles[i].RelativePath = filepath.Join(cfg.CRDPath, f.RelativePath)
			}
		}

		goManifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGoGenerator(filepath.Base(cfg.GoGenBasePath), ""))
		if err != nil {
			return nil, err
		}
		for i, f := range goManifestFiles {
			goManifestFiles[i].RelativePath = filepath.Join(cfg.GoGenBasePath, f.RelativePath)
		}
	}

	allFiles := append(make(codejen.Files, 0), resourceFiles...)
	allFiles = append(allFiles, tsResourceFiles...)
	allFiles = append(allFiles, crdFiles...)
	allFiles = append(allFiles, manifestFiles...)
	allFiles = append(allFiles, goManifestFiles...)
	return allFiles, nil
}

func postGenerateFilesCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	// Get the repo from the go.mod file
	repo, err := getGoModule(cfg.GoGenBasePath)
	if err != nil {
		return nil, err
	}
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	generator, err := codegen.NewGenerator[codegen.Kind](parser.KindParser(true), modFS)
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenBasePath
	if !cfg.GroupKinds {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.FilteredGenerate(cuekind.PostResourceGenerationGenerator(repo, relativePath, cfg.GroupKinds), func(kind codegen.Kind) bool {
		return true
	}, selectors...)
}
