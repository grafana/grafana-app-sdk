package cuekind

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

// generateGoManifest parses selector and runs the Go manifest generator over it,
// returning the generated *_manifest.go contents or the first error from either
// stage. The codegen rules for source.kv may be enforced at parse or at generate
// time; callers only care that generating the manifest fails.
func generateGoManifest(t *testing.T, selector string) (string, error) {
	t.Helper()
	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)
	manifests, err := parser.ManifestParser().Parse(selector)
	if err != nil {
		return "", err
	}
	files, err := ManifestGoGenerator(ManifestGoGeneratorConfig{
		Package:            "manifestdata",
		ProjectRepo:        "codegen-tests",
		GoGenPath:          "pkg/generated",
		ManifestGoFilePath: "manifestdata",
		GroupKinds:         true,
		SkipImportsProcess: true,
	}).Generate(manifests...)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if strings.HasSuffix(f.RelativePath, "_manifest.go") {
			return string(f.Data), nil
		}
	}
	return "", nil
}

func TestParseManifest_SearchFieldKVSource(t *testing.T) {
	t.Parallel()

	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)
	manifest, err := parser.ParseManifest("kvSourceManifest")
	require.NoError(t, err, "a source.kv field without path must be accepted")

	versions := manifest.Versions()
	require.Len(t, versions, 1)
	byKind := map[string]codegen.VersionedKind{}
	for _, k := range versions[0].Kinds() {
		byKind[k.Kind] = k
	}

	kvKind, ok := byKind["KVSourced"]
	require.True(t, ok)
	require.Len(t, kvKind.SearchFields, 3)
	fields := map[string]codegen.SearchField{}
	for _, f := range kvKind.SearchFields {
		fields[f.Name] = f
	}

	t.Run("path field has no source", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "spec.title", fields["title"].Path)
		assert.Nil(t, fields["title"].Source)
	})

	t.Run("kv field decodes owner, key and path", func(t *testing.T) {
		t.Parallel()
		f := fields["views_total"]
		assert.Empty(t, f.Path)
		require.NotNil(t, f.Source)
		require.NotNil(t, f.Source.KV)
		assert.Equal(t, codegen.SearchFieldKVSource{Owner: "usageinsights.grafana.app", Key: "stats", Path: "views_total"}, *f.Source.KV)
		assert.Equal(t, "int64", f.Type)
		assert.Equal(t, []string{"sort", "retrieve"}, f.Capabilities)
	})

	t.Run("kv field with nested key and dotted path", func(t *testing.T) {
		t.Parallel()
		f := fields["nested_views"]
		require.NotNil(t, f.Source)
		require.NotNil(t, f.Source.KV)
		assert.Equal(t, codegen.SearchFieldKVSource{Owner: "other-owner.example.app", Key: "daily/rollup", Path: "totals.views"}, *f.Source.KV)
	})

	t.Run("kind with only path fields has no sources", func(t *testing.T) {
		t.Parallel()
		plain, ok := byKind["PlainSearch"]
		require.True(t, ok)
		require.Len(t, plain.SearchFields, 1)
		assert.Nil(t, plain.SearchFields[0].Source)
	})
}

var (
	kvSourceLiteralRe = regexp.MustCompile(`(?s)Source:\s*&app\.ManifestVersionKindSearchFieldSource\{\s*KV:\s*&app\.ManifestVersionKindSearchFieldKVSource\{(.*?)\}`)
	sourceFieldRe     = regexp.MustCompile(`Source:\s`)
)

func TestManifestGoGenerator_SearchFieldKVSource(t *testing.T) {
	t.Parallel()

	content, err := generateGoManifest(t, "kvSourceManifest")
	require.NoError(t, err)
	require.NotEmpty(t, content, "Go manifest file must be generated")

	literals := kvSourceLiteralRe.FindAllStringSubmatch(content, -1)
	require.Len(t, literals, 2, "exactly the two kv-sourced fields carry a Source")
	assert.Len(t, sourceFieldRe.FindAllString(content, -1), 2, "no other search field may carry a Source")

	want := []codegen.SearchFieldKVSource{
		{Owner: "usageinsights.grafana.app", Key: "stats", Path: "views_total"},
		{Owner: "other-owner.example.app", Key: "daily/rollup", Path: "totals.views"},
	}
	for i, w := range want {
		body := literals[i][1]
		assert.Regexp(t, `Owner:\s*"`+regexp.QuoteMeta(w.Owner)+`"`, body)
		assert.Regexp(t, `Key:\s*"`+regexp.QuoteMeta(w.Key)+`"`, body)
		assert.Regexp(t, `Path:\s*"`+regexp.QuoteMeta(w.Path)+`"`, body)
	}
}

func TestManifestGeneration_SearchFieldKVSourceRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		selector    string
		errContains string
	}{
		{name: "path and source together", selector: "invalidKVSourcePathAndSource", errContains: "path and source are mutually exclusive"},
		{name: "kv source on an array field", selector: "invalidKVSourceArray", errContains: "kv source fields must not be arrays"},
		{name: "kv path with array projection", selector: "invalidKVSourcePathProjection", errContains: "source.kv.path"},
		{name: "kv path with leading dot", selector: "invalidKVSourcePathLeadingDot", errContains: "source.kv.path"},
		{name: "kv owner outside the allowed charset", selector: "invalidKVSourceOwner", errContains: "source.kv.owner"},
		{name: "kv key with an empty segment", selector: "invalidKVSourceKey", errContains: "source.kv.key"},
		{name: "kv key with upper case", selector: "invalidKVSourceKeyCase", errContains: "source.kv.key"},
		{name: "source without kv", selector: "invalidKVSourceEmpty", errContains: "source.kv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := generateGoManifest(t, tt.selector)
			require.ErrorContains(t, err, tt.errContains)
		})
	}
}

// TestParseManifest_KVSourcePathConstraint checks the CUE schema itself: a
// non-dotted source.kv.path is rejected when the manifest is parsed.
func TestParseManifest_KVSourcePathConstraint(t *testing.T) {
	t.Parallel()

	parser, err := NewParser(testingCue(t), false)
	require.NoError(t, err)
	for _, selector := range []string{"invalidKVSourcePathProjection", "invalidKVSourcePathLeadingDot"} {
		t.Run(selector, func(t *testing.T) {
			t.Parallel()
			_, err := parser.ParseManifest(selector)
			require.ErrorContains(t, err, "source.kv.path")
		})
	}
}
