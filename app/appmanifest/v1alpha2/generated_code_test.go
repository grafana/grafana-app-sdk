package v1alpha2

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestAppManifestKind_Read(t *testing.T) {
	kind := AppManifestKind()

	// Valid JSON
	file, err := os.ReadFile(filepath.Join("testfiles", "manifest-01.json"))
	require.Nil(t, err)
	obj, err := kind.Read(bytes.NewReader(file), resource.KindEncodingJSON)
	require.Nil(t, err)
	cast, ok := obj.(*AppManifest)
	require.True(t, ok, "read object is not of type *AppManifest")
	tm, _ := time.Parse(time.RFC3339, "2025-02-19T20:36:00Z")
	schemaBytes, err := os.ReadFile(filepath.Join("testfiles", "schema-01.json"))
	require.Nil(t, err)
	schema := make(map[string]interface{})
	require.Nil(t, json.Unmarshal(schemaBytes, &schema))
	plural := "issues"
	served := true
	preferred := "v1"
	assert.Equal(t, &AppManifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AppManifest",
			APIVersion: "apps.grafana.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "issue-tracker-project",
			CreationTimestamp: metav1.NewTime(tm.Local()),
		},
		Spec: AppManifestSpec{
			AppName:          "issue-tracker-project",
			Group:            "issuetrackerproject.ext.grafana.com",
			PreferredVersion: &preferred,
			Versions: []AppManifestManifestVersion{{
				Name:   "v1",
				Served: &served,
				Kinds: []AppManifestManifestVersionKind{{
					Kind:    "Issue",
					Plural:  &plural,
					Scope:   "Namespaced",
					Schemas: schema,
				}},
			}},
		},
		Status: AppManifestStatus{
			Resources: map[string]AppManifeststatusApplyStatus{
				"crds": {
					Status: AppManifestStatusApplyStatusStatusSuccess,
				},
			},
		},
	}, cast)
}

func TestAppManifestKind_Write(t *testing.T) {
	kind := AppManifestKind()

	// Valid JSON
	file, err := os.ReadFile(filepath.Join("testfiles", "manifest-01.json"))
	require.Nil(t, err)
	obj, err := kind.Read(bytes.NewReader(file), resource.KindEncodingJSON)
	require.Nil(t, err)
	cast, ok := obj.(*AppManifest)
	require.True(t, ok, "read object is not of type *AppManifest")
	out := bytes.Buffer{}
	err = kind.Write(cast, &out, resource.KindEncodingJSON)
	require.Nil(t, err)
	assert.JSONEq(t, string(file), out.String())
}

func TestAppManifestKind_GroupVersionKind(t *testing.T) {
	kind := AppManifestKind()
	assert.Equal(t, "apps.grafana.com", kind.GroupVersionKind().Group)
	assert.Equal(t, "v1alpha2", kind.GroupVersionKind().Version)
	assert.Equal(t, "AppManifest", kind.GroupVersionKind().Kind)
}

func TestAppManifestKind_GroupVersionResource(t *testing.T) {
	kind := AppManifestKind()
	assert.Equal(t, "apps.grafana.com", kind.GroupVersionResource().Group)
	assert.Equal(t, "v1alpha2", kind.GroupVersionResource().Version)
	assert.Equal(t, "appmanifests", kind.GroupVersionResource().Resource)
}
