package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/kindsys"
	"github.com/grafana/thema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/grafana/grafana-app-sdk/codegen"
)

const (
	targetResource = "resource"
	targetModel    = "model"
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
files. Allowed values are 'json' and 'yaml'. Only applicable if type=kubernetes.`)
	generateCmd.Flags().String("crdpath", "definitions", `Path where Custom Resource 
Definitions will be created. Only applicable if type=kubernetes`)

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

	parser, err := codegen.NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(cuePath))
	if err != nil {
		return err
	}

	// We want to run all the codegen before writing any files, to avoid partial generation in the event of errors
	allFiles := make(codejen.Files, 0)

	// Let's start by generating all back-end code
	// resource-target back-end code
	files, err := generateBackendResources(parser, filepath.Join(goGenPath, targetResource), selectors)
	if err != nil {
		return err
	}
	allFiles = append(allFiles, files...)

	// models-target back-end code
	files, err = generateBackendModels(parser, filepath.Join(goGenPath, targetModel+"s"), selectors)
	if err != nil {
		return err
	}
	allFiles = append(allFiles, files...)

	// Front-end codegen
	files, err = generateFrontendModels(parser, tsGenPath, selectors)
	if err != nil {
		return err
	}
	allFiles = append(allFiles, files...)

	// Schema definition generation (CRD-only currently)
	switch storageType {
	case "kubernetes":
		encType, err := cmd.Flags().GetString("crdencoding")
		if err != nil {
			return err
		}
		crdPath, err := cmd.Flags().GetString("crdpath")
		if err != nil {
			return err
		}
		files, err = generateCRDs(parser, crdPath, encType, selectors)
		if err != nil {
			return err
		}
		allFiles = append(allFiles, files...)
	default:
		return fmt.Errorf("unknown storage type '%s'", storageType)
	}

	for _, f := range allFiles {
		err = writeFile(f.RelativePath, f.Data)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateBackendResources(parser *codegen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(codegen.Filter(codegen.ResourceGenerator(),
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

func generateBackendModels(parser *codegen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(codegen.Filter(codegen.ModelsGenerator(),
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

func generateFrontendModels(parser *codegen.CustomKindParser, genPath string, selectors []string) (codejen.Files, error) {
	files, err := parser.FilteredGenerate(codegen.Filter(codegen.TypeScriptModelsGenerator(),
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

func generateCRDs(parser *codegen.CustomKindParser, genPath string, encoding string, selectors []string) (codejen.Files, error) {
	var ms codegen.Generator
	if encoding == "yaml" {
		ms = codegen.CRDGenerator(yaml.Marshal, "yaml")
	} else {
		// Assume JSON
		ms = codegen.CRDGenerator(json.Marshal, "json")
	}
	files, err := parser.FilteredGenerate(codegen.Filter(ms, func(c kindsys.Custom) bool {
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
