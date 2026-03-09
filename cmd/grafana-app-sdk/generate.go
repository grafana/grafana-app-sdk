package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
			cfg.Kinds.PerKindVersion,
		)
		if err != nil {
			return err
		}
		files, err := generateKindsCue(parser, cfg)
		if err != nil {
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
	goManifestFiles, err := generatorForManifest.Generate(cuekind.ManifestGoGenerator(manifestPkg, cfg.Definitions.ManifestSchemas, goModule, goModGenPath, manifestPath, cfg.GroupKinds()), cfg.ManifestSelectors...)
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
