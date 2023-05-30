package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicMetadataObject_CommonMetadata(t *testing.T) {
	obj := BasicMetadataObject{
		CommonMeta: CommonMetadata{
			ResourceVersion: "abc",
			Labels: map[string]string{
				"foo": "bar",
			},
			ExtraFields: map[string]any{
				"extra": "field",
			},
		},
	}
	assert.Equal(t, obj.CommonMeta, obj.CommonMetadata())
}

func TestBasicMetadataObject_SetCommonMetadata(t *testing.T) {
	obj := BasicMetadataObject{}
	assert.Equal(t, CommonMetadata{}, obj.CommonMetadata())
	newMeta := CommonMetadata{
		ResourceVersion: "abc",
		Labels: map[string]string{
			"foo": "bar",
		},
		ExtraFields: map[string]any{
			"extra": "field",
		},
	}
	obj.SetCommonMetadata(newMeta)
	assert.Equal(t, newMeta, obj.CommonMetadata())
}

func TestBasicMetadataObject_StaticMetadata(t *testing.T) {
	obj := BasicMetadataObject{
		StaticMeta: StaticMetadata{
			Group:     "group",
			Version:   "ver",
			Kind:      "kind",
			Namespace: "ns",
			Name:      "test",
		},
	}
	assert.Equal(t, obj.StaticMeta, obj.StaticMetadata())
}

func TestBasicMetadataObject_SetStaticMetadata(t *testing.T) {
	obj := BasicMetadataObject{}
	assert.Equal(t, StaticMetadata{}, obj.StaticMetadata())
	newMeta := StaticMetadata{
		Group:     "group",
		Version:   "ver",
		Kind:      "kind",
		Namespace: "ns",
		Name:      "test",
	}
	obj.SetStaticMetadata(newMeta)
	assert.Equal(t, newMeta, obj.StaticMetadata())
}

func TestBasicMetadataObject_CustomMetadata(t *testing.T) {
	obj := BasicMetadataObject{
		CustomMeta: SimpleCustomMetadata{
			"foo":    "bar",
			"foobar": "baz",
		},
	}
	assert.Equal(t, obj.CustomMeta, obj.CustomMetadata())
}

func TestSimpleObject_Copy(t *testing.T) {
	type spec struct {
		foo string
		bar struct {
			next int
		}
	}
	orig := SimpleObject[spec]{}
	orig.Spec.foo = "foo"
	orig.Spec.bar.next = 2

	objCopy := orig.Copy()
	cast, ok := objCopy.(*SimpleObject[spec])
	assert.True(t, ok)
	assert.Equal(t, orig, *cast)
}

func TestSimpleObject_SpecObject(t *testing.T) {
	type spec struct {
		foo string
	}
	obj := SimpleObject[spec]{}
	obj.Spec = spec{
		foo: "bar",
	}
	assert.Equal(t, obj.Spec, obj.SpecObject())
}

func TestSimpleObject_Subresources(t *testing.T) {
	obj := SimpleObject[any]{}
	obj.SubresourceMap = map[string]any{
		"foo": struct {
			bar int
		}{
			bar: 2,
		},
	}
	assert.Equal(t, obj.SubresourceMap, obj.Subresources())
}

func TestSimpleList_Items(t *testing.T) {
	list := SimpleList[*SimpleObject[string]]{
		Items: []*SimpleObject[string]{
			{
				Spec: "foo",
			},
		},
	}
	items := list.ListItems()
	assert.Len(t, items, 1)
	cast, ok := items[0].(*SimpleObject[string])
	assert.True(t, ok)
	assert.Equal(t, "foo", cast.Spec)
}

func TestSimpleList_SetItems(t *testing.T) {
	list := SimpleList[*SimpleObject[string]]{}
	assert.Len(t, list.ListItems(), 0)
	list.SetItems([]Object{
		&SimpleObject[string]{
			Spec: "foo",
		},
	})
	items := list.ListItems()
	assert.Len(t, items, 1)
	cast, ok := items[0].(*SimpleObject[string])
	assert.True(t, ok)
	assert.Equal(t, "foo", cast.Spec)
}

func TestSimpleList_ListMetadata(t *testing.T) {
	list := SimpleList[*SimpleObject[string]]{
		ListMeta: ListMetadata{
			ResourceVersion: "abc",
			ExtraFields: map[string]any{
				"a": "b",
			},
		},
	}
	assert.Equal(t, list.ListMeta, list.ListMetadata())
}

func TestSimpleList_SetListMetadata(t *testing.T) {
	list := SimpleList[*SimpleObject[string]]{}
	assert.Equal(t, ListMetadata{}, list.ListMetadata())
	newMeta := ListMetadata{
		ResourceVersion: "abc",
		ExtraFields: map[string]any{
			"a": "b",
		},
	}
	list.SetListMetadata(newMeta)
	assert.Equal(t, newMeta, list.ListMetadata())
}

func TestCopyObject(t *testing.T) {
	type spec struct {
		foo string
		bar struct {
			next int
		}
	}
	orig := SimpleObject[spec]{}
	orig.Spec.foo = "foo"
	orig.Spec.bar.next = 2

	objCopy := CopyObject(&orig)
	cast, ok := objCopy.(*SimpleObject[spec])
	assert.True(t, ok)
	assert.Equal(t, orig, *cast)
}
