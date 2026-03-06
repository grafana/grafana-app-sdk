package preflight

import (
	"errors"
	"os"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/config"
)

// GeneratedGoCodeCompiles runs a preflight compilation check on generated Go files.
func GeneratedGoCodeCompiles(cfg *config.Config, files codejen.Files) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if cfg == nil {
		return compileGeneratedGoCodeWithOverlay(files, cwd)
	}

	currentModule, _ := getGoModule("go.mod")
	goModule := resolveGoModule(cfg, currentModule)
	if shouldUseOverlayStrategy(goModule, currentModule) {
		return compileGeneratedGoCodeWithOverlay(files, cwd)
	}

	ctx, err := buildTempModuleContext(cfg, cwd, currentModule, goModule, files)
	if errors.Is(err, errUseOverlayStrategy) {
		return compileGeneratedGoCodeWithOverlay(files, cwd)
	}
	if err != nil {
		return err
	}
	if len(ctx.generatedFiles) == 0 {
		return nil
	}

	return compileGeneratedGoCodeWithTempModule(ctx)
}

func resolveGoModule(cfg *config.Config, currentModule string) string {
	goModule := cfg.Codegen.GoModule
	if goModule == "" {
		return currentModule
	}
	return goModule
}
