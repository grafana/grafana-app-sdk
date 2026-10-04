package jennies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestRouteSchemaSet(t *testing.T) {
	str := spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}}
	num := spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"number"}}}

	t.Run("identical schemas from different owners are shared", func(t *testing.T) {
		set := newRouteSchemaSet()
		require.NoError(t, set.add("kind Foo", "Message", str))
		require.NoError(t, set.add("kind Bar", "Message", str))
		assert.Equal(t, str, set.schemas["Message"])
	})

	t.Run("different schemas from different owners collide", func(t *testing.T) {
		set := newRouteSchemaSet()
		require.NoError(t, set.add("kind Foo", "Message", str))
		err := set.add("kind Bar", "Message", num)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"Message"`)
		assert.Contains(t, err.Error(), "kind Foo and kind Bar")
		assert.Contains(t, err.Error(), "move the shared type to the inline openapi.components.schemas section")
	})

	t.Run("version route schemas collide with kind route schemas", func(t *testing.T) {
		set := newRouteSchemaSet()
		require.NoError(t, set.add("kind Foo", "Message", str))
		version := app.ManifestVersion{Name: "v1"}
		version.Routes.Schemas = map[string]spec.Schema{"Message": num} //nolint:staticcheck
		err := buildVersionOpenAPI(&version, set)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kind Foo and version v1 routes")
	})
}
