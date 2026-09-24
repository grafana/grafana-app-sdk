package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestManifestVersionKind_HasKV verifies the HasKV sentinel: only a non-nil KV pointer
// constitutes an opt-in, regardless of whether the struct carries values.
func TestManifestVersionKind_HasKV(t *testing.T) {
	t.Parallel()

	t.Run("nil KV returns false", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{}
		assert.False(t, k.HasKV())
	})

	t.Run("empty KV block returns true", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{KV: &ManifestVersionKindKV{}}
		assert.True(t, k.HasKV())
	})

	t.Run("KV with MaxValueBytes returns true", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{KV: &ManifestVersionKindKV{MaxValueBytes: 1024}}
		assert.True(t, k.HasKV())
	})

	t.Run("KV with MaxKeysPerOwner returns true", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{KV: &ManifestVersionKindKV{MaxKeysPerOwner: 50}}
		assert.True(t, k.HasKV())
	})
}

// TestManifestVersionKindKV_JSONRoundTrip verifies that:
//   - a nil KV is omitted from JSON output (omitempty)
//   - an empty `kv: {}` block survives the round-trip and still signals HasKV
//   - a populated KV block survives the round-trip
func TestManifestVersionKindKV_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("nil KV is omitted from JSON", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{Kind: "Foo", Scope: "Namespaced"}
		data, err := json.Marshal(k)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"kv"`, "kv field must be absent when nil")
	})

	t.Run("empty kv block round-trips and HasKV is true", func(t *testing.T) {
		t.Parallel()
		orig := ManifestVersionKind{
			Kind:  "Foo",
			Scope: "Namespaced",
			KV:    &ManifestVersionKindKV{},
		}
		data, err := json.Marshal(orig)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"kv"`, "kv field must be present when empty struct")

		var got ManifestVersionKind
		require.NoError(t, json.Unmarshal(data, &got))
		require.NotNil(t, got.KV, "KV must be non-nil after round-trip")
		assert.True(t, got.HasKV())
		assert.Equal(t, 0, got.KV.MaxValueBytes)
		assert.Equal(t, 0, got.KV.MaxKeysPerOwner)
	})

	t.Run("populated kv block round-trips", func(t *testing.T) {
		t.Parallel()
		orig := ManifestVersionKind{
			Kind:  "Foo",
			Scope: "Namespaced",
			KV: &ManifestVersionKindKV{
				MaxValueBytes:   1024,
				MaxKeysPerOwner: 50,
			},
		}
		data, err := json.Marshal(orig)
		require.NoError(t, err)

		var got ManifestVersionKind
		require.NoError(t, json.Unmarshal(data, &got))
		require.NotNil(t, got.KV)
		assert.True(t, got.HasKV())
		assert.Equal(t, 1024, got.KV.MaxValueBytes)
		assert.Equal(t, 50, got.KV.MaxKeysPerOwner)
	})
}

// TestManifestVersionKindKV_YAMLRoundTrip mirrors the JSON round-trip checks for YAML.
func TestManifestVersionKindKV_YAMLRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("nil KV is omitted from YAML", func(t *testing.T) {
		t.Parallel()
		k := ManifestVersionKind{Kind: "Foo", Scope: "Namespaced"}
		data, err := yaml.Marshal(k)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "kv:", "kv field must be absent when nil")
	})

	t.Run("empty kv block round-trips and HasKV is true", func(t *testing.T) {
		t.Parallel()
		orig := ManifestVersionKind{
			Kind:  "Foo",
			Scope: "Namespaced",
			KV:    &ManifestVersionKindKV{},
		}
		data, err := yaml.Marshal(orig)
		require.NoError(t, err)
		assert.Contains(t, string(data), "kv:", "kv field must be present when empty struct")

		var got ManifestVersionKind
		require.NoError(t, yaml.Unmarshal(data, &got))
		require.NotNil(t, got.KV, "KV must be non-nil after round-trip")
		assert.True(t, got.HasKV())
	})

	t.Run("populated kv block round-trips", func(t *testing.T) {
		t.Parallel()
		orig := ManifestVersionKind{
			Kind:  "Foo",
			Scope: "Namespaced",
			KV: &ManifestVersionKindKV{
				MaxValueBytes:   2048,
				MaxKeysPerOwner: 200,
			},
		}
		data, err := yaml.Marshal(orig)
		require.NoError(t, err)

		var got ManifestVersionKind
		require.NoError(t, yaml.Unmarshal(data, &got))
		require.NotNil(t, got.KV)
		assert.True(t, got.HasKV())
		assert.Equal(t, 2048, got.KV.MaxValueBytes)
		assert.Equal(t, 200, got.KV.MaxKeysPerOwner)
	})
}

