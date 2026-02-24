package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grafana/codejen"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/config"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
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
	generateCmd.PersistentFlags().StringP("gogenpath", "g", "pkg/generated/",
		"Path to directory where generated go code will reside")
	_ = generateCmd.PersistentFlags().MarkDeprecated("gogenpath", fmt.Sprintf(deprecationMessage, "codegen.goGenPath"))

	generateCmd.PersistentFlags().StringP("tsgenpath", "t", "plugin/src/generated/",
		"Path to directory where generated TypeScript code will reside")
	_ = generateCmd.PersistentFlags().MarkDeprecated("tsgenpath", fmt.Sprintf(deprecationMessage, "codegen.tsGenPath"))

	generateCmd.Flags().String("defencoding", "json", `Encoding for Custom Resource Definition 
files. Allowed values are 'json', 'yaml', and 'none'. Use 'none' to turn off CRD generation.`)
	_ = generateCmd.Flags().MarkDeprecated("defencoding", fmt.Sprintf(deprecationMessage, "definitions.encoding"))

	generateCmd.Flags().String("defpath", "definitions", `Path where Custom Resource 
Definitions will be created. Only applicable if type=kubernetes`)
	_ = generateCmd.Flags().MarkDeprecated("defpath", fmt.Sprintf(deprecationMessage, "definitions.path"))

	generateCmd.Flags().String("grouping", config.KindGroupingKind, `Kind go package grouping.
Allowed values are 'group' and 'kind'. Dictates the packaging of go kinds, where 'group' places all kinds with the same group in the same package, and 'kind' creates separate packages per kind (packaging will always end with the version)`)
	_ = generateCmd.Flags().MarkDeprecated("grouping", fmt.Sprintf(deprecationMessage, "kinds.grouping"))

	generateCmd.Flags().Bool("postprocess", false, "Whether to run post-processing on the generated files after they are written to disk. Post-processing includes code generation based on +k8s comments on types. Post-processing will fail if the dependencies required by the generated code are absent from go.mod.")
	generateCmd.Flags().Lookup("postprocess").NoOptDefVal = "true"
	_ = generateCmd.Flags().MarkDeprecated("postprocess", fmt.Sprintf(deprecationMessage, "codegen.enableK8sPostProcessing"))

	generateCmd.Flags().Bool("noschemasinmanifest", false, "Whether to exclude kind schemas from the generated app manifest. This flag exists to allow for codegen with recursive types in CUE until github.com/grafana/grafana-app-sdk/issues/460 is resolved.")
	generateCmd.Flags().Lookup("noschemasinmanifest").NoOptDefVal = "true"
	_ = generateCmd.Flags().MarkDeprecated("noschemasinmanifest", fmt.Sprintf(deprecationMessage, "definitions.manifestSchemas"))

	generateCmd.Flags().String("gomodule", "", `module name found in go.mod. If absent it will be inferred from ./go.mod`)
	_ = generateCmd.Flags().MarkDeprecated("gomodule", fmt.Sprintf(deprecationMessage, "codegen.goModule"))

	generateCmd.Flags().String("gomodgenpath", "", `This argument is used as a relative path for generated go code from the go module root. It only needs to be present if gogenpath is an absolute path, or is not a relative path from the go module root.`)
	_ = generateCmd.Flags().MarkDeprecated("gomodgenpath", fmt.Sprintf(deprecationMessage, "codegen.goModGenPath"))

	generateCmd.Flags().Bool("useoldmanifestkinds", false, "Whether to use the legacy manifest style of 'kinds' in the manifest, and 'versions' in each kind. This is a deprecated feature that will be removed in a future release.")
	generateCmd.Flags().Lookup("useoldmanifestkinds").NoOptDefVal = "true"
	_ = generateCmd.Flags().MarkDeprecated("useoldmanifestkinds", fmt.Sprintf(deprecationMessage, "kinds.perKindVersion"))

	generateCmd.Flags().Bool("crdmanifest", false, "Whether the generated manifest JSON/YAML has CRD-compatible schemas or the default OpenAPI documents. Use this flag to keep legacy behavior (CRD schemas in the manifest)")
	generateCmd.Flags().Lookup("crdmanifest").NoOptDefVal = "true"
	_ = generateCmd.Flags().MarkDeprecated("crdmanifest", fmt.Sprintf(deprecationMessage, "definitions.manfiestVersion"))

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
	manifestSelector, err := cmd.Flags().GetString(selectorFlag)
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

	// HACK: Use flags for a base config for backwards-compatibility
	baseConfig := &config.Config{
		Codegen: &config.CodegenConfig{
			GoModule:                       goModule,
			GoModGenPath:                   goModGenPath,
			GoGenPath:                      goGenPath,
			TsGenPath:                      tsGenPath,
			EnableK8sPostProcessing:        postProcess,
			EnableOperatorStatusGeneration: genOperatorState,
		},
		Definitions: &config.DefinitionsConfig{
			GenManifest:     encType != "none",
			GenCRDs:         encType != "none",
			ManifestSchemas: !noSchemasInManifest,
			Encoding:        encType,
			Path:            defPath,
			ManifestVersion: jennies.VersionV1Alpha2,
		},
		Kinds: &config.KindsConfig{
			Grouping:       grouping,
			PerKindVersion: useOldManifestKinds,
		},
		ManifestSelectors: []string{manifestSelector},
	}

	if crdCompatibleManifest {
		baseConfig.Definitions.ManifestVersion = jennies.VersionV1Alpha1
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
	cfg, err := config.Load(genSrc, configSelector, baseConfig)
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

		if !cfg.Codegen.SkipPreflightCompilationCheck {
			if err := preflightGeneratedGoCodeCompiles(cfg, files); err != nil {
				return err
			}
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
			if !cfg.Codegen.SkipPreflightCompilationCheck {
				if err := preflightGeneratedGoCodeCompiles(cfg, files); err != nil {
					return err
				}
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

func preflightGeneratedGoCodeCompiles(cfg *config.Config, files codejen.Files) error {
	if cfg == nil {
		return preflightGeneratedGoCodeCompilesWithOverlay(files)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	currentModule, _ := getGoModule("go.mod")
	goModule := cfg.Codegen.GoModule
	if goModule == "" {
		goModule = currentModule
	}
	if goModule == "" {
		return preflightGeneratedGoCodeCompilesWithOverlay(files)
	}
	if currentModule != "" && goModule == currentModule {
		return preflightGeneratedGoCodeCompilesWithOverlay(files)
	}

	goGenRoot := cfg.Codegen.GoGenPath
	if goGenRoot == "" {
		goGenRoot = "."
	}
	if !filepath.IsAbs(goGenRoot) {
		goGenRoot = filepath.Join(cwd, goGenRoot)
	}
	goGenRoot = filepath.Clean(goGenRoot)

	moduleGenRoot := cfg.Codegen.GoModGenPath
	if moduleGenRoot == "" {
		moduleGenRoot = cfg.Codegen.GoGenPath
	}
	if moduleGenRoot == "" {
		moduleGenRoot = "."
	}
	if filepath.IsAbs(moduleGenRoot) {
		return preflightGeneratedGoCodeCompilesWithOverlay(files)
	}
	moduleGenRoot = filepath.Clean(moduleGenRoot)

	type generatedGoFile struct {
		absDir  string
		relPath string
		data    []byte
	}
	generatedFiles := make([]generatedGoFile, 0, len(files))
	generatedPackageDirs := make(map[string]struct{})
	manifestFileCountByDir := make(map[string]int)

	for _, f := range files {
		if filepath.Ext(f.RelativePath) != ".go" {
			continue
		}

		absTargetPath := f.RelativePath
		if !filepath.IsAbs(absTargetPath) {
			absTargetPath = filepath.Join(cwd, absTargetPath)
		}
		absTargetPath = filepath.Clean(absTargetPath)

		relPath, err := filepath.Rel(goGenRoot, absTargetPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return preflightGeneratedGoCodeCompilesWithOverlay(files)
		}

		generatedFiles = append(generatedFiles, generatedGoFile{
			absDir:  filepath.Dir(absTargetPath),
			relPath: filepath.Join(moduleGenRoot, relPath),
			data:    f.Data,
		})
		generatedPackageDirs[filepath.Dir(absTargetPath)] = struct{}{}
		if strings.HasSuffix(absTargetPath, "_manifest.go") {
			manifestFileCountByDir[filepath.Dir(absTargetPath)]++
		}
	}

	if len(generatedFiles) == 0 {
		return nil
	}

	skipPackages := make(map[string]struct{})
	for dir, count := range manifestFileCountByDir {
		// Multiple generated manifest files share package-level identifiers by design.
		// Skip those packages in preflight and continue validating generated resource code.
		if count > 1 {
			skipPackages[dir] = struct{}{}
		}
	}
	if len(skipPackages) > 0 {
		filtered := generatedFiles[:0]
		for _, f := range generatedFiles {
			if _, skip := skipPackages[f.absDir]; skip {
				continue
			}
			filtered = append(filtered, f)
		}
		generatedFiles = filtered
		for dir := range skipPackages {
			delete(generatedPackageDirs, dir)
		}
		if len(generatedFiles) == 0 {
			return nil
		}
	}

	tempDir, err := os.MkdirTemp("", "grafana-app-sdk-generate-preflight-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	moduleRoot := filepath.Join(tempDir, "module")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		return err
	}

	// Copy existing on-disk files from generated package directories, then overwrite with in-memory generated files.
	for packageDir := range generatedPackageDirs {
		relDir, err := filepath.Rel(goGenRoot, packageDir)
		if err != nil || strings.HasPrefix(relDir, "..") {
			return preflightGeneratedGoCodeCompilesWithOverlay(files)
		}
		tempPackageDir := filepath.Join(moduleRoot, moduleGenRoot, relDir)
		if err := os.MkdirAll(tempPackageDir, 0o755); err != nil {
			return err
		}

		entries, err := os.ReadDir(packageDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
				continue
			}
			src := filepath.Join(packageDir, entry.Name())
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			dst := filepath.Join(tempPackageDir, entry.Name())
			if err := os.WriteFile(dst, data, 0o600); err != nil {
				return err
			}
		}
	}

	for _, f := range generatedFiles {
		tempFilePath := filepath.Join(moduleRoot, f.relPath)
		if err := os.MkdirAll(filepath.Dir(tempFilePath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(tempFilePath, f.data, 0o600); err != nil {
			return err
		}
	}

	goModContents := fmt.Sprintf("module %s\n\ngo 1.24.0\n", goModule)
	if currentModule != "" && currentModule != goModule {
		goModContents += fmt.Sprintf("\nrequire %s v0.0.0\nreplace %s => %s\n", currentModule, currentModule, cwd)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte(goModContents), 0o600); err != nil {
		return err
	}

	buildCmd := exec.Command("go", "build", "-mod=mod", "./...")
	buildCmd.Dir = moduleRoot
	buildCmd.Env = append(os.Environ(),
		"GOSUMDB=off",
		fmt.Sprintf("GOCACHE=%s", filepath.Join(tempDir, "gocache")),
	)
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generated code contains compilation errors, this is likely a bug in sdk. If you'd like to bypass the compilation check, please set skipPreflightCompilationCheck to true.\n\n%s\n%w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func preflightGeneratedGoCodeCompilesWithOverlay(files codejen.Files) error {
	overlay := goBuildOverlay{
		Replace: make(map[string]string),
	}
	generatedPackages := make(map[string]struct{})
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "grafana-app-sdk-generate-preflight-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fileIndex := 0
	for _, f := range files {
		if filepath.Ext(f.RelativePath) != ".go" {
			continue
		}

		absTargetPath := f.RelativePath
		if !filepath.IsAbs(absTargetPath) {
			absTargetPath = filepath.Join(cwd, absTargetPath)
		}
		absTargetPath = filepath.Clean(absTargetPath)

		generatedPackages[filepath.Dir(absTargetPath)] = struct{}{}

		tempFilePath := filepath.Join(tempDir, fmt.Sprintf("file-%06d.go", fileIndex))
		fileIndex++

		if err := os.WriteFile(tempFilePath, f.Data, 0o600); err != nil {
			return err
		}
		overlay.Replace[absTargetPath] = tempFilePath
	}

	if len(generatedPackages) == 0 {
		return nil
	}

	packages := make([]string, 0, len(generatedPackages))
	for pkg := range generatedPackages {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	overlayBytes, err := json.Marshal(overlay)
	if err != nil {
		return err
	}
	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayBytes, 0o600); err != nil {
		return err
	}

	buildArgs := append([]string{"build", "-overlay", overlayPath}, packages...)
	buildCmd := exec.Command("go", buildArgs...)
	buildCmd.Env = append(os.Environ(),
		"GOSUMDB=off",
		fmt.Sprintf("GOCACHE=%s", filepath.Join(tempDir, "gocache")),
	)
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generated code contains compilation errors, this is likely a bug in sdk. If you'd like to bypass the compilation check, please set skipPreflightCompilationCheck to true.\n\n%s\n%w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

type goBuildOverlay struct {
	Replace map[string]string `json:"Replace"`
}

//nolint:funlen,goconst
func generateKindsCue(parser *cuekind.Parser, cfg *config.Config) (codejen.Files, error) {
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
	resourceFiles, err := generatorForKinds.Generate(cuekind.ResourceGenerator(goModule, goModGenPath, cfg.GroupKinds()), cfg.ManifestSelectors...)
	if err != nil {
		return nil, err
	}
	for i, f := range resourceFiles {
		resourceFiles[i].RelativePath = filepath.Join(cfg.Codegen.GoGenPath, f.RelativePath)
	}
	tsResourceFiles, err := generatorForKinds.Generate(cuekind.TypeScriptResourceGenerator(), cfg.ManifestSelectors...)
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
		crdFiles, err = generatorForKinds.Generate(cuekind.CRDGenerator(encFunc, cfg.Definitions.Encoding), cfg.ManifestSelectors...)
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
	generator, err := codegen.NewGenerator[codegen.Kind](parser.KindParser())
	if err != nil {
		return nil, err
	}
	relativePath := cfg.Codegen.GoGenPath
	if !cfg.GroupKinds() {
		relativePath = filepath.Join(relativePath, targetResource)
	}
	return generator.Generate(cuekind.PostResourceGenerationGenerator(repo, relativePath, cfg.GroupKinds()), cfg.ManifestSelectors...)
}
