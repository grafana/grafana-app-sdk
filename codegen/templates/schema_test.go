package templates

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSchema_Resource(t *testing.T) {
	md := SchemaMetadata{
		Package: "v1", Group: "foo.grafana.app", Version: "v1", Kind: "FooBar",
		Plural: "foo_bars", Scope: "Namespaced", FuncPrefix: "FooBar",
	}

	t.Run("omits WithResource when not set", func(t *testing.T) {
		var b bytes.Buffer
		require.NoError(t, WriteSchema(md, &b))
		assert.Contains(t, b.String(), `resource.WithPlural("foo_bars"), resource.WithScope(`)
		assert.NotContains(t, b.String(), "WithResource")
	})

	t.Run("emits WithResource when set", func(t *testing.T) {
		md := md
		md.Resource = "foo-bars"
		var b bytes.Buffer
		require.NoError(t, WriteSchema(md, &b))
		assert.Contains(t, b.String(), `resource.WithPlural("foo_bars"), resource.WithResource("foo-bars"), resource.WithScope(`)
	})
}