// TestManifestData_ValidateKV checks the boundary conditions for validateKV.
func TestManifestData_ValidateKV(t *testing.T) {
	t.Parallel()

	makeManifest := func(kv *ManifestVersionKindKV) ManifestData {
		return ManifestData{
			Versions: []ManifestVersion{{
				Name:   "v1",
				Served: true,
				Kinds: []ManifestVersionKind{{
					Kind:  "Foo",
					Scope: "Namespaced",
					KV:    kv,
				}},
			}},
		}
	}

	tests := []struct {
		name    string
		kv      *ManifestVersionKindKV
		wantErr bool
		errMsgs []string
	}{
		{
			name:    "nil KV passes",
			kv:      nil,
			wantErr: false,
		},
		{
			name:    "empty KV block (zero values) passes",
			kv:      &ManifestVersionKindKV{},
			wantErr: false,
		},
		{
			name:    "maxValueBytes 1 passes",
			kv:      &ManifestVersionKindKV{MaxValueBytes: 1},
			wantErr: false,
		},
		{
			name:    "large maxValueBytes passes (platform enforces the upper bound)",
			kv:      &ManifestVersionKindKV{MaxValueBytes: 1 << 20},
			wantErr: false,
		},
		{
			name:    "maxValueBytes negative is rejected",
			kv:      &ManifestVersionKindKV{MaxValueBytes: -1},
			wantErr: true,
			errMsgs: []string{"kv.maxValueBytes"},
		},
		{
			name:    "maxKeysPerOwner 1 passes",
			kv:      &ManifestVersionKindKV{MaxKeysPerOwner: 1},
			wantErr: false,
		},
		{
			name:    "maxKeysPerOwner negative is rejected",
			kv:      &ManifestVersionKindKV{MaxKeysPerOwner: -1},
			wantErr: true,
			errMsgs: []string{"kv.maxKeysPerOwner"},
		},
		{
			name:    "both valid passes",
			kv:      &ManifestVersionKindKV{MaxValueBytes: 1024, MaxKeysPerOwner: 100},
			wantErr: false,
		},
		{
			name:    "both invalid reports both errors",
			kv:      &ManifestVersionKindKV{MaxValueBytes: -1, MaxKeysPerOwner: -1},
			wantErr: true,
			errMsgs: []string{"kv.maxValueBytes", "kv.maxKeysPerOwner"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := makeManifest(tc.kv)
			err := m.Validate()
			if tc.wantErr {
				require.Error(t, err)
				for _, msg := range tc.errMsgs {
					assert.Contains(t, err.Error(), msg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestManifestData_ValidateKV_MultipleVersions checks that KV is validated
// consistently across multiple versions of the same kind.
func TestManifestData_ValidateKV_MultipleVersions(t *testing.T) {
	t.Parallel()
	m := ManifestData{
		Versions: []ManifestVersion{
			{
				Name:   "v1",
				Served: true,
				Kinds: []ManifestVersionKind{{
					Kind:  "Foo",
					Scope: "Namespaced",
					KV:    &ManifestVersionKindKV{MaxValueBytes: 1024},
				}},
			},
			{
				Name:   "v2",
				Served: true,
				Kinds: []ManifestVersionKind{{
					Kind:  "Foo",
					Scope: "Namespaced",
					KV:    &ManifestVersionKindKV{MaxValueBytes: -1},
				}},
			},
		},
	}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kv.maxValueBytes")
}
