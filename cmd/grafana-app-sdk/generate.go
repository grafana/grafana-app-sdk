package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
	generateCmd.Flags().Bool("noschemasinmanifest", false, "Whether to exclude kind schemas from the generated app manifest. This flag exists to allow for codegen with recursive types in CUE until github.com/grafana/grafana-app-sdk/issues/460 is resolved.")
	generateCmd.Flags().Lookup("noschemasinmanifest").NoOptDefVal = "true"
	generateCmd.Flags().String("gomodule", "", `module name found in go.mod. If absent it will be inferred from ./go.mod`)
	generateCmd.Flags().String("gomodgenpath", "", `This argument is used as a relative path for generated go code from the go module root. It only needs to be present if gogenpath is an absolute path, or is not a relative path from the go module root.`)
	generateCmd.Flags().Bool("useoldmanifestkinds", false, "Whether to use the legacy manifest style of 'kinds' in the manifest, and 'versions' in each kind. This is a deprecated feature that will be removed in a future release.")
	generateCmd.Flags().Lookup("useoldmanifestkinds").NoOptDefVal = "true"
	generateCmd.Flags().Bool("crdmanifest", false, "Whether the generated manifest JSON/YAML has CRD-compatible schemas or the default OpenAPI documents. Use this flag to keep legacy behavior (CRD schemas in the manifest)")
	generateCmd.Flags().Lookup("crdmanifest").NoOptDefVal = "true"

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
	noSchemasInManifest, err := cmd.Flags().GetBool("noschemasinmanifest")
	if err != nil {
		return err
	}
	genOperatorState, err := cmd.Flags().GetBool(genOperatorStateFlag)
	if err != nil {
		return err
	}
	goModule, err := cmd.Flags().GetString("gomodule")
	if err != nil {
		return err
	}
	goModGenPath, err := cmd.Flags().GetString("gomodgenpath")
	if err != nil {
		return err
	}
	useOldManifestKinds, err := cmd.Flags().GetBool("useoldmanifestkinds")
	if err != nil {
		return err
	}
	crdCompatibleManifest, err := cmd.Flags().GetBool("crdmanifest")
	if err != nil {
		return err
	}

	if goModule == "" {
		goModule, err = getGoModule("go.mod")
		if err != nil {
			return fmt.Errorf("unable to load go module from ./go.mod: %w. Use --gomodule to set a value", err)
		}
	}

	if goModGenPath == "" {
		goModGenPath = goGenPath
	}

	// Backwards-compatibility for manifests written to the base generated path
	manifestPath := "manifestdata"
	if m, _ := filepath.Glob(filepath.Join(goModGenPath, "*_manifest.go")); len(m) > 0 {
		manifestPath = ""
	}
	var files codejen.Files
	switch format {
	case FormatCUE:
		files, err = generateKindsCue(os.DirFS(sourcePath), kindGenConfig{
			GoModuleName:           goModule,
			GoModuleGenBasePath:    goModGenPath,
			GoGenBasePath:          goGenPath,
			TSGenBasePath:          tsGenPath,
			CRDEncoding:            encType,
			CRDPath:                defPath,
			GroupKinds:             grouping == kindGroupingGroup,
			ManifestIncludeSchemas: !noSchemasInManifest,
			GenOperatorState:       genOperatorState,
			UseOldManifestKinds:    useOldManifestKinds,
			CRDCompatibleManifest:  crdCompatibleManifest,
			ManifestPath:           manifestPath,
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
				GoGenBasePath:       goGenPath,
				TSGenBasePath:       tsGenPath,
				CRDEncoding:         encType,
				CRDPath:             defPath,
				GroupKinds:          grouping == kindGroupingGroup,
				UseOldManifestKinds: useOldManifestKinds,
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
	GoModuleName           string
	GoModuleGenBasePath    string
	GoGenBasePath          string
	TSGenBasePath          string
	CRDEncoding            string
	CRDPath                string
	ManifestPath           string
	GroupKinds             bool
	ManifestIncludeSchemas bool
	GenOperatorState       bool
	UseOldManifestKinds    bool
	CRDCompatibleManifest  bool
}

//nolint:funlen,goconst
func generateKindsCue(modFS fs.FS, cfg kindGenConfig, selectors ...string) (codejen.Files, error) {
	parser, err := cuekind.NewParser()
	if err != nil {
		return nil, err
	}
	// Slightly hacky multiple generators as an intermediary while we move to a better system.
	// Both still source from a Manifest, but generatorForKinds supplies []Kind to jennies, vs AppManifest
	generatorForKinds, err := codegen.NewGenerator[codegen.Kind](parser.KindParser(cuekind.ParseConfig{
		GenOperatorState: cfg.GenOperatorState,
		UseOldKinds:      cfg.UseOldManifestKinds,
	}), modFS)
	if err != nil {
		return nil, err
	}
	generatorForManifest, err := codegen.NewGenerator[codegen.AppManifest](parser.ManifestParser(cuekind.ParseConfig{
		GenOperatorState: cfg.GenOperatorState,
		UseOldKinds:      cfg.UseOldManifestKinds,
	}), modFS)
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
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
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

	manifestPkg := filepath.Base(cfg.ManifestPath)
	if cfg.ManifestPath == "" {
		manifestPkg = filepath.Base(cfg.GoGenBasePath)
	}
	// Manifest
	goManifestFiles, err := generatorForManifest.Generate(cuekind.ManifestGoGenerator(manifestPkg, cfg.ManifestIncludeSchemas, cfg.GoModuleName, cfg.GoModuleGenBasePath, cfg.ManifestPath, cfg.GroupKinds), selectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range goManifestFiles {
		goManifestFiles[i].RelativePath = filepath.Join(cfg.GoGenBasePath, f.RelativePath)
	}

	// Manifest CRD
	var manifestFiles codejen.Files
	if cfg.CRDEncoding != "none" {
		encFunc := func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
		if cfg.CRDEncoding == "yaml" {
			encFunc = yaml.Marshal
		}

		manifestFiles, err = generatorForManifest.Generate(cuekind.ManifestGenerator(encFunc, cfg.CRDEncoding, cfg.ManifestIncludeSchemas, cfg.CRDCompatibleManifest), selectors...)
		if err != nil {
			return nil, err
		}
		for i, f := range manifestFiles {
			manifestFiles[i].RelativePath = filepath.Join(cfg.CRDPath, f.RelativePath)
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
	generator, err := codegen.NewGenerator[codegen.Kind](parser.KindParser(cuekind.ParseConfig{
		GenOperatorState: cfg.GenOperatorState,
		UseOldKinds:      cfg.UseOldManifestKinds,
	}), modFS)
	if err != nil {
		return nil, err
	}
	relativePath := cfg.GoGenBasePath
	if !cfg.GroupKinds {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.Generate(cuekind.PostResourceGenerationGenerator(repo, relativePath, cfg.GroupKinds), selectors...)
}
