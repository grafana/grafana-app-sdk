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

type packageCopySpec struct {
	srcDir string
	dstDir string
}

type tempModuleContext struct {
	cwd            string
	currentModule  string
	goModule       string
	generatedFiles []generatedGoFile
	packageCopies  []packageCopySpec
}

func buildTempModuleContext(cfg *config.Config, cwd, currentModule, goModule string, files codejen.Files) (tempModuleContext, error) {
	goGenRoot := normalizeAbsolutePath(cfg.Codegen.GoGenPath, cwd)
	moduleGenRoot := cfg.Codegen.GoModGenPath
	if moduleGenRoot == "" {
		moduleGenRoot = cfg.Codegen.GoGenPath
	}
	moduleGenRoot = normalizeRelativePath(moduleGenRoot)
	if filepath.IsAbs(moduleGenRoot) {
		return tempModuleContext{}, errUseOverlayStrategy
	}

	generatedFiles, packageDirs, err := collectGeneratedFiles(files, cwd, goGenRoot, moduleGenRoot)
	if err != nil {
		return tempModuleContext{}, err
	}
	if len(generatedFiles) == 0 {
		return tempModuleContext{
			cwd:            cwd,
			currentModule:  currentModule,
			goModule:       goModule,
			generatedFiles: nil,
			packageCopies:  nil,
		}, nil
	}

	packageCopies, err := buildPackageCopySpecs(packageDirs, goGenRoot, moduleGenRoot)
	if err != nil {
		return tempModuleContext{}, err
	}

	return tempModuleContext{
		cwd:            cwd,
		currentModule:  currentModule,
		goModule:       goModule,
		generatedFiles: generatedFiles,
		packageCopies:  packageCopies,
	}, nil
}

func collectGeneratedFiles(files codejen.Files, cwd, goGenRoot, moduleGenRoot string) ([]generatedGoFile, map[string]int, error) {
	manifestFileCountByDir := make(map[string]int)
	generatedFiles := make([]generatedGoFile, 0, len(files))

	for _, f := range files {
		if filepath.Ext(f.RelativePath) != goSourceFileExtension {
			continue
		}

		absTargetPath := normalizeAbsolutePath(f.RelativePath, cwd)
		relPath, err := filepath.Rel(goGenRoot, absTargetPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return nil, nil, errUseOverlayStrategy
		}

		dir := filepath.Dir(absTargetPath)
		generatedFiles = append(generatedFiles, generatedGoFile{
			absDir:  dir,
			relPath: filepath.Join(moduleGenRoot, relPath),
			data:    f.Data,
		})
		manifestFileCountByDir[dir]++
	}

	if len(generatedFiles) == 0 {
		return nil, nil, nil
	}

	conflictingDirs := conflictingManifestDirs(generatedFiles)
	if len(conflictingDirs) > 0 {
		filtered := generatedFiles[:0]
		for _, f := range generatedFiles {
			if _, skip := conflictingDirs[f.absDir]; skip {
				continue
			}
			filtered = append(filtered, f)
		}
		generatedFiles = filtered
		for dir := range conflictingDirs {
			delete(manifestFileCountByDir, dir)
		}
	}

	return generatedFiles, manifestFileCountByDir, nil
}

func conflictingManifestDirs(files []generatedGoFile) map[string]struct{} {
	manifestCounts := make(map[string]int)
	for _, f := range files {
		if strings.HasSuffix(f.relPath, "_manifest.go") {
			manifestCounts[f.absDir]++
		}
	}

	conflicting := make(map[string]struct{})
	for dir, count := range manifestCounts {
		if count > 1 {
			conflicting[dir] = struct{}{}
		}
	}
	return conflicting
}

func buildPackageCopySpecs(packageDirs map[string]int, goGenRoot, moduleGenRoot string) ([]packageCopySpec, error) {
	specs := make([]packageCopySpec, 0, len(packageDirs))
	for dir := range packageDirs {
		relDir, err := filepath.Rel(goGenRoot, dir)
		if err != nil || strings.HasPrefix(relDir, "..") {
			return nil, errUseOverlayStrategy
		}
		specs = append(specs, packageCopySpec{
			srcDir: dir,
			dstDir: filepath.Join(moduleGenRoot, relDir),
		})
	}
	return specs, nil
}

func compileGeneratedGoCodeWithTempModule(ctx tempModuleContext) error {
	tempDir, moduleRoot, err := createTempModuleRoot()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := copyExistingGoFilesIntoTempModule(ctx.packageCopies, moduleRoot); err != nil {
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

func createTempModuleRoot() (tempDir string, moduleRoot string, err error) {
	tempDir, err = os.MkdirTemp("", preflightTempDirPattern)
	if err != nil {
		return "", "", err
	}
	moduleRoot = filepath.Join(tempDir, "module")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", "", err
	}
	return tempDir, moduleRoot, nil
}

func copyExistingGoFilesIntoTempModule(specs []packageCopySpec, moduleRoot string) error {
	for _, spec := range specs {
		tempPackageDir := filepath.Join(moduleRoot, spec.dstDir)
		if err := os.MkdirAll(tempPackageDir, 0o755); err != nil {
			return err
		}
		if err := copyGoFilesInDirectory(spec.srcDir, tempPackageDir); err != nil {
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
		if entry.IsDir() || filepath.Ext(entry.Name()) != goSourceFileExtension {
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
