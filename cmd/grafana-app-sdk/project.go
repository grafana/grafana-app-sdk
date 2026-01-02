package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/grafana/codejen"
	"github.com/spf13/cobra"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
)

//go:embed templates/*.tmpl
var templates embed.FS

var projectCmd = &cobra.Command{
	Use: "project",
}

var projectInitCmd = &cobra.Command{
	Use:          "init",
	RunE:         projectInit,
	SilenceUsage: true,
}

var projectComponentCmd = &cobra.Command{
	Use: "component",
}

var projectAddComponentCmd = &cobra.Command{
	Use:          "add",
	RunE:         projectAddComponent,
	SilenceUsage: true,
}

var projectKindCmd = &cobra.Command{
	Use: "kind",
}

var projectAddKindCmd = &cobra.Command{
	Use:          "add",
	RunE:         projectAddKind,
	SilenceUsage: true,
}

var projectVersionCmd = &cobra.Command{
	Use: "version",
}

var projectVersionAddCmd = &cobra.Command{
	Use:          "add",
	RunE:         projectVersionAdd,
	SilenceUsage: true,
}

var projectLocalCmd = &cobra.Command{
	Use: "local",
}

var projectLocalGenerateCmd = &cobra.Command{
	Use:          "generate",
	RunE:         projectLocalEnvGenerate,
	SilenceUsage: true,
}

var projectLocalInitCmd = &cobra.Command{
	Use:          "init",
	RunE:         projectLocalEnvInit,
	SilenceUsage: true,
}

//nolint:goconst
func setupProjectCmd() {
	projectCmd.PersistentFlags().StringP("path", "p", "", "Path to project directory")
	projectCmd.PersistentFlags().Bool("overwrite", false, "Overwrite existing files instead of prompting")
	projectCmd.PersistentFlags().Lookup("overwrite").NoOptDefVal = "true"

	projectAddComponentCmd.Flags().String("grouping", kindGroupingKind, `Kind go package grouping.
Allowed values are 'group' and 'kind'. This should match the flag used in the 'generate' command`)

	projectLocalGenerateCmd.Flags().Bool("useoldmanifestkinds", false, "Whether to use the legacy manifest style of 'kinds' in the manifest, and 'versions' in each kind. This is a deprecated feature that will be removed in a future release.")
	projectLocalGenerateCmd.Flags().Lookup("useoldmanifestkinds").NoOptDefVal = "true"

	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectComponentCmd)
	projectCmd.AddCommand(projectKindCmd)
	projectCmd.AddCommand(projectLocalCmd)
	projectCmd.AddCommand(projectVersionCmd)

	projectComponentCmd.AddCommand(projectAddComponentCmd)
	projectKindCmd.AddCommand(projectAddKindCmd)
	projectVersionCmd.AddCommand(projectVersionAddCmd)

	projectLocalCmd.AddCommand(projectLocalInitCmd)
	projectLocalCmd.AddCommand(projectLocalGenerateCmd)
}

