package preflight

import (
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/config"
)

// GeneratedGoCodeCompiles runs a preflight compilation check on generated Go files.
func GeneratedGoCodeCompiles(cfg *config.Config, files codejen.Files) error {
	if cfg == nil {
		return compileGeneratedGoCodeWithOverlay(files)
	}

	cwd, err := getWorkingDirectory()
	if err != nil {
		return err
	}

	currentModule, _ := getGoModule("go.mod")
	goModule := resolveGoModule(cfg, currentModule)
	if useOverlayCompilationPreflight(goModule, currentModule) {
		return compileGeneratedGoCodeWithOverlay(files)
	}

	ctx, shouldUseTempModule, err := buildTempModuleContext(cfg, cwd, currentModule, goModule, files)
	if err != nil {
		return err
	}
	if !shouldUseTempModule {
		return compileGeneratedGoCodeWithOverlay(files)
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
