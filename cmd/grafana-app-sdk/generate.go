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

const deprecationMessage = "this flag is no longer effective, please modify your manifest.cue and set config.%s instead"

//nolint:goconst
func setupGenerateCmd() {
	generateCmd.PersistentFlags().StringP("gogenpath", "g", "pkg/generated/",
		"Path to directory where generated go code will reside")
	_ = generateCmd.PersistentFlags().MarkDeprecated("gogenpath", fmt.Sprintf(deprecationMessage, "goGenPath"))

	generateCmd.PersistentFlags().StringP("tsgenpath", "t", "plugin/src/generated/",
		"Path to directory where generated TypeScript code will reside")
	_ = generateCmd.PersistentFlags().MarkDeprecated("tsgenpath", fmt.Sprintf(deprecationMessage, "tsGenPath"))

	generateCmd.Flags().String("defencoding", "json", `Encoding for Custom Resource Definition 
files. Allowed values are 'json', 'yaml', and 'none'. Use 'none' to turn off CRD generation.`)
	_ = generateCmd.Flags().MarkDeprecated("defencoding", fmt.Sprintf(deprecationMessage, "defEncoding"))

	generateCmd.Flags().String("defpath", "definitions", `Path where Custom Resource 
Definitions will be created. Only applicable if type=kubernetes`)
	_ = generateCmd.Flags().MarkDeprecated("defpath", fmt.Sprintf(deprecationMessage, "defPath"))

	generateCmd.Flags().String("grouping", kindGroupingKind, `Kind go package grouping.
Allowed values are 'group' and 'kind'. Dictates the packaging of go kinds, where 'group' places all kinds with the same group in the same package, and 'kind' creates separate packages per kind (packaging will always end with the version)`)
	_ = generateCmd.Flags().MarkDeprecated("grouping", fmt.Sprintf(deprecationMessage, "grouping"))

	generateCmd.Flags().Bool("postprocess", false, "Whether to run post-processing on the generated files after they are written to disk. Post-processing includes code generation based on +k8s comments on types. Post-processing will fail if the dependencies required by the generated code are absent from go.mod.")
	_ = generateCmd.Flags().MarkDeprecated("postprocess", fmt.Sprintf(deprecationMessage, "postProcess"))

	generateCmd.Flags().Bool("noschemasinmanifest", false, "Whether to exclude kind schemas from the generated app manifest. This flag exists to allow for codegen with recursive types in CUE until github.com/grafana/grafana-app-sdk/issues/460 is resolved.")
	_ = generateCmd.Flags().MarkDeprecated("noschemasinmanifest", fmt.Sprintf(deprecationMessage, "manifestIncludeSchemas"))

	generateCmd.Flags().String("gomodule", "", `module name found in go.mod. If absent it will be inferred from ./go.mod`)
	_ = generateCmd.Flags().MarkDeprecated("gomodule", fmt.Sprintf(deprecationMessage, "goModule"))

	generateCmd.Flags().String("gomodgenpath", "", `This argument is used as a relative path for generated go code from the go module root. It only needs to be present if gogenpath is an absolute path, or is not a relative path from the go module root.`)
	_ = generateCmd.Flags().MarkDeprecated("gomodgenpath", fmt.Sprintf(deprecationMessage, "goModGenPath"))

	generateCmd.Flags().Bool("useoldmanifestkinds", false, "Whether to use the legacy manifest style of 'kinds' in the manifest, and 'versions' in each kind. This is a deprecated feature that will be removed in a future release.")
	_ = generateCmd.Flags().MarkDeprecated("useoldmanifestkinds", fmt.Sprintf(deprecationMessage, "useOldManifestKinds"))

	generateCmd.Flags().Bool("crdmanifest", false, "Whether the generated manifest JSON/YAML has CRD-compatible schemas or the default OpenAPI documents. Use this flag to keep legacy behavior (CRD schemas in the manifest)")
	_ = generateCmd.Flags().MarkDeprecated("crdmanifest", fmt.Sprintf(deprecationMessage, "crdManifest"))

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

	// Selector (optional)
	selector, err := cmd.Flags().GetString(selectorFlag)
	if err != nil {
		return err
	}

	parser, err := cuekind.NewParser(os.DirFS(sourcePath))
	if err != nil {
		return err
	}

	cfg := parser.GetParsedConfig()

	if cfg.Grouping != kindGroupingGroup && cfg.Grouping != kindGroupingKind {
		return fmt.Errorf("--grouping must be one of 'group'|'kind'")
	}

	var files codejen.Files
	switch format {
	case FormatCUE:
		files, err = generateKindsCue(parser, selector)
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
	if cfg.PostProcess {
		if format == FormatCUE {
			files, err = postGenerateFilesCue(parser, selector)
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

//nolint:funlen,goconst
func generateKindsCue(parser *cuekind.Parser, selectors ...string) (codejen.Files, error) {
	// Slightly hacky multiple generators as an intermediary while we move to a better system.
	// Both still source from a Manifest, but generatorForKinds supplies []Kind to jennies, vs AppManifest
	generatorForKinds, err := codegen.NewGenerator(parser.KindParser())
	if err != nil {
		return nil, err
	}
	generatorForManifest, err := codegen.NewGenerator(parser.ManifestParser())
	if err != nil {
		return nil, err
	}

	cfg := parser.GetParsedConfig()

	// Resource
	resourceFiles, err := generatorForKinds.Generate(cuekind.ResourceGenerator(cfg.GroupKinds()), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(cfg.GoGenPath, f.RelativePath)
	}
	tsResourceFiles, err := generatorForKinds.Generate(cuekind.TypeScriptResourceGenerator(), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range tsResourceFiles {
		tsResourceFiles[i].RelativePath = filepath.Join(cfg.TsGenPath, f.RelativePath)
	}
	// CRD
	var crdFiles codejen.Files
	if cfg.DefEncoding != "none" {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.DefEncoding == "yaml" {
			encFunc = yaml.Marshal
		}
		crdFiles, err = generatorForKinds.Generate(cuekind.CRDGenerator(encFunc, cfg.DefEncoding), selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range crdFiles {
			crdFiles[i].RelativePath = filepath.Join(cfg.DefPath, f.RelativePath)
		}
	}

	manifestPath := "manifestdata"
	if m, _ := filepath.Glob(filepath.Join(cfg.GoModGenPath, "*_manifest.go")); len(m) > 0 {
		manifestPath = ""
	}

	manifestPkg := filepath.Base(manifestPath)
	if manifestPath == "" {
		manifestPkg = filepath.Base(cfg.GoModGenPath)
	}

	goModule := cfg.GoModule
	if goModule == "" {
		goModule, err = getGoModule("go.mod")
		if err != nil {
			return nil, fmt.Errorf("unable to load go module from ./go.mod: %w. Use --gomodule to set a value", err)
		}
	}

	goModGenPath := cfg.GoModGenPath
	if goModGenPath == "" {
		goModGenPath = cfg.GoGenPath
	}

	// Manifest
	goManifestFiles, err := generatorForManifest.Generate(cuekind.ManifestGoGenerator(manifestPkg, cfg.ManifestIncludeSchemas, goModule, goModGenPath, manifestPath, cfg.GroupKinds()), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range goManifestFiles {
		goManifestFiles[i].RelativePath = filepath.Join(cfg.GoGenPath, f.RelativePath)
	}

	// Manifest CRD
	var manifestFiles codejen.Files
	if cfg.DefEncoding != "none" {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.DefEncoding == "yaml" {
			encFunc = yaml.Marshal
		}

		manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(encFunc, cfg.DefEncoding, cfg.ManifestIncludeSchemas, cfg.CRDManifest), selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.DefPath, f.RelativePath)
		}
	}

	allFiles := append(make(codejen.Files, 0), resourceFiles...)
	allFiles = append(allFiles, tsResourceFiles...)
	allFiles = append(allFiles, crdFiles...)
	allFiles = append(allFiles, manifestFiles...)
	allFiles = append(allFiles, goManifestFiles...)
	return allFiles, nil
}

func postGenerateFilesCue(parser *cuekind.Parser, selectors ...string) (codejen.Files, error) {
	cfg := parser.GetParsedConfig()
	repo, err := getGoModule(cfg.GoGenPath)
	if err != nil {
		return nil, err
	}
	generator, err := codegen.NewGenerator[codegen.Kind](parser.KindParser())
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenPath
	if !cfg.GroupKinds() {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.Generate(cuekind.PostResourceGenerationGenerator(repo, relativePath, cfg.GroupKinds()), selectors...)
}
