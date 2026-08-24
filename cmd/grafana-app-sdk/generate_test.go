package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen/config"
	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
)

// testConfig returns a config equivalent to the `configJson` selector in
// codegen/cuekind/testing/config.cue, with go codegen enabled or disabled.
func testConfig(goEnabled bool) *config.Config {
	return &config.Config{
		Kinds: &config.KindsConfig{Grouping: config.KindGroupingGroup},
		Definitions: &config.DefinitionsConfig{
			GenManifest:     true,
			GenCRDs:         true,
			ManifestSchemas: true,
			Encoding:        "json",
			Path:            "definitions",
			ManifestVersion: jennies.VersionV1Alpha1,
		},
		Codegen: &config.CodegenConfig{
			GoEnabled:    goEnabled,
			GoModule:     "codegen-tests",
			GoModGenPath: "pkg/generated",
			GoGenPath:    "pkg/generated",
			TsGenPath:    "plugin/src/generated",
		},
		ManifestSelectors: []string{"customManifest", "testManifest"},
	}
}

func testParser(t *testing.T) *cuekind.Parser {
	t.Helper()
	c, err := cuekind.LoadCue(os.DirFS(filepath.Join("..", "..", "codegen", "cuekind", "testing")))
	require.NoError(t, err)
	parser, err := cuekind.NewParser(c, true)
	require.NoError(t, err)
	return parser
}

// TestGenerateKindsCueGoEnabled is the control case: with go codegen on, go files are generated
// alongside the TypeScript and definition files.
func TestGenerateKindsCueGoEnabled(t *testing.T) {
	files, err := generateKindsCue(testParser(t), testConfig(true))
	require.NoError(t, err)

	var goCount, tsCount, defCount int
	for _, f := range files {
		switch filepath.Ext(f.RelativePath) {
		case ".go":
			goCount++
		case ".ts":
			tsCount++
		case ".json":
			defCount++
		}
	}
	assert.Positive(t, goCount, "go files should be generated when goEnabled is true")
	assert.Positive(t, tsCount, "TypeScript files should be generated")
	assert.Positive(t, defCount, "CRD/manifest files should be generated")
}

// TestGenerateKindsCueGoDisabled verifies that `codegen: goEnabled: false` suppresses all go
// output, while leaving TypeScript and CRD/manifest generation untouched.
func TestGenerateKindsCueGoDisabled(t *testing.T) {
	files, err := generateKindsCue(testParser(t), testConfig(false))
	require.NoError(t, err)

	var tsCount, defCount int
	for _, f := range files {
		assert.NotEqual(t, ".go", filepath.Ext(f.RelativePath),
			"no go files should be generated when goEnabled is false, got %s", f.RelativePath)
		assert.False(t, strings.HasSuffix(f.RelativePath, "_manifest.go"),
			"the go app manifest should not be generated when goEnabled is false")
		switch filepath.Ext(f.RelativePath) {
		case ".ts":
			tsCount++
		case ".json":
			defCount++
		}
	}
	assert.Positive(t, tsCount, "TypeScript files should still be generated when goEnabled is false")
	assert.Positive(t, defCount, "CRD/manifest files should still be generated when goEnabled is false")
}

// TestGenerateKindsCueManifestFileNameMultipleSelectors verifies that a fixed manifest filename
// is rejected when more than one manifest would be generated, rather than silently having each
// manifest clobber the previous one.
func TestGenerateKindsCueManifestFileNameMultipleSelectors(t *testing.T) {
	cfg := testConfig(true)
	cfg.Definitions.ManifestFileName = "my-manifest.json"
	require.Len(t, cfg.ManifestSelectors, 2, "this test relies on more than one manifest selector")

	_, err := generateKindsCue(testParser(t), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifestFileName")
}

// TestGenerateKindsCueManifestFileNameSingleSelector verifies that with a single manifest,
// the configured filename is used verbatim inside the definitions path.
func TestGenerateKindsCueManifestFileNameSingleSelector(t *testing.T) {
	cfg := testConfig(true)
	cfg.Definitions.ManifestFileName = "my-manifest.json"
	cfg.ManifestSelectors = []string{"testManifest"}

	files, err := generateKindsCue(testParser(t), cfg)
	require.NoError(t, err)

	want := filepath.Join(cfg.Definitions.Path, "my-manifest.json")
	var found int
	for _, f := range files {
		if f.RelativePath == want {
			found++
		}
	}
	assert.Equal(t, 1, found, "expected exactly one manifest written to %s", want)
}

// TestGenerateKindsCueManifestFileNameUnset verifies the default filename is unchanged when
// no manifestFileName is configured.
func TestGenerateKindsCueManifestFileNameUnset(t *testing.T) {
	cfg := testConfig(true)
	cfg.ManifestSelectors = []string{"testManifest"}

	files, err := generateKindsCue(testParser(t), cfg)
	require.NoError(t, err)

	want := filepath.Join(cfg.Definitions.Path, "test-app-manifest.json")
	var found int
	for _, f := range files {
		if f.RelativePath == want {
			found++
		}
	}
	assert.Equal(t, 1, found, "expected the default manifest filename at %s", want)
}
