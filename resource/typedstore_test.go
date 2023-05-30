package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTypedStore(t *testing.T) {
	schema := NewSimpleSchema("g", "v", &SimpleObject[int]{})
	t.Run("type mismatch", func(t *testing.T) {
		store, err := NewTypedStore[*SimpleObject[string]](schema, &mockClientGenerator{})
		assert.Nil(t, store)
		assert.Equal(t, fmt.Errorf("underlying types of schema.ZeroValue() and provided ObjectType are not the same (SimpleObject[int] != SimpleObject[string])"), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("SOY ERROR")
		generator := &mockClientGenerator{
			ClientForFunc: func(s Schema) (Client, error) {
				assert.Equal(t, schema, s)
				return nil, cerr
			},
		}
		store, err := NewTypedStore[*SimpleObject[int]](schema, generator)
		assert.Nil(t, store)
		assert.Equal(t, fmt.Errorf("error getting client from generator: %w", cerr), err)
	})

	t.Run("success", func(t *testing.T) {
		client := &mockClient{}
		generator := &mockClientGenerator{
			ClientForFunc: func(s Schema) (Client, error) {
				assert.Equal(t, schema, s)
				return client, nil
			},
		}
		store, err := NewTypedStore[*SimpleObject[int]](schema, generator)
		assert.Nil(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, client, store.client)
	})
}

func TestTypedStore_Get(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil, cerr
		}
		ret, err := store.Get(ctx, id)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		obj := &SimpleObject[string]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Name: "test",
				},
				CommonMeta: CommonMetadata{
					ResourceVersion: "123",
				},
			},
			Spec: "foo",
		}
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return obj, nil
		}
		ret, err := store.Get(ctx, id)
		assert.Nil(t, err)
		assert.Equal(t, obj, ret)
	})
}

func TestTypedStore_Add(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	addObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Namespace: "ns",
				Name:      "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "abc",
			},
		},
		Spec: "bar",
	}
	retObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Name: "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "123",
			},
		},
		Spec: "foo",
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.CreateFunc = func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Add(ctx, addObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, addObj.StaticMeta.Name, identifier.Name)
			assert.Equal(t, addObj.StaticMeta.Namespace, identifier.Namespace)
			assert.Equal(t, addObj, obj)
			return retObj, nil
		}
		ret, err := store.Add(ctx, addObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
}

func TestTypedStore_Update(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	updateObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Namespace: "ns",
				Name:      "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "abc",
			},
		},
		Spec: "bar",
	}
	retObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Name: "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "123",
			},
		},
		Spec: "foo",
	}
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Update(ctx, id, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, updateObj.StaticMeta.Name, identifier.Name)
			assert.Equal(t, updateObj.StaticMeta.Namespace, identifier.Namespace)
			assert.Equal(t, updateObj, obj)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, updateObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
}

func TestTypedStore_UpdateSubresource(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	updateObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Namespace: "ns",
				Name:      "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "abc",
			},
		},
		SubresourceMap: map[string]any{
			string(SubresourceStatus): "foo",
		},
	}
	retObj := &SimpleObject[string]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Name: "test",
			},
			CommonMeta: CommonMetadata{
				ResourceVersion: "123",
			},
		},
		Spec: "foo",
	}
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.UpdateSubresource(ctx, id, SubresourceStatus, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, updateObj, obj)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, string(SubresourceStatus), options.Subresource)
			return retObj, nil
		}
		ret, err := store.UpdateSubresource(ctx, id, SubresourceStatus, updateObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
}

func TestTypedStore_Delete(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.DeleteFunc = func(c context.Context, identifier Identifier) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return cerr
		}
		err := store.Delete(ctx, id)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		client.DeleteFunc = func(c context.Context, identifier Identifier) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil
		}
		err := store.Delete(ctx, id)
		assert.Nil(t, err)
	})
}

func TestTypedStore_List(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	ns := "ns"
	list := &SimpleList[*SimpleObject[string]]{
		Items: []*SimpleObject[string]{
			{
				Spec: "a",
			},
			{
				Spec: "b",
			},
		},
	}

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.ListFunc = func(ctx context.Context, namespace string, options ListOptions) (ListObject, error) {
			return nil, cerr
		}
		ret, err := store.List(ctx, ns)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			return list, nil
		}
		ret, err := store.List(ctx, ns)
		assert.Nil(t, err)
		assert.Equal(t, len(list.ListItems()), len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i].StaticMeta, ret.Items[i].StaticMeta)
			assert.Equal(t, list.Items[i].CommonMeta, ret.Items[i].CommonMeta)
			assert.Equal(t, list.Items[i].SubresourceMap, ret.Items[i].SubresourceMap)
		}
	})

	t.Run("success, with filters", func(t *testing.T) {
		filters := []string{"a", "b"}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, filters, options.LabelFilters)
			return list, nil
		}
		ret, err := store.List(ctx, ns, filters...)
		assert.Nil(t, err)
		assert.Equal(t, len(list.ListItems()), len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i].StaticMeta, ret.Items[i].StaticMeta)
			assert.Equal(t, list.Items[i].CommonMeta, ret.Items[i].CommonMeta)
			assert.Equal(t, list.Items[i].SubresourceMap, ret.Items[i].SubresourceMap)
		}
	})
}

func getTypedStoreTestSetup() (*TypedStore[*SimpleObject[string]], *mockClient) {
	client := &mockClient{}
	generator := &mockClientGenerator{
		ClientForFunc: func(schema Schema) (Client, error) {
			return client, nil
		},
	}
	schema := NewSimpleSchema("g", "v", &SimpleObject[string]{})
	store, _ := NewTypedStore[*SimpleObject[string]](schema, generator)
	return store, client
}
