package preflight

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const preflightTempDirPattern = "grafana-app-sdk-generate-preflight-*"

func getWorkingDirectory() (string, error) {
	return os.Getwd()
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
