package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/config"
)

type generatedGoFile struct {
	absDir  string
	relPath string
	data    []byte
}

type tempModuleContext struct {
	cwd                 string
	currentModule       string
	goModule            string
	goGenRoot           string
	moduleGenRoot       string
	generatedFiles      []generatedGoFile
	generatedPackageDir map[string]struct{}
}

func buildTempModuleContext(cfg *config.Config, cwd, currentModule, goModule string, files codejen.Files) (tempModuleContext, bool, error) {
	moduleGenRoot := cfg.Codegen.GoModGenPath
	if moduleGenRoot == "" {
		moduleGenRoot = cfg.Codegen.GoGenPath
	}
	moduleGenRoot = normalizeRelativePath(moduleGenRoot)
	if filepath.IsAbs(moduleGenRoot) {
		return tempModuleContext{}, false, nil
	}

	ctx := tempModuleContext{
		cwd:                 cwd,
		currentModule:       currentModule,
		goModule:            goModule,
		goGenRoot:           normalizeAbsolutePath(cfg.Codegen.GoGenPath, cwd),
		moduleGenRoot:       moduleGenRoot,
		generatedPackageDir: make(map[string]struct{}),
	}

	ok, err := collectGeneratedFilesForTempModule(&ctx, files)
	if err != nil {
		return tempModuleContext{}, false, err
	}
	if !ok {
		return tempModuleContext{}, false, nil
	}

	filterOutConflictingManifestPackages(&ctx)
	return ctx, true, nil
}

func collectGeneratedFilesForTempModule(ctx *tempModuleContext, files codejen.Files) (bool, error) {
	manifestFileCountByDir := make(map[string]int)
	generatedFiles := make([]generatedGoFile, 0, len(files))

	for _, f := range files {
		if filepath.Ext(f.RelativePath) != ".go" {
			continue
		}

		absTargetPath := normalizeAbsolutePath(f.RelativePath, ctx.cwd)
		relPath, err := filepath.Rel(ctx.goGenRoot, absTargetPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return false, nil
		}

		dir := filepath.Dir(absTargetPath)
		generatedFiles = append(generatedFiles, generatedGoFile{
			absDir:  dir,
			relPath: filepath.Join(ctx.moduleGenRoot, relPath),
			data:    f.Data,
		})
		ctx.generatedPackageDir[dir] = struct{}{}
		if strings.HasSuffix(absTargetPath, "_manifest.go") {
			manifestFileCountByDir[dir]++
		}
	}

	ctx.generatedFiles = generatedFiles
	for dir, count := range manifestFileCountByDir {
		if count > 1 {
			delete(ctx.generatedPackageDir, dir)
		}
	}

	return true, nil
}

func filterOutConflictingManifestPackages(ctx *tempModuleContext) {
	if len(ctx.generatedFiles) == 0 {
		return
	}

	filtered := ctx.generatedFiles[:0]
	for _, f := range ctx.generatedFiles {
		if _, keep := ctx.generatedPackageDir[f.absDir]; !keep {
			continue
		}
		filtered = append(filtered, f)
	}
	ctx.generatedFiles = filtered
}

func compileGeneratedGoCodeWithTempModule(ctx tempModuleContext) error {
	tempDir, moduleRoot, err := createTempModuleRoot()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := copyExistingGoFilesIntoTempModule(ctx, moduleRoot); err != nil {
		return err
	}
	if err := writeGeneratedFilesIntoTempModule(ctx.generatedFiles, moduleRoot); err != nil {
		return err
	}
	if err := writeTempGoMod(ctx, moduleRoot); err != nil {
		return err
	}

	out, err := runPreflightGoBuild(tempDir, moduleRoot, "build", "-mod=mod", "./...")
	if err != nil {
		return preflightBuildError(out, err)
	}
	return nil
}

func createTempModuleRoot() (string, string, error) {
	tempDir, err := os.MkdirTemp("", preflightTempDirPattern)
	if err != nil {
		return "", "", err
	}
	moduleRoot := filepath.Join(tempDir, "module")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", "", err
	}
	return tempDir, moduleRoot, nil
}

func copyExistingGoFilesIntoTempModule(ctx tempModuleContext, moduleRoot string) error {
	for packageDir := range ctx.generatedPackageDir {
		relDir, err := filepath.Rel(ctx.goGenRoot, packageDir)
		if err != nil || strings.HasPrefix(relDir, "..") {
			return fmt.Errorf("unable to map generated package directory '%s' into temp module root", packageDir)
		}
		tempPackageDir := filepath.Join(moduleRoot, ctx.moduleGenRoot, relDir)
		if err := os.MkdirAll(tempPackageDir, 0o755); err != nil {
			return err
		}
		if err := copyGoFilesInDirectory(packageDir, tempPackageDir); err != nil {
			return err
		}
	}
	return nil
}

func copyGoFilesInDirectory(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func writeGeneratedFilesIntoTempModule(files []generatedGoFile, moduleRoot string) error {
	for _, f := range files {
		tempFilePath := filepath.Join(moduleRoot, f.relPath)
		if err := os.MkdirAll(filepath.Dir(tempFilePath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(tempFilePath, f.data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func writeTempGoMod(ctx tempModuleContext, moduleRoot string) error {
	goModContents := fmt.Sprintf("module %s\n\ngo 1.24.0\n", ctx.goModule)
	if ctx.currentModule != "" && ctx.currentModule != ctx.goModule {
		goModContents += fmt.Sprintf(
			"\nrequire %s v0.0.0\nreplace %s => %s\n",
			ctx.currentModule, ctx.currentModule, ctx.cwd,
		)
	}
	return os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte(goModContents), 0o600)
}
