package resource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestCatalog struct {
	A string `json:"a"`
	B int    `json:"b"`
}

func TestTypedObject_GetSubresources(t *testing.T) {
	t.Run("TestCatalog", func(t *testing.T) {
		o := TypedObject[any, TestCatalog]{
			Subresources: TestCatalog{
				A: "foo",
				B: 2,
			},
		}
		sr := o.GetSubresources()
		assert.Equal(t, map[string]any{
			"a": o.Subresources.A,
			"b": o.Subresources.B,
		}, sr)
	})

	t.Run("map[int]any", func(t *testing.T) {
		o := TypedObject[any, map[int]any]{
			Subresources: map[int]any{
				2: "foo",
			},
		}
		// This isn't a valid use-case, so the returned data isn't valid
		sr := o.GetSubresources()
		assert.Equal(t, map[string]any{
			"<int Value>": "foo",
		}, sr)
	})

	t.Run("map[string]int", func(t *testing.T) {
		o := TypedObject[any, map[string]int]{
			Subresources: map[string]int{
				"foo": 2,
				"bar": 4,
			},
		}
		sr := o.GetSubresources()
		assert.Equal(t, map[string]any{
			"foo": 2,
			"bar": 4,
		}, sr)
	})

	t.Run("map[string]testCatalog", func(t *testing.T) {
		o := TypedObject[any, map[string]TestCatalog]{
			Subresources: map[string]TestCatalog{
				"foo": {
					A: "foo",
					B: 2,
				},
				"bar": {
					A: "bar",
					B: 4,
				},
			},
		}
		sr := o.GetSubresources()
		assert.Equal(t, map[string]any{
			"foo": o.Subresources["foo"],
			"bar": o.Subresources["bar"],
		}, sr)
	})

	t.Run("invalid types", func(t *testing.T) {
		o := TypedObject[any, int]{}
		sr := o.GetSubresources()
		assert.Nil(t, sr)

		o2 := TypedObject[any, map[int]any]{}
		sr = o2.GetSubresources()
		assert.Equal(t, map[string]any{}, sr)
	})
}

func TestTypedObject_GetSubresource(t *testing.T) {
	t.Run("TestCatalog", func(t *testing.T) {
		o := TypedObject[any, TestCatalog]{
			Subresources: TestCatalog{
				A: "foo",
				B: 2,
			},
		}
		val, ok := o.GetSubresource("c")
		assert.Nil(t, val)
		assert.False(t, ok)

		val, ok = o.GetSubresource("a")
		assert.Equal(t, o.Subresources.A, val)
		assert.True(t, ok)

		val, ok = o.GetSubresource("b")
		assert.Equal(t, o.Subresources.B, val)
		assert.True(t, ok)
	})

	t.Run("map[int]any", func(t *testing.T) {
		o := TypedObject[any, map[int]any]{
			Subresources: map[int]any{
				2: "foo",
			},
		}
		val, ok := o.GetSubresource("2")
		assert.Nil(t, val)
		assert.False(t, ok)
	})

	t.Run("map[string]int", func(t *testing.T) {
		o := TypedObject[any, map[string]int]{
			Subresources: map[string]int{
				"foo": 2,
			},
		}
		val, ok := o.GetSubresource("bar")
		assert.Nil(t, val)
		assert.False(t, ok)

		val, ok = o.GetSubresource("foo")
		assert.Equal(t, o.Subresources["foo"], val)
		assert.True(t, ok)
	})

	t.Run("map[string]testCatalog", func(t *testing.T) {
		o := TypedObject[any, map[string]TestCatalog]{
			Subresources: map[string]TestCatalog{
				"foo": {
					A: "foo",
					B: 2,
				},
			},
		}
		val, ok := o.GetSubresource("bar")
		assert.Nil(t, val)
		assert.False(t, ok)

		val, ok = o.GetSubresource("foo")
		assert.Equal(t, o.Subresources["foo"], val)
		assert.True(t, ok)
	})

	t.Run("invalid types", func(t *testing.T) {
		o := TypedObject[any, int]{}
		val, ok := o.GetSubresource("2")
		assert.Nil(t, val)
		assert.False(t, ok)

		o2 := TypedObject[any, map[int]any]{}
		val, ok = o2.GetSubresource("2")
		assert.Nil(t, val)
		assert.False(t, ok)
	})
}

func TestTypedObject_SetSubresource(t *testing.T) {
	t.Run("TestCatalog", func(t *testing.T) {
		o := TypedObject[any, TestCatalog]{}
		assert.Equal(t, fmt.Errorf("subresource 'c' does not exist"), o.SetSubresource("c", "foo"))

		assert.Nil(t, o.SetSubresource("a", "foo"))
		assert.Equal(t, "foo", o.Subresources.A)

		assert.Equal(t, fmt.Errorf("cannot assign value of type string to subresource 'b' of type int"), o.SetSubresource("b", "foo"))

		assert.Nil(t, o.SetSubresource("b", 2))
		assert.Equal(t, 2, o.Subresources.B)
	})

	t.Run("map[int]any", func(t *testing.T) {
		o := TypedObject[any, map[int]any]{}
		assert.Equal(t, fmt.Errorf("subresource map has an unassignable key type of 'int'"), o.SetSubresource("foo", "bar"))
	})

	t.Run("map[string]int", func(t *testing.T) {
		o := TypedObject[any, map[string]int]{}
		assert.Equal(t, fmt.Errorf("subresource map requires a value of type 'int', provided value 'string' is not assignable"), o.SetSubresource("foo", "bar"))

		assert.Nil(t, o.SetSubresource("foo", 1))
		assert.Equal(t, 1, o.Subresources["foo"])
	})

	t.Run("map[string]testCatalog", func(t *testing.T) {
		o := TypedObject[any, map[string]TestCatalog]{}
		assert.Equal(t, fmt.Errorf("subresource map requires a value of type 'resource.TestCatalog', provided value 'string' is not assignable"), o.SetSubresource("foo", "bar"))

		val := TestCatalog{}
		assert.Nil(t, o.SetSubresource("foo", val))
		assert.Equal(t, val, o.Subresources["foo"])
	})

	t.Run("invalid types", func(t *testing.T) {
		o := TypedObject[any, int]{}
		assert.Equal(t, fmt.Errorf("subresource 'foo' does not exist"), o.SetSubresource("foo", "bar"))

		o2 := TypedObject[any, map[int]any]{}
		assert.Equal(t, fmt.Errorf("subresource map has an unassignable key type of 'int'"), o2.SetSubresource("1", "bar"))
	})
}
