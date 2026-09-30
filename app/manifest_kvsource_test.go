package app

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func kvSourceManifest(fields ...ManifestVersionKindSearchField) ManifestData {
	return ManifestData{
		AppName: "kvsrc",
		Group:   "kvsrc.ext.grafana.app",
		Versions: []ManifestVersion{{
			Name:   "v1",
			Served: true,
			Kinds: []ManifestVersionKind{{
				Kind:         "Widget",
				Plural:       "Widgets",
				Scope:        "Namespaced",
				KV:           &ManifestVersionKindKV{},
				SearchFields: fields,
			}},
		}},
	}
}

func kvSourceField(owner, key, path string) ManifestVersionKindSearchField {
	return ManifestVersionKindSearchField{
		Name:         "views_total",
		Type:         "int64",
		Capabilities: []string{"sort", "retrieve"},
		Source: &ManifestVersionKindSearchFieldSource{
			KV: &ManifestVersionKindSearchFieldKVSource{Owner: owner, Key: key, Path: path},
		},
	}
}

func TestManifestVersionKindSearchField_KVSource(t *testing.T) {
	t.Parallel()

	t.Run("no source returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, ManifestVersionKindSearchField{Name: "title", Path: "spec.title"}.KVSource())
	})
	t.Run("source without kv returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, ManifestVersionKindSearchField{Name: "x", Source: &ManifestVersionKindSearchFieldSource{}}.KVSource())
	})
	t.Run("kv source is returned", func(t *testing.T) {
		t.Parallel()
		f := kvSourceField("usageinsights.grafana.app", "stats", "views_total")
		got := f.KVSource()
		require.NotNil(t, got)
		assert.Equal(t, ManifestVersionKindSearchFieldKVSource{Owner: "usageinsights.grafana.app", Key: "stats", Path: "views_total"}, *got)
	})
}

func TestManifestData_Validate_SearchFieldKVSource(t *testing.T) {
	t.Parallel()

	both := kvSourceField("usageinsights.grafana.app", "stats", "views_total")
	both.Path = "spec.title"

	tests := []struct {
		name    string
		field   ManifestVersionKindSearchField
		wantErr bool
	}{
		{name: "valid kv source", field: kvSourceField("usageinsights.grafana.app", "stats", "views_total")},
		{name: "valid path field", field: ManifestVersionKindSearchField{Name: "title", Path: "spec.title", Type: "string", Capabilities: []string{"filter"}}},
		{name: "path and kv source both set", field: both, wantErr: true},
		{name: "empty owner", field: kvSourceField("", "stats", "views_total"), wantErr: true},
		{name: "empty key", field: kvSourceField("usageinsights.grafana.app", "", "views_total"), wantErr: true},
		{name: "empty path", field: kvSourceField("usageinsights.grafana.app", "stats", ""), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := kvSourceManifest(tt.field)
			err := m.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManifestVersionKindSearchField_SourceEncoding(t *testing.T) {
	t.Parallel()

	t.Run("json omits source when nil", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(ManifestVersionKindSearchField{Name: "title", Path: "spec.title", Type: "string"})
		require.NoError(t, err)
		assert.NotContains(t, string(b), "source")
	})
	t.Run("json uses source.kv owner/key/path", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(kvSourceField("o", "k", "p"))
		require.NoError(t, err)
		assert.Contains(t, string(b), `"source":{"kv":{"owner":"o","key":"k","path":"p"}}`)
	})
	t.Run("yaml omits source when nil", func(t *testing.T) {
		t.Parallel()
		b, err := yaml.Marshal(ManifestVersionKindSearchField{Name: "title", Path: "spec.title", Type: "string"})
		require.NoError(t, err)
		assert.NotContains(t, string(b), "source")
	})
}

type kvSourceValues struct{ Owner, Key, Path string }

func (kvSourceValues) Generate(r *rand.Rand, _ int) reflect.Value {
	s := func() string {
		const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789._-/"
		b := make([]byte, 1+r.Intn(24))
		for i := range b {
			b[i] = alphabet[r.Intn(len(alphabet))]
		}
		return string(b)
	}
	return reflect.ValueOf(kvSourceValues{Owner: s(), Key: s(), Path: s()})
}

// For any kv source, JSON and YAML round-trips preserve it exactly.
func TestManifestVersionKindSearchField_SourceRoundTrip_Property(t *testing.T) {
	t.Parallel()

	prop := func(v kvSourceValues) bool {
		in := kvSourceField(v.Owner, v.Key, v.Path)

		jb, err := json.Marshal(in)
		if err != nil {
			return false
		}
		var fromJSON ManifestVersionKindSearchField
		if json.Unmarshal(jb, &fromJSON) != nil || !reflect.DeepEqual(in, fromJSON) {
			return false
		}

		yb, err := yaml.Marshal(in)
		if err != nil {
			return false
		}
		var fromYAML ManifestVersionKindSearchField
		if yaml.Unmarshal(yb, &fromYAML) != nil || !reflect.DeepEqual(in, fromYAML) {
			return false
		}
		return fromJSON.KVSource() != nil && *fromJSON.KVSource() == ManifestVersionKindSearchFieldKVSource(v)
	}
	require.NoError(t, quick.Check(prop, &quick.Config{MaxCount: 200}))
}
