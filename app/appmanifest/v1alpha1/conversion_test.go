package v1alpha1

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppManifestSpec_ToManifestData(t *testing.T) {
	t.Run("successful conversion", func(t *testing.T) {
		// For v1alpha1, app.ManifestData is essentially a subset of v1alpha1.AppManifestSpec,
		// so we only need to check that the same JSON loaded for the AppManifestSpec and using ToManifestData()
		// is identical to loading that JSON for app.ManifestData
		file, err := os.ReadFile(filepath.Join("testfiles", "spec-01.json"))
		require.Nil(t, err)
		v1alpha1 := AppManifestSpec{}
		md := app.ManifestData{}
		err = json.Unmarshal(file, &v1alpha1)
		require.Nil(t, err)
		err = json.Unmarshal(file, &md)
		require.Nil(t, err)
		v1md, err := v1alpha1.ToManifestData()
		require.Nil(t, err)
		assert.Equal(t, md, v1md)
	})

	t.Run("bad schema data", func(t *testing.T) {
		v1alpha1 := AppManifestSpec{
			Kinds: []AppManifestManifestKind{{
				Versions: []AppManifestManifestKindVersion{{
					Schema: map[string]interface{}{
						"openAPIV3Schema": "foo", // Bad OpenAPI document, conversion will fail when loading the openAPI
					},
				}},
			}},
		}
		_, err := v1alpha1.ToManifestData()
		assert.Equal(t, errors.New("'openAPIV3Schema' must be an object"), err)
	})
}
