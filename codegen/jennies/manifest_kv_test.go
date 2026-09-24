package jennies

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

// TestManifestKindKV_Translation verifies manifestKindKV translates codegen.KindKV into
// app.ManifestVersionKindKV with the correct field mapping.
func TestManifestKindKV_Translation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *codegen.KindKV
		expected *app.ManifestVersionKindKV
	}{
		{
			name:     "nil returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty struct returns empty struct",
			input:    &codegen.KindKV{},
			expected: &app.ManifestVersionKindKV{},
		},
		{
			name:  "MaxValueBytes only",
			input: &codegen.KindKV{MaxValueBytes: 1024},
			expected: &app.ManifestVersionKindKV{
				MaxValueBytes: 1024,
			},
		},
		{
			name:  "MaxKeysPerOwner only",
			input: &codegen.KindKV{MaxKeysPerOwner: 50},
			expected: &app.ManifestVersionKindKV{
				MaxKeysPerOwner: 50,
			},
		},
		{
			name:  "both fields",
			input: &codegen.KindKV{MaxValueBytes: 2048, MaxKeysPerOwner: 100},
			expected: &app.ManifestVersionKindKV{
				MaxValueBytes:   2048,
				MaxKeysPerOwner: 100,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := manifestKindKV(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

// TestProcessKindVersion_KV verifies that processKindVersion maps KV from VersionedKind
// to the ManifestVersionKind with presence semantics preserved.
func TestProcessKindVersion_KV(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		kv       *codegen.KindKV
		expected *app.ManifestVersionKindKV
	}{
		{name: "nil KV produces nil", kv: nil, expected: nil},
		{name: "empty KV block propagates", kv: &codegen.KindKV{}, expected: &app.ManifestVersionKindKV{}},
		{
			name:     "KV with MaxValueBytes",
			kv:       &codegen.KindKV{MaxValueBytes: 4096},
			expected: &app.ManifestVersionKindKV{MaxValueBytes: 4096},
		},
		{
			name:     "KV with MaxKeysPerOwner",
			kv:       &codegen.KindKV{MaxKeysPerOwner: 200},
			expected: &app.ManifestVersionKindKV{MaxKeysPerOwner: 200},
		},
		{
			name: "KV with both fields",
			kv:   &codegen.KindKV{MaxValueBytes: 8192, MaxKeysPerOwner: 50},
			expected: &app.ManifestVersionKindKV{
				MaxValueBytes:   8192,
				MaxKeysPerOwner: 50,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mver, err := processKindVersion(codegen.VersionedKind{
				Kind:         "Foo",
				PluralName:   "Foos",
				Scope:        "Namespaced",
				FolderScoped: true,
				KV:           tc.kv,
			}, "v1", false)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, mver.KV)
		})
	}
}

// TestBuildManifestData_KV_EmptyBlock verifies that a kind with an empty kv block
// produces a ManifestData where KV is non-nil (presence is the opt-in).
func TestBuildManifestData_KV_EmptyBlock(t *testing.T) {
	t.Parallel()

	manifest, err := buildManifestData(newKVTestManifest(&codegen.KindKV{}), false)
	require.NoError(t, err)
	require.Len(t, manifest.Versions, 1)
	require.Len(t, manifest.Versions[0].Kinds, 1)
	kind := manifest.Versions[0].Kinds[0]
	require.NotNil(t, kind.KV, "empty kv block must produce a non-nil KV field")
	assert.Equal(t, 0, kind.KV.MaxValueBytes)
	assert.Equal(t, 0, kind.KV.MaxKeysPerOwner)
}

// TestBuildManifestData_KV_WithValues verifies a kind with kv values produces the correct manifest.
func TestBuildManifestData_KV_WithValues(t *testing.T) {
	t.Parallel()

	manifest, err := buildManifestData(newKVTestManifest(&codegen.KindKV{MaxValueBytes: 4096, MaxKeysPerOwner: 25}), false)
	require.NoError(t, err)
	require.Len(t, manifest.Versions[0].Kinds, 1)
	kind := manifest.Versions[0].Kinds[0]
	require.NotNil(t, kind.KV)
	assert.Equal(t, 4096, kind.KV.MaxValueBytes)
	assert.Equal(t, 25, kind.KV.MaxKeysPerOwner)
}

// TestBuildManifestData_KV_Absent verifies a kind without kv has nil KV in the manifest.
func TestBuildManifestData_KV_Absent(t *testing.T) {
	t.Parallel()

	manifest, err := buildManifestData(newKVTestManifest(nil), false)
	require.NoError(t, err)
	require.Len(t, manifest.Versions[0].Kinds, 1)
	kind := manifest.Versions[0].Kinds[0]
	assert.Nil(t, kind.KV, "kind without kv must have nil KV in manifest")
}

// TestManifestGoTemplate_KV_EmptyBlock verifies the Go manifest template emits a KV block
// when a kind has an empty kv declaration.
func TestManifestGoTemplate_KV_EmptyBlock(t *testing.T) {
	t.Parallel()

	generated, err := generateKVManifestGo(t, &codegen.KindKV{})
	require.NoError(t, err)
	assert.Contains(t, generated, "KV: &app.ManifestVersionKindKV{",
		"generated Go must contain a KV block for an empty kv declaration")
}

// TestManifestGoTemplate_KV_WithValues verifies the Go manifest template propagates kv field values.
func TestManifestGoTemplate_KV_WithValues(t *testing.T) {
	t.Parallel()

	generated, err := generateKVManifestGo(t, &codegen.KindKV{MaxValueBytes: 1024, MaxKeysPerOwner: 10})
	require.NoError(t, err)
	assert.Contains(t, generated, "KV: &app.ManifestVersionKindKV{")
	// gofmt aligns struct field values, so allow any whitespace between field name and value
	assert.Regexp(t, `MaxValueBytes:\s+1024`, generated)
	assert.Regexp(t, `MaxKeysPerOwner:\s+10`, generated)
}

// TestManifestGoTemplate_KV_Absent verifies the Go manifest template omits the KV block
// when a kind has no kv declaration.
func TestManifestGoTemplate_KV_Absent(t *testing.T) {
	t.Parallel()

	generated, err := generateKVManifestGo(t, nil)
	require.NoError(t, err)
	assert.False(t, strings.Contains(generated, "KV: &app.ManifestVersionKindKV{"),
		"generated Go must not contain a KV block when kv is absent")
}

// newKVTestManifest builds a minimal AppManifest for KV codegen tests. It has a single
// version "v1" with a single kind carrying the given kv config.
func newKVTestManifest(kv *codegen.KindKV) codegen.AppManifest {
	schema := cuecontext.New().CompileString(`spec: { field: string }`)
	return &codegen.SimpleManifest{
		AppManifestProperties: codegen.AppManifestProperties{
			AppName:          "kv-test",
			FullGroup:        "kv-test.ext.grafana.app",
			PreferredVersion: "v1",
		},
		AllVersions: map[string]*codegen.SimpleVersion{
			"v1": {
				VersionProperties: codegen.VersionProperties{Name: "v1", Served: true},
				AllKinds: []codegen.VersionedKind{{
					Kind:         "KVKind",
					PluralName:   "KVKinds",
					Scope:        "Namespaced",
					FolderScoped: true,
					Schema:       schema,
					KV:           kv,
				}},
			},
		},
	}
}

// generateKVManifestGo is a helper that runs the ManifestGoGenerator jenny on a minimal
// manifest and returns the generated file content as a string.
func generateKVManifestGo(t *testing.T, kv *codegen.KindKV) (string, error) {
	t.Helper()
	g := &ManifestGoGenerator{
		Package:            "manifestdata",
		SkipImportsProcess: true,
	}
	files, err := g.Generate(newKVTestManifest(kv))
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	return string(files[0].Data), nil
}
