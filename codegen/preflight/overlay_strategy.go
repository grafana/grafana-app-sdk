package preflight

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/grafana/codejen"
)

func compileGeneratedGoCodeWithOverlay(files codejen.Files, cwd string) error {
	overlay := goBuildOverlay{Replace: make(map[string]string)}

	tempDir, err := os.MkdirTemp("", preflightTempDirPattern)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	packages, err := writeOverlayFiles(files, cwd, tempDir, overlay.Replace)
	if err != nil || len(packages) == 0 {
		return err
	}

	overlayPath, err := writeOverlayJSON(tempDir, overlay)
	if err != nil {
		return err
	}

	buildArgs := append([]string{"build", "-overlay", overlayPath}, packages...)
	out, err := runPreflightGoBuild(tempDir, "", buildArgs...)
	if err != nil {
		return preflightBuildError(out, err)
	}
	return nil
}

func writeOverlayFiles(files codejen.Files, cwd, tempDir string, replaceMap map[string]string) ([]string, error) {
	generatedPackages := make(map[string]struct{})
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
			return nil, err
		}
		replaceMap[absTargetPath] = tempFilePath
	}

	if len(generatedPackages) == 0 {
		return nil, nil
	}

	packages := make([]string, 0, len(generatedPackages))
	for pkg := range generatedPackages {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return packages, nil
}

func writeOverlayJSON(tempDir string, overlay goBuildOverlay) (string, error) {
	overlayBytes, err := json.Marshal(overlay)
	if err != nil {
		return "", err
	}
	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayBytes, 0o600); err != nil {
		return "", err
	}
	return overlayPath, nil
}

type goBuildOverlay struct {
	Replace map[string]string `json:"Replace"`
}
