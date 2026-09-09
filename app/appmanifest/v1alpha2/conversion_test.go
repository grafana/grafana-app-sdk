package v1alpha2

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestSearchFieldsConversion(t *testing.T) {
	ptr := func(s string) *string { return &s }
	truePtr := func() *bool { b := true; return &b }

	md := app.ManifestData{
		AppName: "foo",
		Versions: []app.ManifestVersion{{
			Name:   "v1",
			Served: true,
			Kinds: []app.ManifestVersionKind{{
				Kind:  "Foo",
				Scope: "Namespaced",
				SearchFields: []app.ManifestVersionKindSearchField{
					{
						Name:         "email",
						Path:         "spec.email",
						Type:         "string",
						Capabilities: []string{"filter", "text", "retrieve"},
						Description:  "User email",
					},
					{
						Name:             "labels",
						Type:             "int64",
						Array:            true,
						Capabilities:     []string{"filter", "retrieve"},
						EmitZeroIfAbsent: true,
					},
				},
			}},
		}},
	}

	// manifest -> spec: optional scalars become pointers and only the set ones are
	// populated; type and capabilities become the generated enum types.
	spec, err := SpecFromManifestData(md)
	require.NoError(t, err)
	require.Len(t, spec.Versions[0].Kinds[0].SearchFields, 2)
	assert.Equal(t, AppManifestSearchField{
		Name:         "email",
		Path:         ptr("spec.email"),
		Type:         AppManifestSearchFieldTypeString,
		Capabilities: []AppManifestSearchFieldCapabilities{"filter", "text", "retrieve"},
		Description:  ptr("User email"),
	}, spec.Versions[0].Kinds[0].SearchFields[0])
	assert.Equal(t, AppManifestSearchField{
		Name:             "labels",
		Type:             AppManifestSearchFieldTypeInt64,
		Array:            truePtr(),
		Capabilities:     []AppManifestSearchFieldCapabilities{"filter", "retrieve"},
		EmitZeroIfAbsent: truePtr(),
	}, spec.Versions[0].Kinds[0].SearchFields[1])
	encoded, err := json.Marshal(spec.Versions[0].Kinds[0].SearchFields)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"embed"`)

	// spec -> manifest: the set pointers are dereferenced back, unset ones stay zero.
	roundTripped, err := spec.ToManifestData()
	require.NoError(t, err)
	assert.Equal(t, md.Versions[0].Kinds[0].SearchFields, roundTripped.Versions[0].Kinds[0].SearchFields)
}

func TestFolderScopedConversion(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	for _, tc := range []struct {
		name  string
		value *bool
	}{
		{name: "unset", value: nil},
		{name: "explicit true", value: boolPtr(true)},
		{name: "explicit false", value: boolPtr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := app.ManifestData{
				AppName: "foo",
				Versions: []app.ManifestVersion{{
					Name:   "v1",
					Served: true,
					Kinds: []app.ManifestVersionKind{{
						Kind:         "Foo",
						Scope:        "Namespaced",
						FolderScoped: tc.value,
					}},
				}},
			}

			// manifest -> spec: the pointer is copied verbatim.
			spec, err := SpecFromManifestData(md)
			require.NoError(t, err)
			assert.Equal(t, tc.value, spec.Versions[0].Kinds[0].FolderScoped)

			// spec -> manifest: the pointer is copied back verbatim, preserving unset vs explicit.
			roundTripped, err := spec.ToManifestData()
			require.NoError(t, err)
			assert.Equal(t, tc.value, roundTripped.Versions[0].Kinds[0].FolderScoped)
		})
	}
}

func TestAppManifestSpec_ToManifestData(t *testing.T) {
	t.Run("successful conversion", func(t *testing.T) {
		// For v1alpha2, app.ManifestData is essentially a subset of v1alpha2.AppManifestSpec,
		// so we only need to check that the same JSON loaded for the AppManifestSpec and using ToManifestData()
		// is identical to loading that JSON for app.ManifestData
		file, err := os.ReadFile(filepath.Join("testfiles", "spec-01.json"))
		require.Nil(t, err)
		v1alpha2 := AppManifestSpec{}
		md := app.ManifestData{}
		err = json.Unmarshal(file, &v1alpha2)
		require.Nil(t, err)
		err = json.Unmarshal(file, &md)
		schFile, err := os.ReadFile(filepath.Join("testfiles", "schema-01.json"))
		require.Nil(t, err)
		m := make(map[string]any)
		err = json.Unmarshal(schFile, &m)
		require.Nil(t, err)
		md.Versions[0].Kinds[0].Schema, err = app.VersionSchemaFromMap(m, md.Versions[0].Kinds[0].Kind)
		require.Nil(t, err)
		require.Nil(t, err)
		v1md, err := v1alpha2.ToManifestData()
		require.Nil(t, err)
		assert.Equal(t, md, v1md)
	})

	t.Run("bad schema data", func(t *testing.T) {
		v1alpha2 := AppManifestSpec{
			Versions: []AppManifestManifestVersion{{
				Kinds: []AppManifestManifestVersionKind{{
					Kind: "Foo",
					Schemas: map[string]any{
						"bar": "foo", // Bad OpenAPI document, conversion will fail when loading the openAPI
					},
				}},
			}},
		}
		_, err := v1alpha2.ToManifestData()
		assert.Equal(t, errors.New("schemas for Foo must contain an entry named 'Foo'"), err)
	})

	t.Run("no versions", func(t *testing.T) {
		v1alpha2 := AppManifestSpec{
			AppName: "foo",
		}
		md, err := v1alpha2.ToManifestData()
		require.NoError(t, err)
		assert.Equal(t, app.ManifestData{
			AppName:  "foo",
			Versions: []app.ManifestVersion{},
		}, md)
	})

	t.Run("Roles not cast", func(t *testing.T) {
		roleKind1 := AppManifestRoleKindWithPermissionSet{
			Kind:          "Kind1",
			PermissionSet: "viewer",
		}
		roleKind1Json, err := json.Marshal(roleKind1)
		require.Nil(t, err)
		var roleKind1Unmarshaled any
		assert.Nil(t, json.Unmarshal(roleKind1Json, &roleKind1Unmarshaled))
		roleKind2 := AppManifestRoleKindWithVerbs{
			Kind:  "Kind2",
			Verbs: []string{"get", "list"},
		}
		roleKind2Json, err := json.Marshal(roleKind2)
		require.Nil(t, err)
		var roleKind2Unmarshaled any
		assert.Nil(t, json.Unmarshal(roleKind2Json, &roleKind2Unmarshaled))
		v1alpha1 := AppManifestSpec{
			AppName: "foo",
			Roles: map[string]AppManifestRole{
				"foo": {
					Title:       "Foo",
					Description: "Bar",
					Kinds:       []AppManifestRoleKind{roleKind1Unmarshaled, roleKind2Unmarshaled},
				},
			},
		}
		md, err := v1alpha1.ToManifestData()
		require.NoError(t, err)
		permSet := string(roleKind1.PermissionSet)
		assert.Equal(t, app.ManifestData{
			AppName:  "foo",
			Versions: []app.ManifestVersion{},
			Roles: map[string]app.ManifestRole{
				"foo": {
					Title:       "Foo",
					Description: "Bar",
					Kinds: []app.ManifestRoleKind{{
						Kind:          roleKind1.Kind,
						PermissionSet: &permSet,
					}, {
						Kind:  roleKind2.Kind,
						Verbs: roleKind2.Verbs,
					}},
					Routes: []string{},
				},
			},
		}, md)
	})
}

func TestSearchEndpointsConversion(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	for _, tc := range []struct {
		name   string
		search *app.ManifestVersionKindSearch
	}{
		{name: "unset", search: nil},
		{name: "empty", search: &app.ManifestVersionKindSearch{}},
		{name: "search opt-out", search: &app.ManifestVersionKindSearch{Endpoint: boolPtr(false)}},
		{name: "trash opt-out", search: &app.ManifestVersionKindSearch{Trash: boolPtr(false)}},
		{name: "explicit true", search: &app.ManifestVersionKindSearch{Endpoint: boolPtr(true), Trash: boolPtr(true)}},
		{name: "hybrid opt-in", search: &app.ManifestVersionKindSearch{Hybrid: boolPtr(true)}},
		{name: "hybrid explicit false", search: &app.ManifestVersionKindSearch{Hybrid: boolPtr(false)}},
		{name: "hybrid opt-in with search opt-out", search: &app.ManifestVersionKindSearch{Endpoint: boolPtr(false), Hybrid: boolPtr(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := app.ManifestData{
				AppName: "foo",
				Versions: []app.ManifestVersion{{
					Name:   "v1",
					Served: true,
					Kinds: []app.ManifestVersionKind{{
						Kind:   "Foo",
						Scope:  "Namespaced",
						Search: tc.search,
					}},
				}},
			}

			spec, err := SpecFromManifestData(md)
			require.NoError(t, err)

			// spec -> manifest: unset vs explicit is preserved in both directions.
			roundTripped, err := spec.ToManifestData()
			require.NoError(t, err)
			assert.Equal(t, tc.search, roundTripped.Versions[0].Kinds[0].Search)
		})
	}
}

func TestEmbedConversion(t *testing.T) {
	for _, tc := range []struct {
		name           string
		contentVersion int
		embed          *app.ManifestVersionKindEmbed
		wantJSON       string
	}{
		{name: "unset"},
		{
			name:           "initial content version",
			contentVersion: 1,
			embed: &app.ManifestVersionKindEmbed{
				Fields: []app.ManifestVersionKindEmbedField{
					{Name: "title", Path: "spec.title"},
				},
			},
			wantJSON: `{"fields":[{"name":"title","path":"spec.title"}]}`,
		},
		{
			name:           "increased content version with multiple fields",
			contentVersion: 3,
			embed: &app.ManifestVersionKindEmbed{
				Fields: []app.ManifestVersionKindEmbedField{
					{Name: "title", Path: "spec.title"},
					{Name: "tags", Path: "spec.tags"},
				},
			},
			wantJSON: `{"fields":[{"name":"title","path":"spec.title"},{"name":"tags","path":"spec.tags"}]}`,
		},
		{
			name:           "empty fields",
			contentVersion: 1,
			embed: &app.ManifestVersionKindEmbed{
				Fields: []app.ManifestVersionKindEmbedField{},
			},
			wantJSON: `{"fields":[]}`,
		},
		{
			name:           "custom builder without declared fields",
			contentVersion: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := app.ManifestData{
				AppName: "foo",
				Versions: []app.ManifestVersion{{
					Name:   "v1",
					Served: true,
					Kinds: []app.ManifestVersionKind{{
						Kind:  "Foo",
						Scope: "Namespaced",
						Embed: tc.embed,
						SearchFields: []app.ManifestVersionKindSearchField{{
							Name:         "category",
							Path:         "spec.category",
							Type:         "string",
							Capabilities: []string{"filter"},
						}},
					}},
				}},
			}
			if tc.contentVersion != 0 {
				md.Embed = map[string]app.ManifestResourceEmbed{
					"foos": {ContentVersion: tc.contentVersion},
				}
			}

			spec, err := SpecFromManifestData(md)
			require.NoError(t, err)
			kindJSON, err := json.Marshal(spec.Versions[0].Kinds[0])
			require.NoError(t, err)
			var kindFields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(kindJSON, &kindFields))
			if tc.embed == nil {
				assert.NotContains(t, kindFields, "embed")
			} else {
				assert.JSONEq(t, tc.wantJSON, string(kindFields["embed"]))
			}
			assert.NotContains(t, string(kindFields["searchFields"]), `"embed"`)
			encoded, err := json.Marshal(spec)
			require.NoError(t, err)
			var specFields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &specFields))
			if tc.contentVersion == 0 {
				assert.NotContains(t, specFields, "embed")
			} else {
				wantEmbedJSON, err := json.Marshal(md.Embed)
				require.NoError(t, err)
				assert.JSONEq(t, string(wantEmbedJSON), string(specFields["embed"]))
			}
			var decoded AppManifestSpec
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			roundTripped, err := decoded.ToManifestData()
			require.NoError(t, err)
			assert.Equal(t, md.Embed, roundTripped.Embed)
			assert.Equal(t, tc.embed, roundTripped.Versions[0].Kinds[0].Embed)
			assert.Equal(t, md.Versions[0].Kinds[0].SearchFields, roundTripped.Versions[0].Kinds[0].SearchFields)
		})
	}
}

func TestEmbedConversion_SharedContentVersion(t *testing.T) {
	md := app.ManifestData{
		AppName: "testapp",
		Group:   "testapp.grafana.app",
		Embed: map[string]app.ManifestResourceEmbed{
			"foos":     {ContentVersion: 3},
			"children": {ContentVersion: 7},
		},
		Versions: []app.ManifestVersion{
			{
				Name:   "v1",
				Served: true,
				Kinds: []app.ManifestVersionKind{
					{
						Kind:  "Foo",
						Scope: "Namespaced",
						Embed: &app.ManifestVersionKindEmbed{Fields: []app.ManifestVersionKindEmbedField{
							{Name: "title", Path: "spec.title"},
						}},
					},
					{
						Kind:   "Child",
						Plural: "children",
						Scope:  "Namespaced",
					},
				},
			},
			{
				Name:   "v2",
				Served: true,
				Kinds: []app.ManifestVersionKind{
					{
						Kind:  "Foo",
						Scope: "Namespaced",
						Embed: &app.ManifestVersionKindEmbed{Fields: []app.ManifestVersionKindEmbedField{
							{Name: "title", Path: "spec.details.title"},
						}},
					},
					{
						Kind:   "Child",
						Plural: "children",
						Scope:  "Namespaced",
					},
				},
			},
		},
	}

	spec, err := SpecFromManifestData(md)
	require.NoError(t, err)
	encoded, err := json.Marshal(spec)
	require.NoError(t, err)
	var specFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &specFields))
	assert.JSONEq(t, `{"foos":{"contentVersion":3},"children":{"contentVersion":7}}`, string(specFields["embed"]))
	assert.NotContains(t, string(specFields["versions"]), `"contentVersion"`)

	var decoded AppManifestSpec
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	roundTripped, err := decoded.ToManifestData()
	require.NoError(t, err)
	assert.Equal(t, md.Embed, roundTripped.Embed)
	for vi, version := range md.Versions {
		for ki, kind := range version.Kinds {
			assert.Equal(t, kind.Embed, roundTripped.Versions[vi].Kinds[ki].Embed)
		}
	}
}
