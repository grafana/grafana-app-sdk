package cuekind

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigOverridesDefaults(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`
config: {
	kinds: {
		grouping: "group"
		perKindVersion: true
	}
	customResourceDefinitions: {
		includeInManifest: false
		format: "yaml"
		path: "custom/defs"
		useCRDFormat: true
	}
	codegen: {
		goModule: "github.com/example/module"
		goModGenPath: "internal/mod"
		goGenPath: "alt/pkg/"
		tsGenPath: "alt/ts/"
		enableK8sPostProcessing: true
		enableOperatorStatusGeneration: false
	}
}
`)

	cfg, err := parseConfig(val)
	require.NoError(t, err)

	assert.Equal(t, "group", cfg.Kinds.Grouping)
	assert.True(t, cfg.Kinds.PerKindVersion)

	assert.False(t, cfg.CustomResourceDefinitions.IncludeInManifest)
	assert.Equal(t, "yaml", cfg.CustomResourceDefinitions.Format)
	assert.Equal(t, "custom/defs", cfg.CustomResourceDefinitions.Path)
	assert.True(t, cfg.CustomResourceDefinitions.UseCRDFormat)

	assert.Equal(t, "github.com/example/module", cfg.Codegen.GoModule)
	assert.Equal(t, "internal/mod", cfg.Codegen.GoModGenPath)
	assert.Equal(t, "alt/pkg/", cfg.Codegen.GoGenPath)
	assert.Equal(t, "alt/ts/", cfg.Codegen.TsGenPath)
	assert.True(t, cfg.Codegen.EnableK8sPostProcessing)
	assert.False(t, cfg.Codegen.EnableOperatorStatusGeneration)
}

func TestParseConfigDefaultFallback(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`
config: {
	codegen: {
		goModule: "github.com/example/module"
	}
}
`)

	cfg, err := parseConfig(val)
	require.NoError(t, err)

	assert.Equal(t, "kind", cfg.Kinds.Grouping)
	assert.False(t, cfg.Kinds.PerKindVersion)

	assert.True(t, cfg.CustomResourceDefinitions.IncludeInManifest)
	assert.Equal(t, "json", cfg.CustomResourceDefinitions.Format)
	assert.Equal(t, "definitions", cfg.CustomResourceDefinitions.Path)
	assert.False(t, cfg.CustomResourceDefinitions.UseCRDFormat)

	assert.Equal(t, "pkg/generated/", cfg.Codegen.GoGenPath)
	assert.Equal(t, "plugin/src/generated/", cfg.Codegen.TsGenPath)
	assert.False(t, cfg.Codegen.EnableK8sPostProcessing)
	assert.True(t, cfg.Codegen.EnableOperatorStatusGeneration)

	assert.Equal(t, "github.com/example/module", cfg.Codegen.GoModule)
	assert.Equal(t, "", cfg.Codegen.GoModGenPath)
}
