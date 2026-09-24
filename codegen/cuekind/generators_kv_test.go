package cuekind

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

// TestParseManifest_KV verifies that the CUE parser correctly decodes the kv block
// from a manifest definition into the codegen.VersionedKind representation.

// gofmt aligns single-line composite values, so allow any whitespace after "KV:".
var kvBlockRe = regexp.MustCompile(`KV:\s+&app\.ManifestVersionKindKV\{`)

func TestParseManifest_KV(t *testing.T) {
	t.Parallel()

	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)

	manifest, err := parser.ParseManifest("kvManifest")
	require.NoError(t, err)

	versions := manifest.Versions()
	require.Len(t, versions, 1)
	kinds := versions[0].Kinds()
	require.Len(t, kinds, 3)

	kindsByName := make(map[string]codegen.VersionedKind, len(kinds))
	for _, k := range kinds {
		kindsByName[k.Kind] = k
	}

	t.Run("empty kv block is non-nil", func(t *testing.T) {
		t.Parallel()
		k, ok := kindsByName["KVEmpty"]
		require.True(t, ok, "KVEmpty kind must be present")
		require.NotNil(t, k.KV, "kv:{} block must decode to a non-nil KV pointer")
		assert.Equal(t, 0, k.KV.MaxValueBytes)
		assert.Equal(t, 0, k.KV.MaxKeysPerOwner)
	})

	t.Run("kv with explicit limits", func(t *testing.T) {
		t.Parallel()
		k, ok := kindsByName["KVLimited"]
		require.True(t, ok, "KVLimited kind must be present")
		require.NotNil(t, k.KV)
		assert.Equal(t, 1024, k.KV.MaxValueBytes)
		assert.Equal(t, 50, k.KV.MaxKeysPerOwner)
	})

	t.Run("kind without kv has nil KV", func(t *testing.T) {
		t.Parallel()
		k, ok := kindsByName["NoKV"]
		require.True(t, ok, "NoKV kind must be present")
		assert.Nil(t, k.KV, "kind without kv block must have nil KV")
	})
}

// TestBuildManifestData_KV_FromCUE verifies that buildManifestData correctly propagates
// the kv field from a CUE-parsed manifest into the ManifestData structure used by
// the Go manifest generator and downstream consumers.
//
// Note: the JSON/YAML AppManifest resource format (v1alpha1/v1alpha2) is not in scope
// for slice C3 and does not include the kv field.
func TestBuildManifestData_KV_FromCUE(t *testing.T) {
	t.Parallel()

	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)
	kinds, err := parser.ManifestParser().Parse("kvManifest")
	require.NoError(t, err)

	// Reuse the ManifestGoGenerator infrastructure to extract the manifest data
	// (buildManifestData is package-private to the jennies package, but the Go
	// template output is the authoritative representation for C3).
	files, err := ManifestGoGenerator(ManifestGoGeneratorConfig{
		Package:            "manifestdata",
		ProjectRepo:        "codegen-tests",
		GoGenPath:          "pkg/generated",
		ManifestGoFilePath: "manifestdata",
		GroupKinds:         true,
		SkipImportsProcess: true,
	}).Generate(kinds...)
	require.NoError(t, err)

	var manifestContent string
	for _, f := range files {
		if strings.HasSuffix(f.RelativePath, "_manifest.go") {
			manifestContent = string(f.Data)
			break
		}
	}
	require.NotEmpty(t, manifestContent, "Go manifest file must be generated")

	t.Run("kv block present for KVEmpty", func(t *testing.T) {
		t.Parallel()
		assert.Regexp(t, kvBlockRe, manifestContent,
			"empty kv:{} must produce a KV block in generated Go")
	})

	t.Run("kv block present with values for KVLimited", func(t *testing.T) {
		t.Parallel()
		assert.Regexp(t, kvBlockRe, manifestContent)
		assert.Regexp(t, `MaxValueBytes:\s+1024`, manifestContent)
		assert.Regexp(t, `MaxKeysPerOwner:\s+50`, manifestContent)
	})

	t.Run("no kv block for NoKV", func(t *testing.T) {
		t.Parallel()
		// Two KV blocks expected: one for KVEmpty, one for KVLimited.
		count := len(kvBlockRe.FindAllString(manifestContent, -1))
		assert.Equal(t, 2, count, "expected exactly 2 KV blocks (KVEmpty + KVLimited)")
	})
}

// TestManifestGoGenerator_KV verifies the Go manifest template emits the KV block
// for declaring kinds and omits it for non-declaring kinds.
func TestManifestGoGenerator_KV(t *testing.T) {
	t.Parallel()

	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)
	kinds, err := parser.ManifestParser().Parse("kvManifest")
	require.NoError(t, err)

	files, err := ManifestGoGenerator(ManifestGoGeneratorConfig{
		Package:            "manifestdata",
		ProjectRepo:        "codegen-tests",
		GoGenPath:          "pkg/generated",
		ManifestGoFilePath: "manifestdata",
		GroupKinds:         true,
		SkipImportsProcess: true,
	}).Generate(kinds...)
	require.NoError(t, err)

	// Find the manifest file (should be the first non-client file)
	var manifestContent string
	for _, f := range files {
		if strings.HasSuffix(f.RelativePath, "_manifest.go") {
			manifestContent = string(f.Data)
			break
		}
	}
	require.NotEmpty(t, manifestContent, "manifest Go file must be generated")

	t.Run("KVEmpty kind has KV block in Go", func(t *testing.T) {
		t.Parallel()
		assert.Regexp(t, kvBlockRe, manifestContent,
			"generated Go must contain KV block for KVEmpty")
	})

	t.Run("KVLimited kind has KV with values in Go", func(t *testing.T) {
		t.Parallel()
		assert.Regexp(t, kvBlockRe, manifestContent)
		assert.Regexp(t, `MaxValueBytes:\s+1024`, manifestContent)
		assert.Regexp(t, `MaxKeysPerOwner:\s+50`, manifestContent)
	})

	t.Run("NoKV kind count of KV blocks is correct", func(t *testing.T) {
		t.Parallel()
		// There are 2 kinds with kv, 1 without — exactly 2 KV blocks expected.
		count := len(kvBlockRe.FindAllString(manifestContent, -1))
		assert.Equal(t, 2, count, "expected exactly 2 KV blocks in generated Go (one per declaring kind)")
	})
}
