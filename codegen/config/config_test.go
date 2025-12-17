package config

import (
	"os"
	"testing"

	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigOverridesDefaults(t *testing.T) {
	val, err := cuekind.LoadCue(os.DirFS("testing"))
	require.NoError(t, err)
	cfg, err := Load(val, "configA", nil)
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
	val, err := cuekind.LoadCue(os.DirFS("testing"))
	require.NoError(t, err)
	cfg, err := Load(val, "configB", nil)
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
