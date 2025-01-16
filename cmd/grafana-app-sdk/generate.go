package main

import (
	"bytes"
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
	generateCmd.Flags().String("defencoding", "json", `Encoding for Custom Resource Definition 
files. Allowed values are 'json', 'yaml', and 'none'. Use 'none' to turn off CRD generation.`)
	generateCmd.Flags().String("defpath", "definitions", `Path where Custom Resource 
Definitions will be created. Only applicable if type=kubernetes`)
	generateCmd.Flags().String("grouping", kindGroupingKind, `Kind go package grouping.
Allowed values are 'group' and 'kind'. Dictates the packaging of go kinds, where 'group' places all kinds with the same group in the same package, and 'kind' creates separate packages per kind (packaging will always end with the version)`)
	generateCmd.Flags().Bool("postprocess", false, "Whether to run post-processing on the generated files after they are written to disk. Post-processing includes code generation based on +k8s comments on types. Post-processing will fail if the dependencies required by the generated code are absent from go.mod.")
	generateCmd.Flags().Lookup("postprocess").NoOptDefVal = "true"

	// Don't show "usage" information when an error is returned form the command,
	// because our errors are not command-usage-based
	generateCmd.SilenceUsage = true
}

//nolint:funlen,revive
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
	selector, err := cmd.Flags().GetString(selectorFlag)
	if err != nil {
		return err
	}

	// command-specific flags
	goGenPath, err := cmd.Flags().GetString("gogenpath")
	if err != nil {
		return err
	}

	tsGenPath, err := cmd.Flags().GetString("tsgenpath")
	if err != nil {
		return err
	}

	encType, err := cmd.Flags().GetString("defencoding")
	if err != nil {
		return err
	}

	defPath, err := cmd.Flags().GetString("defpath")
	if err != nil {
		return err
	}

	grouping, err := cmd.Flags().GetString("grouping")
	if err != nil {
		return err
	}
	if grouping != kindGroupingGroup && grouping != kindGroupingKind {
		return fmt.Errorf("--grouping must be one of 'group'|'kind'")
	}
	postProcess, err := cmd.Flags().GetBool("postprocess")
	if err != nil {
		return err
	}

	var files codejen.Files
	switch format {
	case FormatCUE:
		files, err = generateKindsCue(os.DirFS(sourcePath), kindGenConfig{
			GoGenBasePath: goGenPath,
			TSGenBasePath: tsGenPath,
			CRDEncoding:   encType,
			CRDPath:       defPath,
			GroupKinds:    grouping == kindGroupingGroup,
		}, selector)
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
			files, err = postGenerateFilesCue(os.DirFS(sourcePath), kindGenConfig{
				GoGenBasePath: goGenPath,
				TSGenBasePath: tsGenPath,
				CRDEncoding:   encType,
				CRDPath:       defPath,
				GroupKinds:    grouping == kindGroupingGroup,
			}, selector)
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
	CRDEncoding   string
	CRDPath       string
	GroupKinds    bool
}

//nolint:funlen,goconst
func generateKindsCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
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
	resourceFiles, err := generatorForKinds.Generate(cuekind.ResourceGenerator(cfg.GroupKinds), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(cfg.GoGenBasePath, f.RelativePath)
	}
	tsResourceFiles, err := generatorForKinds.Generate(cuekind.TypeScriptResourceGenerator(), selectors...)
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
		crdFiles, err = generatorForKinds.Generate(cuekind.CRDGenerator(encFunc, cfg.CRDEncoding), selectors...)
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
	if cfg.CRDEncoding != "none" {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.CRDEncoding == "yaml" {
			encFunc = yaml.Marshal
		}
		manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(encFunc, cfg.CRDEncoding), selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.CRDPath, f.RelativePath)
		}

		goManifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGoGenerator(filepath.Base(cfg.GoGenBasePath)), selectors...)
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

//nolint:revive
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
	// Patch for kubernetes openAPI generation, which errors on some cases where map[string]any is used instead of map[string]interface{}
	// Update all the generated spec/status files on-disk to switch from map[string]any to map[string]interface{}
	// We don't use the modFS here because we can't write files to an fs.FS, only read them
	err = filepath.Walk(cfg.GoGenBasePath, func(path string, info os.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		if (len(info.Name()) < 15 || info.Name()[len(info.Name())-12:] != "_spec_gen.go") && (len(info.Name()) < 15 || info.Name()[len(info.Name())-14:] != "_status_gen.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Println("Updating ", path)
		updated := bytes.ReplaceAll(contents, []byte(`map[string]any`), []byte(`map[string]interface{}`))
		return writeFile(path, updated)
	})
	if err != nil {
		return nil, err
	}
	return generator.Generate(cuekind.PostResourceGenerationGenerator(repo, cfg.GoGenBasePath, cfg.GroupKinds), selectors...)
}
