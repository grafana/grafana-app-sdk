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
					Schemas: map[string]interface{}{
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
