package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/kindsys"
	"github.com/grafana/thema"
	"github.com/spf13/cobra"

	"github.com/grafana/grafana-app-sdk/codegen"
)

//go:embed templates/*.tmpl
var templates embed.FS

//go:embed templates/frontend-static/*
var frontEndStaticFiles embed.FS

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

func setupProjectCmd() {
	projectCmd.PersistentFlags().StringP("path", "p", "", "Path to project directory")
	projectCmd.PersistentFlags().Bool("overwrite", false, "Overwrite existing files instead of prompting")
	projectCmd.PersistentFlags().Lookup("overwrite").NoOptDefVal = "true"

	projectAddComponentCmd.Flags().String("plugin-id", "", "Plugin ID")
	projectAddKindCmd.Flags().String("type", "resource", "Kind codegen type. 'resource' or 'model'")
	projectAddKindCmd.Flags().String("plugin-id", "", "Plugin ID")

	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectComponentCmd)
	projectCmd.AddCommand(projectKindCmd)
	projectCmd.AddCommand(projectLocalCmd)

	projectComponentCmd.AddCommand(projectAddComponentCmd)
	projectKindCmd.AddCommand(projectAddKindCmd)

	projectLocalCmd.AddCommand(projectLocalInitCmd)
	projectLocalCmd.AddCommand(projectLocalGenerateCmd)
}

var validNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9\_\-]+[A-Za-z0-9]$`)

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
	err = os.MkdirAll(filepath.Join(path, "schemas/cue.mod"), mkDirPerms)
	if err != nil {
		return err
	}

	// Init go
	name, err = projectWriteGoModule(path, name, overwrite)
	if err != nil {
		return err
	}

	// Init CUE
	cueModPath := filepath.Join(path, "kinds/cue.mod", "module.cue")
	cueModContents := []byte(fmt.Sprintf("module: \"%s/kinds\"\n", name))
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
	goModContents := []byte(fmt.Sprintf("module %s\n\ngo 1.20\n", moduleName))

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

type simplePluginJSON struct {
	ID string `json:"id"`
}

//nolint:revive,funlen
func projectAddKind(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: grafana-app-sdk project add kind [options] <Human-Readable Kind Name>
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

	// Cue path (optional)
	cuePath, err := cmd.Flags().GetString("cuepath")
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Target
	target, err := cmd.Flags().GetString("type")
	if err != nil {
		return err
	}
	if target != "resource" && target != "model" {
		return fmt.Errorf("type must be one of 'resource' | 'model'")
	}

	pluginID, err := cmd.Flags().GetString("plugin-id")
	if err != nil {
		return err
	}
	if pluginID == "" {
		// Try to load the plugin ID from plugin/src/plugin.json
		pluginJSONPath := filepath.Join(path, "plugin", "src", "plugin.json")
		if _, err := os.Stat(pluginJSONPath); err != nil {
			return fmt.Errorf("--plugin-id is required if plugin/src/plugin.json is not present")
		}
		contents, err := os.ReadFile(pluginJSONPath)
		if err != nil {
			return fmt.Errorf("could not read plugin/src/plugin.json: %w", err)
		}
		spj := simplePluginJSON{}
		err = json.Unmarshal(contents, &spj)
		if err != nil {
			return fmt.Errorf("could not parse plugin.json: %w", err)
		}
		pluginID = spj.ID
	}

	for _, kindName := range args {
		validName := regexp.MustCompile(`^([A-Z][a-zA-Z0-9]{0,61}[a-zA-Z0-9])$`)
		if !validName.MatchString(kindName) {
			return fmt.Errorf("name '%s' is invalid, must begin with a capital letter, and contain only alphanumeric characters", kindName)
		}

		pkg := "kinds"
		if len(cuePath) > 0 {
			pkg = filepath.Base(cuePath)
		}

		kindTmpl, err := template.ParseFS(templates, "templates/kind.cue.tmpl")
		if err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		err = kindTmpl.Execute(buf, map[string]string{
			"FieldName": strings.ToLower(kindName[0:1]) + kindName[1:],
			"Name":      kindName,
			"Target":    target,
			"Package":   pkg,
			"PluginID":  pluginID,
		})
		if err != nil {
			return err
		}
		kindPath := filepath.Join(path, cuePath, fmt.Sprintf("%s.cue", strings.ToLower(kindName)))
		if !overwrite {
			err = writeFileWithOverwriteConfirm(kindPath, buf.Bytes())
		} else {
			err = writeFile(kindPath, buf.Bytes())
		}
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:revive,funlen
func projectAddComponent(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: grafana-app-sdk project add component [options] <components>
	where <components> are one or more of:
		backend
		frontend
		operator`)
		os.Exit(1)
	}

	// Flag arguments
	// Cue path (optional)
	cuePath, err := cmd.Flags().GetString("cuepath")
	if err != nil {
		return err
	}

	// Path (optional)
	path, err := cmd.Flags().GetString("path")
	if err != nil {
		return err
	}

	// Selectors (optional)
	selectors, err := cmd.Flags().GetStringSlice("selectors")
	if err != nil {
		return err
	}

	// Default overwrite
	overwrite, err := cmd.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}

	// Plugin ID (optional depending on component)
	pluginID, err := cmd.Flags().GetString("plugin-id")
	if err != nil {
		return err
	}
	if len(pluginID) > 0 && !validNameRegex.MatchString(pluginID) {
		fmt.Printf("plugin-id '%s' is not valid. Name must begin and end with an alphanumeric character, "+
			"and only contain alphanumeric characters and _ or -", pluginID)
		os.Exit(1)
	}

	// Create the generator (used for generating non-static code)
	generator, err := codegen.NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(cuePath))
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}

	// Allow for multiple components to be added at once
	for _, component := range args {
		switch component {
		case "backend":
			err = addComponentBackend(path, generator, selectors, pluginID)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			}
		case "frontend":
			err = addComponentFrontend(path, pluginID, !overwrite)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			}
		case "operator":
			err = addComponentOperator(path, generator, selectors)
		default:
			return fmt.Errorf("unknown component %s", component)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func addComponentOperator(projectRootPath string, generator *codegen.CustomKindParser, selectors []string) error {
	// Get the repo from the go.mod file
	repo, err := getGoModule(filepath.Join(projectRootPath, "go.mod"))
	if err != nil {
		return err
	}

	files, err := generator.Generate(codegen.OperatorGenerator(repo, "pkg/generated"), selectors...)
	if err != nil {
		return err
	}
	if err := checkAndMakePath("pkg"); err != nil {
		return err
	}
	for _, f := range files {
		err = writeFile(filepath.Join(projectRootPath, f.RelativePath), f.Data)
		if err != nil {
			return err
		}
	}

	dockerfile, err := templates.ReadFile("templates/operator_Dockerfile.tmpl")
	if err != nil {
		return err
	}
	err = writeFile(filepath.Join(projectRootPath, "cmd", "operator", "Dockerfile"), dockerfile)
	if err != nil {
		return err
	}
	return nil
}

/**
Backend plugin
*/

// Linter doesn't like "Potential file inclusion via variable", which is actually desired here
//
//nolint:gosec
func addComponentBackend(projectRootPath string, generator *codegen.CustomKindParser,
	selectors []string, pluginID string) error {
	// Check plugin ID
	if pluginID == "" {
		return fmt.Errorf("plugin-id is required")
	}

	// Get the repo from the go.mod file
	repo, err := getGoModule(filepath.Join(projectRootPath, "go.mod"))
	if err != nil {
		return err
	}

	err = projectAddPluginAPI(generator, repo, filepath.Join(projectRootPath, "pkg/generated"), selectors)
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
		m["executable"] = fmt.Sprintf("gpx_%s-app", pluginID)
		m["backend"] = true
		b, _ = json.MarshalIndent(m, "", "  ")
		err = writeFile(pluginJSONPath, b)
	} else {
		// New plugin.json
		err = writePluginJSON(pluginJSONPath,
			fmt.Sprintf("%s-app", pluginID), "NAME", "AUTHOR", pluginID)
	}
	return err
}

