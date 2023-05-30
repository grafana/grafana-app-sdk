package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSimpleSchema(t *testing.T) {
	t.Run("no options", func(t *testing.T) {
		sch := NewSimpleSchema("g", "v", &SimpleObject[any]{})
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "SimpleObject[interface {}]", sch.Kind())
		assert.Equal(t, "simpleobject[interface {}]s", sch.Plural())
		assert.Equal(t, &SimpleObject[any]{}, sch.ZeroValue())
	})

	t.Run("with kind", func(t *testing.T) {
		sch := NewSimpleSchema("g", "v", &SimpleObject[any]{}, WithKind("Obj"))
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "Obj", sch.Kind())
		assert.Equal(t, "objs", sch.Plural())
		assert.Equal(t, &SimpleObject[any]{}, sch.ZeroValue())
	})

	t.Run("with plural", func(t *testing.T) {
		sch := NewSimpleSchema("g", "v", &SimpleObject[any]{}, WithKind("Obj"), WithPlural("plural"))
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "Obj", sch.Kind())
		assert.Equal(t, "plural", sch.Plural())
		assert.Equal(t, &SimpleObject[any]{}, sch.ZeroValue())
	})
}

func TestSimpleSchema_ZeroValue(t *testing.T) {
	// Test that the ZeroValue returns a copy of the original, not the original
	orig := SimpleObject[string]{
		Spec: "a",
	}
	sch := NewSimpleSchema("g", "v", &orig)
	zv := sch.ZeroValue()
	cast, ok := zv.(*SimpleObject[string])
	assert.True(t, ok)
	assert.Equal(t, orig, *cast)
	cast.Spec = "b"
	assert.NotEqual(t, orig, *cast)
}

func TestNewSimpleSchemaGroup(t *testing.T) {
	g := NewSimpleSchemaGroup("g", "v")
	assert.Equal(t, "g", g.group)
	assert.Equal(t, "v", g.version)
}

func TestSimpleSchemaGroup_AddSchema(t *testing.T) {
	g := NewSimpleSchemaGroup("g", "v")
	t.Run("no options", func(t *testing.T) {
		sch := g.AddSchema(&SimpleObject[string]{})
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "SimpleObject[string]", sch.Kind())
		assert.Equal(t, "simpleobject[string]s", sch.Plural())
		assert.Equal(t, &SimpleObject[string]{}, sch.ZeroValue())
	})

	t.Run("with kind", func(t *testing.T) {
		sch := g.AddSchema(&SimpleObject[string]{}, WithKind("Obj"))
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "Obj", sch.Kind())
		assert.Equal(t, "objs", sch.Plural())
		assert.Equal(t, &SimpleObject[string]{}, sch.ZeroValue())
	})

	t.Run("with plural", func(t *testing.T) {
		sch := g.AddSchema(&SimpleObject[string]{}, WithKind("Obj"), WithPlural("other"))
		assert.Equal(t, "g", sch.Group())
		assert.Equal(t, "v", sch.Version())
		assert.Equal(t, "Obj", sch.Kind())
		assert.Equal(t, "other", sch.Plural())
		assert.Equal(t, &SimpleObject[string]{}, sch.ZeroValue())
	})
}

func TestSimpleSchemaGroup_Schemas(t *testing.T) {
	g := NewSimpleSchemaGroup("g", "v")
	s1 := g.AddSchema(&SimpleObject[int]{})
	s2 := g.AddSchema(&SimpleObject[string]{})
	schemas := g.Schemas()
	assert.Len(t, schemas, 2)
	assert.Equal(t, s1, schemas[0])
	assert.Equal(t, s2, schemas[1])
}
