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

		// Jennies that need to be run post-file-write.
		// Post-processing is go-only (it reads the generated go types off disk), so it is skipped
		// entirely when go codegen is disabled, regardless of enableK8sPostProcessing.
		if cfg.Codegen.EnableK8sPostProcessing && cfg.Codegen.GoEnabled {
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

	// The go module path is only needed when go code is generated. A frontend-only project with
	// `codegen: goEnabled: false` may have no go.mod at all, so only resolve it if it's needed.
	goModule := cfg.Codegen.GoModule
	if goModule == "" && cfg.Codegen.GoEnabled {
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
	var resourceFiles codejen.Files
	if cfg.Codegen.GoEnabled {
		resourceFiles, err = generatorForManifest.Generate(cuekind.ResourceGenerator(goModule, goModGenPath, cfg.GroupKinds()), cfg.ManifestSelectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range resourceFiles {
			resourceFiles[i].RelativePath = filepath.Join(cfg.Codegen.GoGenPath, f.RelativePath)
		}
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

	// Manifest
	var goManifestFiles codejen.Files
	if cfg.Codegen.GoEnabled {
		// Backwards-compatibility for manifests written to the base generated path
		manifestPath := "manifestdata"
		if m, _ := filepath.Glob(filepath.Join(goModGenPath, "*_manifest.go")); len(m) > 0 {
			manifestPath = ""
		}

		manifestPkg := filepath.Base(manifestPath)
		if manifestPath == "" {
			manifestPkg = filepath.Base(goModGenPath)
		}

		goManifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGoGenerator(cuekind.ManifestGoGeneratorConfig{
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
	}

	// Manifest CRD
	var manifestFiles codejen.Files
	if cfg.Definitions.GenManifest {
		// One manifest is generated per manifest selector, and a fixed filename would make
		// every one of them collide on the same path, so reject the combination up-front.
		if cfg.Definitions.ManifestFileName != "" && len(cfg.ManifestSelectors) > 1 {
			return nil, fmt.Errorf("definitions.manifestFileName cannot be used with multiple "+
				"manifestSelectors (%d configured): all manifests would be written to %q",
				len(cfg.ManifestSelectors), cfg.Definitions.ManifestFileName)
		}

		manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(cuekind.ManifestGeneratorConfig{
			Extension:      cfg.Definitions.Encoding,
			FileName:       cfg.Definitions.ManifestFileName,
			IncludeSchemas: cfg.Definitions.ManifestSchemas,
			Version:        cfg.Definitions.ManifestVersion,
		}), cfg.ManifestSelectors...)
		if err != nil {
			return nil, err
		}
		seenManifestPaths := make(map[string]struct{}, len(manifestFiles))
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.Definitions.Path, f.RelativePath)
			if _, dup := seenManifestPaths[manifestFiles[i].RelativePath]; dup {
				return nil, fmt.Errorf("multiple app manifests would be written to the same file %q", manifestFiles[i].RelativePath)
			}
			seenManifestPaths[manifestFiles[i].RelativePath] = struct{}{}
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
