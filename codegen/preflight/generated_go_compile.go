package preflight

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/config"
)

// GeneratedGoCodeCompiles runs a preflight compilation check on generated Go files.
func GeneratedGoCodeCompiles(cfg *config.Config, files codejen.Files) error {
	if cfg == nil {
		return generatedGoCodeCompilesWithOverlay(files)
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
	if useOverlayCompilationPreflight(goModule, currentModule) {
		return generatedGoCodeCompilesWithOverlay(files)
	}

	goGenRoot := normalizeAbsolutePath(cfg.Codegen.GoGenPath, cwd)

	moduleGenRoot := cfg.Codegen.GoModGenPath
	if moduleGenRoot == "" {
		moduleGenRoot = cfg.Codegen.GoGenPath
	}
	moduleGenRoot = normalizeRelativePath(moduleGenRoot)
	if filepath.IsAbs(moduleGenRoot) {
		return generatedGoCodeCompilesWithOverlay(files)
	}

	generatedFiles := make([]generatedGoFile, 0, len(files))
	generatedPackageDirs := make(map[string]struct{})
	manifestFileCountByDir := make(map[string]int)

	for _, f := range files {
		if filepath.Ext(f.RelativePath) != ".go" {
			continue
		}

		absTargetPath := normalizeAbsolutePath(f.RelativePath, cwd)

		relPath, err := filepath.Rel(goGenRoot, absTargetPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return generatedGoCodeCompilesWithOverlay(files)
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
			return generatedGoCodeCompilesWithOverlay(files)
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

	out, err := runPreflightGoBuild(tempDir, moduleRoot, "build", "-mod=mod", "./...")
	if err != nil {
		return preflightBuildError(out, err)
	}

	return nil
}

func generatedGoCodeCompilesWithOverlay(files codejen.Files) error {
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

		absTargetPath := normalizeAbsolutePath(f.RelativePath, cwd)

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
	out, err := runPreflightGoBuild(tempDir, "", buildArgs...)
	if err != nil {
		return preflightBuildError(out, err)
	}

	return nil
}

type generatedGoFile struct {
	absDir  string
	relPath string
	data    []byte
}

type goBuildOverlay struct {
	Replace map[string]string `json:"Replace"`
}

func useOverlayCompilationPreflight(goModule, currentModule string) bool {
	if goModule == "" {
		return true
	}
	return currentModule != "" && goModule == currentModule
}

func normalizeAbsolutePath(path, cwd string) string {
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

func normalizeRelativePath(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Clean(path)
}

func runPreflightGoBuild(tempDir, dir string, args ...string) ([]byte, error) {
	buildCmd := exec.Command("go", args...)
	buildCmd.Dir = dir
	buildCmd.Env = append(os.Environ(),
		"GOSUMDB=off",
		fmt.Sprintf("GOCACHE=%s", filepath.Join(tempDir, "gocache")),
	)
	return buildCmd.CombinedOutput()
}

func preflightBuildError(out []byte, err error) error {
	return fmt.Errorf("generated code contains compilation errors, this is likely a bug in sdk. If you'd like to bypass the compilation check, please set skipPreflightCompilationCheck to true.\n\n%s\n%w", strings.TrimSpace(string(out)), err)
}

type goModJSON struct {
	Module struct {
		Path string `json:"Path"`
	} `json:"Module"`
}

func getGoModule(goModPath string) (string, error) {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = filepath.Dir(goModPath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("unable to run go mod edit --json: %w", err)
	}

	var mod goModJSON
	if err := json.Unmarshal(out, &mod); err == nil {
		return mod.Module.Path, nil
	}

	return "", errors.New("unable to locate module in go.mod file")
}