//nolint:revive
func projectAddPluginAPI(generator *codegen.CustomKindParser, repo, generatedAPIModelsPath string, selectors []string) error {
	goFiles, err := generator.FilteredGenerate(codegen.Filter(codegen.BackendPluginGenerator(repo, generatedAPIModelsPath), func(c kindsys.Custom) bool {
		return c.Def().Properties.Codegen.Frontend
	}), selectors...)
	if err != nil {
		return err
	}
	if err := checkAndMakePath("pkg"); err != nil {
		return err
	}
	for _, f := range goFiles {
		err = writeFile(filepath.Join("pkg", f.RelativePath), f.Data)
		if err != nil {
			return err
		}
	}
	return nil
}

/**
Frontend plugin
*/

func addComponentFrontend(projectRootPath string, pluginID string, promptForOverwrite bool) error {
	// Check plugin ID
	if pluginID == "" {
		return fmt.Errorf("plugin-id is required")
	}

	err := writeStaticFrontendFiles(filepath.Join(projectRootPath, "plugin"), promptForOverwrite)
	if err != nil {
		return err
	}
	err = writePluginJSON(filepath.Join(projectRootPath, "plugin/src/plugin.json"),
		fmt.Sprintf("%s-app", pluginID), "NAME", "AUTHOR", pluginID)
	if err != nil {
		return err
	}
	err = writePluginConstants(filepath.Join(projectRootPath, "plugin/src/constants.ts"), pluginID)
	if err != nil {
		return err
	}
	err = writePackageJSON(filepath.Join(projectRootPath, "plugin/package.json"), "NAME", "AUTHOR")
	if err != nil {
		return err
	}
	return nil
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

func writePackageJSON(fullPath, name, author string) error {
	tmp, err := template.ParseFS(templates, "templates/package.json.tmpl")
	if err != nil {
		return err
	}
	data := struct {
		PluginName   string
		PluginAuthor string
	}{
		PluginName:   name,
		PluginAuthor: author,
	}
	b := bytes.Buffer{}
	err = tmp.Execute(&b, data)
	if err != nil {
		return err
	}
	return writeFile(fullPath, b.Bytes())
}

func writePluginConstants(fullPath, pluginID string) error {
	tmp, err := template.ParseFS(templates, "templates/constants.ts.tmpl")
	if err != nil {
		return err
	}
	data := struct {
		PluginID string
	}{
		PluginID: pluginID,
	}
	b := bytes.Buffer{}
	err = tmp.Execute(&b, data)
	if err != nil {
		return err
	}
	return writeFile(fullPath, b.Bytes())
}

func writeStaticFrontendFiles(pluginPath string, promptForOverwrite bool) error {
	return writeStaticFiles(frontEndStaticFiles, "templates/frontend-static", pluginPath, promptForOverwrite)
}

type mergedFS interface {
	fs.ReadDirFS
	fs.ReadFileFS
}

//nolint:revive
func writeStaticFiles(fs mergedFS, readDir, writeDir string, promptForOverwrite bool) error {
	files, err := fs.ReadDir(readDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			err = writeStaticFiles(fs, filepath.Join(readDir, f.Name()), filepath.Join(writeDir, f.Name()),
				promptForOverwrite)
			if err != nil {
				return err
			}
			continue
		}
		b, err := fs.ReadFile(filepath.Join(readDir, f.Name()))
		if err != nil {
			return err
		}
		if promptForOverwrite {
			err = writeFileWithOverwriteConfirm(filepath.Join(writeDir, f.Name()), b)
		} else {
			err = writeFile(filepath.Join(writeDir, f.Name()), b)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