//nolint:revive,lll,funlen
func projectInit(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		fmt.Println("Usage: grafana-app-sdk project init [options] <module_name>")
		os.Exit(1)
	}

	name := args[0]

	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Schemas
	err = os.MkdirAll(filepath.Join(path, "kinds/cue.mod"), mkDirPerms)
	if err != nil {
		return err
	}

	// Init go
	name, err = projectWriteGoModule(path, name, overwrite)
	if err != nil {
		return err
	}

	// Init CUE
	cueModName := name
	// Check that the module's first path section is a domain. If it isn't, turn it into one
	if cueModSegments := strings.Split(cueModName, "/"); !strings.Contains(cueModSegments[0], ".") {
		cueModSegments[0] += ".grafana.app"
		cueModName = strings.Join(cueModSegments, "/")
	}
	cueModPath := filepath.Join(path, "kinds/cue.mod", "module.cue")
	cueModContents := []byte(fmt.Sprintf("module: \"%s/kinds\"\nlanguage: version: \"v0.8.2\"\n", cueModName))
	if _, err = os.Stat(cueModPath); err == nil && !overwrite {
		if promptYN(fmt.Sprintf("CUE module already exists at '%s', overwrite?", cueModPath), true) {
			err = writeFile(cueModPath, cueModContents)
		}
	} else {
		err = writeFile(cueModPath, cueModContents)
	}
	if err != nil {
		return err
	}

	// Init app manifest
	mtmpl, err := template.ParseFS(templates, "templates/manifest.cue.tmpl")
	if err != nil {
		return err
	}
	mbuf := bytes.Buffer{}
	appName := strings.Split(name, "/")[len(strings.Split(name, "/"))-1]
	err = mtmpl.Execute(&mbuf, map[string]any{
		"AppName": appName,
		"Group":   appName,
	})
	if err != nil {
		return err
	}
	err = writeFileWithOverwriteConfirm(filepath.Join(path, "kinds", "manifest.cue"), mbuf.Bytes())
	if err != nil {
		return err
	}

	// Initial empty project directory structure
	err = checkAndMakePath(filepath.Join(path, "pkg"))
	if err != nil {
		return err
	}
	err = checkAndMakePath(filepath.Join(path, "plugin"))
	if err != nil {
		return err
	}
	err = checkAndMakePath(filepath.Join(path, "cmd", "operator"))
	if err != nil {
		return err
	}
	modName := name
	if strings.LastIndex(modName, "/") > 0 {
		modName = name[strings.LastIndex(modName, "/")+1:]
	}

	// Create makefile
	makefileTmpl, err := template.ParseFS(templates, "templates/Makefile.tmpl")
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	err = makefileTmpl.Execute(buf, map[string]string{
		"ModuleName": modName,
	})
	if err != nil {
		return err
	}
	err = writeFileWithOverwriteConfirm(filepath.Join(path, "Makefile"), buf.Bytes())
	if err != nil {
		return err
	}
	return initializeLocalEnvFiles(path, modName, modName)
}

// projectWriteGoModule creates the go module if it doesn't exist (or prompt overwrite/merge if it does).
// Returns the module name (this may be different from the supplied moduleName if the go module already exists,
// and the user elects to use the existing name), and an error if an error occurred
//
//nolint:revive
func projectWriteGoModule(path, moduleName string, overwrite bool) (string, error) {
	goModPath := filepath.Join(path, "go.mod")
	goSumPath := filepath.Join(path, "go.sum")
	goModContents := []byte(fmt.Sprintf("module %s\n\ngo 1.22\n", moduleName))

	// If we weren't instructed to overwrite without prompting, let's check if the go.mod file already exists
	if _, err := os.Stat(goModPath); err == nil && !overwrite {
		// The go.mod file already exists, for convenience, let's check if the module listed matches
		mod, err := getGoModule(goModPath)
		if err != nil {
			if promptYN(fmt.Sprintf("Invalid go module file already exists at '%s', overwrite?", goModPath), true) {
				err = writeFile(goModPath, goModContents)
				if err != nil {
					return moduleName, err
				}
				err = writeFile(goSumPath, []byte("\n"))
				if err != nil {
					return moduleName, err
				}
			} else {
				fmt.Println("Not initializing go module")
			}
		} else if mod != moduleName {
			if promptYN(fmt.Sprintf("Go module already exists at '%s', with diffing module name '%s'. Use existing module name '%s'?", goModPath, mod, mod), true) {
				fmt.Printf("Using new module name '%s'.\n", mod)
				moduleName = mod
			} else {
				fmt.Printf("Continuing to use provided module name '%s'.\n", moduleName)
			}
			if promptYN("Do you want to overwrite the existing go.mod file?", false) {
				err = writeFile(goModPath, goModContents)
				if err != nil {
					return moduleName, err
				}
				err = writeFile(goSumPath, []byte("\n"))
				if err != nil {
					return moduleName, err
				}
			} else {
				err = exec.Command("go", "get", "github.com/grafana/grafana-app-sdk").Run()
				if err != nil {
					return moduleName, err
				}
			}
		}
	} else {
		err = writeFile(goModPath, goModContents)
		if err != nil {
			return moduleName, err
		}
		err = writeFile(goSumPath, []byte("\n"))
		if err != nil {
			return moduleName, err
		}
	}
	return moduleName, nil
}

