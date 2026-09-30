package jennies

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

func kvSourcedKind(fields ...codegen.SearchField) codegen.VersionedKind {
	return codegen.VersionedKind{
		Kind:         "Widget",
		PluralName:   "Widgets",
		Scope:        "Namespaced",
		FolderScoped: true,
		Schema:       cuecontext.New().CompileString(`spec: { title: string }`),
		KV:           &codegen.KindKV{},
		SearchFields: fields,
	}
}

func kvField(name, owner, key, path string) codegen.SearchField {
	return codegen.SearchField{
		Name:         name,
		Type:         "int64",
		Capabilities: []string{"sort", "retrieve"},
		Source:       &codegen.SearchFieldSource{KV: &codegen.SearchFieldKVSource{Owner: owner, Key: key, Path: path}},
	}
}

func TestBuildManifestData_SearchFieldKVSource(t *testing.T) {
	t.Parallel()

	vk := kvSourcedKind(
		codegen.SearchField{Name: "title", Path: "spec.title", Type: "string", Capabilities: []string{"filter"}},
		kvField("views_total", "usageinsights.grafana.app", "stats", "views_total"),
		kvField("nested", "other.example.app", "daily/rollup", "totals.views"),
	)
	manifest := &codegen.SimpleManifest{
		AppManifestProperties: codegen.AppManifestProperties{
			AppName:          "kvsrc",
			FullGroup:        "kvsrc.ext.grafana.app",
			PreferredVersion: "v1",
		},
		AllVersions: map[string]*codegen.SimpleVersion{
			"v1": {
				VersionProperties: codegen.VersionProperties{Name: "v1", Served: true},
				AllKinds:          []codegen.VersionedKind{vk},
			},
		},
	}

	data, err := buildManifestData(manifest, false)
	require.NoError(t, err)
	require.Len(t, data.Versions, 1)
	require.Len(t, data.Versions[0].Kinds, 1)
	got := data.Versions[0].Kinds[0].SearchFields
	require.Len(t, got, 3)

	assert.Nil(t, got[0].Source, "a path field must not gain a source")
	assert.Nil(t, got[0].KVSource())
	assert.Equal(t, "spec.title", got[0].Path)

	require.NotNil(t, got[1].Source)
	assert.Equal(t, &app.ManifestVersionKindSearchFieldKVSource{Owner: "usageinsights.grafana.app", Key: "stats", Path: "views_total"}, got[1].KVSource())
	assert.Empty(t, got[1].Path)

	require.NotNil(t, got[2].Source)
	assert.Equal(t, &app.ManifestVersionKindSearchFieldKVSource{Owner: "other.example.app", Key: "daily/rollup", Path: "totals.views"}, got[2].KVSource())

	assert.NoError(t, data.Validate(), "a manifest built from valid kv sources must validate")
}

func TestProcessKindVersion_SearchFieldKVSourceRules(t *testing.T) {
	t.Parallel()

	both := kvField("views_total", "usageinsights.grafana.app", "stats", "views_total")
	both.Path = "spec.title"
	both.Type = "string"
	array := kvField("views_total", "usageinsights.grafana.app", "stats", "views_total")
	array.Array = true

	tests := []struct {
		name        string
		field       codegen.SearchField
		errContains string
	}{
		{name: "path and source are mutually exclusive", field: both, errContains: "path and source are mutually exclusive"},
		{name: "kv source on array field", field: array, errContains: "kv source fields must not be arrays"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := processKindVersion(kvSourcedKind(tt.field), "v1", false)
			require.ErrorContains(t, err, tt.errContains)
		})
	}

	t.Run("kv source without path is accepted", func(t *testing.T) {
		t.Parallel()
		mver, err := processKindVersion(kvSourcedKind(kvField("views_total", "usageinsights.grafana.app", "stats", "views_total")), "v1", false)
		require.NoError(t, err)
		require.Len(t, mver.SearchFields, 1)
		require.NotNil(t, mver.SearchFields[0].KVSource())
	})
}

// kvTriple generates owner/key/path values that satisfy the CUE constraints.
type kvTriple struct{ Owner, Key, Path string }

func (kvTriple) Generate(r *rand.Rand, _ int) reflect.Value {
	pick := func(alphabet string, n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = alphabet[r.Intn(len(alphabet))]
		}
		return string(b)
	}
	const lower = "abcdefghijklmnopqrstuvwxyz0123456789"
	const ident = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_"
	path := pick(ident, 1+r.Intn(8))
	for i := r.Intn(3); i > 0; i-- {
		path += "." + pick(ident, 1+r.Intn(8))
	}
	return reflect.ValueOf(kvTriple{
		Owner: pick(lower+".-", 1+r.Intn(20)),
		Key:   pick(lower, 1) + pick(lower+"_.-", r.Intn(20)),
		Path:  path,
	})
}

// For any valid kv source, the manifest carries exactly the declared values.
func TestProcessKindVersion_KVSourceCopiedVerbatim_Property(t *testing.T) {
	t.Parallel()

	prop := func(tr kvTriple) bool {
		mver, err := processKindVersion(kvSourcedKind(kvField("f", tr.Owner, tr.Key, tr.Path)), "v1", false)
		if err != nil || len(mver.SearchFields) != 1 {
			return false
		}
		got := mver.SearchFields[0].KVSource()
		return got != nil && *got == app.ManifestVersionKindSearchFieldKVSource{Owner: tr.Owner, Key: tr.Key, Path: tr.Path}
	}
	require.NoError(t, quick.Check(prop, &quick.Config{MaxCount: 200}))
}