//nolint:revive,funlen
func projectAddKind(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: grafana-app-sdk project kind add [options] <Human-Readable Kind Name>
	example:
		grafana-app-sdk project add kind "MyKind"`)
		os.Exit(1)
	}

	// Flag arguments
	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}

	// Source files path (optional)
	sourcePath, err := cmd.Flags().GetString(sourceFlag)
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Kind format
	format, err := cmd.Flags().GetString(formatFlag)
	if err != nil {
		return err
	}

	for _, kindName := range args {
		validName := regexp.MustCompile(`^([A-Z][a-zA-Z0-9]{0,61}[a-zA-Z0-9])$`)
		if !validName.MatchString(kindName) {
			return fmt.Errorf("name '%s' is invalid, must begin with a capital letter, and contain only alphanumeric characters", kindName)
		}

		pkg := "kinds"
		if len(sourcePath) > 0 {
			pkg = filepath.Base(sourcePath)
		}

		fieldName := strings.ToLower(kindName[0:1]) + kindName[1:]

		var files codejen.Files
		switch format {
		case FormatCUE:
			srcPath := filepath.Join(path, sourcePath)
			current, err := getManifestLatestVersion(srcPath)
			if err != nil {
				if err != errNoVersions {
					return err
				}
				current = "v1alpha1"
			}
			files, err = projectAddKindCUE(srcPath, "manifest.cue", fieldName, kindName, current, pkg)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown kind format '%s'", format)
		}

		for _, f := range files {
			if !overwrite {
				err = writeFileWithOverwriteConfirm(filepath.Join(path, sourcePath, f.RelativePath), f.Data)
			} else {
				err = writeFile(filepath.Join(path, sourcePath, f.RelativePath), f.Data)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

//nolint:revive
func projectVersionAdd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: grafana-app-sdk project version add [options] <version name>
	example:
		grafana-app-sdk project version add "v2alpha1"`)
		os.Exit(1)
	}

	// Flag arguments
	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}

	// Source files path (optional)
	sourcePath, err := cmd.Flags().GetString(sourceFlag)
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Kind format
	format, err := cmd.Flags().GetString(formatFlag)
	if err != nil {
		return err
	}

	current, err := getManifestLatestVersion(filepath.Join(path, sourcePath))
	if err != nil {
		return err
	}

	for _, versionName := range args {
		validName := regexp.MustCompile(`^v([0-9]+)((alpha|beta)[0-9]+)?$`)
		if !validName.MatchString(versionName) {
			return fmt.Errorf("name '%s' is invalid, version names should adhere to regex `v[0-9]+((alpha|beta)[0-9]+)?`", versionName)
		}

		pkg := "kinds"
		if len(sourcePath) > 0 {
			pkg = filepath.Base(sourcePath)
		}

		kinds, err := getManifestKindsForVersion(pkg, current)
		if err != nil {
			return err
		}

		for _, kind := range kinds {
			fieldName := strings.ToLower(kind[0:1]) + kind[1:]

			var files codejen.Files
			switch format {
			case FormatCUE:
				files, err = projectAddKindCUE(filepath.Join(path, sourcePath), "manifest.cue", fieldName, kind, versionName, pkg)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown kind format '%s'", format)
			}

			for _, f := range files {
				if !overwrite {
					err = writeFileWithOverwriteConfirm(filepath.Join(path, sourcePath, f.RelativePath), f.Data)
				} else {
					err = writeFile(filepath.Join(path, sourcePath, f.RelativePath), f.Data)
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func projectAddKindCUE(srcPath, manifestFileName, fieldName, kindName, version, pkg string) (codejen.Files, error) {
	kindTmpl, err := template.ParseFS(templates, "templates/kind.cue.tmpl")
	if err != nil {
		return nil, err
	}
	kindVersionTmpl, err := template.ParseFS(templates, "templates/kindversion.cue.tmpl")
	if err != nil {
		return nil, err
	}
	data := map[string]string{
		"FieldName": fieldName,
		"Name":      kindName,
		"Target":    "resource",
		"Package":   pkg,
		"Version":   version,
	}

	buf := &bytes.Buffer{}
	err = kindTmpl.Execute(buf, data)
	if err != nil {
		return nil, err
	}
	files := make(codejen.Files, 2)
	files[0] = codejen.File{
		RelativePath: fmt.Sprintf("%s.cue", strings.ToLower(kindName)),
		Data:         buf.Bytes(),
	}
	buf2 := &bytes.Buffer{}
	err = kindVersionTmpl.Execute(buf2, data)
	if err != nil {
		return nil, err
	}
	files[1] = codejen.File{
		RelativePath: fmt.Sprintf("%s_%s.cue", strings.ToLower(kindName), version),
		Data:         buf2.Bytes(),
	}

	mFiles, err := addVersionedKindToManifestBytesCUE(srcPath, manifestFileName, version, fieldName+version)
	if err != nil {
		return nil, err
	}
	files = append(files, mFiles...)
	return files, nil
}

//nolint:revive,funlen,gocyclo
func projectAddComponent(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: grafana-app-sdk project component add [options] <components>
	where <components> are one or more of:
		backend
		frontend
		operator`)
		os.Exit(1)
	}

	// Flag arguments
	// Source file path (optional)
	sourcePath, err := cmd.Flags().GetString(sourceFlag)
	if err != nil {
		return err
	}

	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}

	// Selector (optional)
	selector, err := cmd.Flags().GetString(selectorFlag)
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Kind format
	format, err := cmd.Flags().GetString(formatFlag)
	if err != nil {
		return err
	}

	genOperatorState, err := cmd.Flags().GetBool(genOperatorStateFlag)
	if err != nil {
		return err
	}

	kindGrouping, err := cmd.Flags().GetString("grouping")
	if err != nil {
		return err
	}
	if kindGrouping != kindGroupingGroup && kindGrouping != kindGroupingKind {
		return errors.New("--grouping must be one of 'group'|'kind'")
	}

	// Create the generator (used for generating non-static code)
	var generator any
	var manifestParser codegen.Parser[codegen.AppManifest]
	switch format {
	case FormatCUE:
		parser, err := cuekind.NewParser()
		if err != nil {
			return err
		}
		generator, err = codegen.NewGenerator[codegen.Kind](parser.KindParser(cuekind.ParseConfig{
			GenOperatorState: genOperatorState,
		}), os.DirFS(sourcePath))
		if err != nil {
			return err
		}
		manifestParser = parser.ManifestParser(cuekind.ParseConfig{
			GenOperatorState: genOperatorState,
		})
	default:
		return fmt.Errorf("unknown kind format '%s'", format)
	}

	manifests, err := manifestParser.Parse(os.DirFS(sourcePath), selector)
	if err != nil {
		return fmt.Errorf("error parsing manifest '%s': %v", sourcePath, err)
	}
	if len(manifests) == 0 {
		return fmt.Errorf("no manifest found in '%s'", sourcePath)
	}
	manifest := manifests[0]

	// Allow for multiple components to be added at once
	for _, component := range args {
		switch component {
		case "backend":
			switch format {
			case FormatCUE:
				err = addComponentBackend(path, generator.(*codegen.Generator[codegen.Kind]), []string{selector}, manifest.Properties().Group, kindGrouping == kindGroupingGroup)
			default:
				return fmt.Errorf("unknown kind format '%s'", format)
			}
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			}
		case "frontend":
			err = addComponentFrontend(path, manifest.Properties().Group)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			}
		case "operator":
			switch format {
			case FormatCUE:
				err = addComponentOperator(path, generator.(*codegen.Generator[codegen.Kind]), []string{selector}, kindGrouping == kindGroupingGroup, !overwrite)
			default:
				return fmt.Errorf("unknown kind format '%s'", format)
			}
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			}
		default:
			return fmt.Errorf("unknown component %s", component)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

type anyGenerator interface {
	*codegen.Generator[codegen.Kind]
}

//nolint:revive
func addComponentOperator[G anyGenerator](projectRootPath string, generator G, selectors []string, groupKinds bool, confirmOverwrite bool) error {
	// Get the repo from the go.mod file
	repo, err := getGoModule(filepath.Join(projectRootPath, "go.mod"))
	if err != nil {
		return err
	}
	var writeFileFunc = writeFile
	if confirmOverwrite {
		writeFileFunc = writeFileWithOverwriteConfirm
	}
	// Backwards-compatibility for manifests written to the base generated path
	manifestPath := "manifestdata"
	if m, _ := filepath.Glob(filepath.Join("pkg/generated", "*_manifest.go")); len(m) > 0 {
		manifestPath = ""
	}

	var files codejen.Files
	switch cast := any(generator).(type) {
	case *codegen.Generator[codegen.Kind]:
		files, err = cast.Generate(cuekind.OperatorGenerator(repo, "pkg/generated", groupKinds), selectors...)
		if err != nil {
			return err
		}
		appFiles, err := cast.Generate(cuekind.AppGenerator(repo, "pkg/generated", manifestPath, groupKinds), selectors...)
		if err != nil {
			return err
		}
		files = append(files, appFiles...)
	default:
		return fmt.Errorf("unknown generator type: %T", cast)
	}
	if err = checkAndMakePath("pkg"); err != nil {
		return err
	}
	for _, f := range files {
		err = writeFileFunc(filepath.Join(projectRootPath, f.RelativePath), f.Data)
		if err != nil {
			return err
		}
	}

	dockerfile, err := templates.ReadFile("templates/operator_Dockerfile.tmpl")
	if err != nil {
		return err
	}
	err = writeFileFunc(filepath.Join(projectRootPath, "cmd", "operator", "Dockerfile"), dockerfile)
	if err != nil {
		return err
	}
	return nil
}

//
// Backend plugin
//

// Linter doesn't like "Potential file inclusion via variable", which is actually desired here
//
//nolint:gosec
func addComponentBackend[G anyGenerator](projectRootPath string, generator G, selectors []string, manifestGroup string, groupKinds bool) error {
	// Check plugin ID
	if manifestGroup == "" {
		return errors.New("manifest group is required")
	}

	// Get the repo from the go.mod file
	repo, err := getGoModule(filepath.Join(projectRootPath, "go.mod"))
	if err != nil {
		return err
	}

	err = projectAddPluginAPI(generator, repo, filepath.Join(projectRootPath, "pkg/generated"), groupKinds, selectors)
	if err != nil {
		return err
	}

	// Magefile
	mg, _ := templates.ReadFile("templates/Magefile.go.tmpl")
	err = writeFile(filepath.Join(projectRootPath, "plugin/Magefile.go"), mg)
	if err != nil {
		return err
	}

	// Write or update the plugin.json
	pluginJSONPath := filepath.Join(projectRootPath, "plugin/src/plugin.json")
	if _, err = os.Stat(pluginJSONPath); err == nil {
		// Update plugin.json to include the executable name and backend bool
		m := make(map[string]any)
		b, _ := os.ReadFile(pluginJSONPath)
		err = json.Unmarshal(b, &m)
		if err != nil {
			return err
		}
		m["executable"] = fmt.Sprintf("gpx_%s-app", manifestGroup)
		m["backend"] = true
		b, _ = json.MarshalIndent(m, "", "  ")
		err = writeFile(pluginJSONPath, b)
	} else {
		// New plugin.json
		err = writePluginJSON(pluginJSONPath,
			fmt.Sprintf("%s-app", manifestGroup), "NAME", "AUTHOR", manifestGroup)
	}
	return err
}

//nolint:revive
func projectAddPluginAPI[G anyGenerator](generator G, repo, generatedAPIModelsPath string, groupKinds bool, selectors []string) error {
	var files codejen.Files
	var err error
	switch cast := any(generator).(type) {
	case *codegen.Generator[codegen.Kind]:
		files, err = cast.Generate(cuekind.BackendPluginGenerator(repo, generatedAPIModelsPath, groupKinds), selectors...)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown generator type: %T", cast)
	}
	if err = checkAndMakePath("pkg"); err != nil {
		return err
	}
	for _, f := range files {
		err = writeFile(filepath.Join("pkg", f.RelativePath), f.Data)
		if err != nil {
			return err
		}
	}
	return nil
}

// Frontend plugin
//
//nolint:revive
func addComponentFrontend(projectRootPath string, manifestGroup string) error {
	// Check plugin ID
	if manifestGroup == "" {
		return errors.New("manifest group is required")
	}

	if !isCommandInstalled("yarn") {
		return errors.New("yarn must be installed to add the frontend component")
	}

	args := []string{"create", "@grafana/plugin", "--pluginType=app", "--hasBackend=true", "--pluginName=tmp", "--orgName=tmp"}
	cmd := exec.Command("yarn", args...)
	buf := bytes.Buffer{}
	ebuf := bytes.Buffer{}
	cmd.Stdout = &buf
	cmd.Stderr = &ebuf
	err := cmd.Start()
	if err != nil {
		return err
	}
	fmt.Println("Creating plugin frontend using `\033[0;32myarn create @grafana/plugin\033[0m` (this may take a moment)...")
	err = cmd.Wait()
	if err != nil {
		// Only print command output on error
		fmt.Println(buf.String())
		fmt.Println(ebuf.String())
		return err
	}

	// Remove a few directories that get created which we don't actually want
	err = os.RemoveAll("./tmp-tmp-app/.github")
	if err != nil {
		return err
	}
	err = os.RemoveAll("./tmp-tmp-app/pkg")
	if err != nil {
		return err
	}
	err = os.Remove("./tmp-tmp-app/go.mod")
	if err != nil {
		return err
	}
	err = os.Remove("./tmp-tmp-app/go.sum")
	if err != nil {
		return err
	}
	// Move the remaining contents into /plugin
	err = moveFiles("./tmp-tmp-app/", filepath.Join(projectRootPath, "plugin"))
	if err != nil {
		return err
	}
	err = writePluginJSON(filepath.Join(projectRootPath, "plugin/src/plugin.json"),
		fmt.Sprintf("%s-app", manifestGroup), "NAME", "AUTHOR", manifestGroup)
	if err != nil {
		return err
	}
	return os.Remove("./tmp-tmp-app")
}

func moveFiles(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Just move directories wholesale by renaming
		if d.IsDir() {
			if path == srcDir {
				return nil
			}
			dst := filepath.Join(destDir, d.Name())
			if _, serr := os.Stat(dst); serr == nil {
				err := moveFiles(path, dst)
				if err != nil {
					return err
				}
				if err = os.Remove(path); err != nil {
					return err
				}
				return fs.SkipDir
			}
			err = os.Rename(path, filepath.Join(destDir, d.Name()))
			if err != nil {
				return err
			}
			return fs.SkipDir
		}

		return os.Rename(path, filepath.Join(destDir, d.Name()))
	})
}

func isCommandInstalled(command string) bool {
	cmd := exec.Command("which", command)
	err := cmd.Run()
	return err == nil
}

func writePluginJSON(fullPath, id, name, author, slug string) error {
	tmp, err := template.ParseFS(templates, "templates/plugin.json.tmpl")
	if err != nil {
		return err
	}
	data := struct {
		ID     string
		Name   string
		Author string
		Slug   string
	}{
		ID:     id,
		Name:   name,
		Author: author,
		Slug:   slug,
	}
	b := bytes.Buffer{}
	err = tmp.Execute(&b, data)
	if err != nil {
		return err
	}
	return writeFile(fullPath, b.Bytes())
}
