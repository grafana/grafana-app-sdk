package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSimpleStore(t *testing.T) {
	schema := NewSimpleSchema("g", "v", &SimpleObject[int]{})
	t.Run("type mismatch", func(t *testing.T) {
		store, err := NewSimpleStore[string](schema, &mockClientGenerator{})
		assert.Nil(t, store)
		assert.Equal(t, fmt.Errorf("SpecType 'string' does not match underlying schema.ZeroValue().SpecObject() type 'int'"), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("SOY ERROR")
		generator := &mockClientGenerator{
			ClientForFunc: func(s Schema) (Client, error) {
				assert.Equal(t, schema, s)
				return nil, cerr
			},
		}
		store, err := NewSimpleStore[int](schema, generator)
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
		store, err := NewSimpleStore[int](schema, generator)
		assert.Nil(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, client, store.client)
	})
}

func TestSimpleStore_Get(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
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
		assert.Equal(t, obj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, obj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, obj.SpecObject(), ret.Spec)
	})
}

func TestSimpleStore_Add(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
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
		client.CreateFunc = func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Add(ctx, id, "")
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.SpecObject())
			return retObj, nil
		}
		ret, err := store.Add(ctx, id, "test")
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})

	t.Run("success, with metadata options", func(t *testing.T) {
		lbls := map[string]string{
			"field": "value",
		}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.SpecObject())
			assert.Equal(t, lbls, obj.CommonMetadata().Labels)
			return retObj, nil
		}
		ret, err := store.Add(ctx, id, "test", WithLabels(lbls))
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})
}

func TestSimpleStore_Update(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
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
		ret, err := store.Update(ctx, id, "")
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.SpecObject())
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test")
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})

	t.Run("success, with metadata options", func(t *testing.T) {
		lbls := map[string]string{
			"field": "value",
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.SpecObject())
			assert.Equal(t, lbls, obj.CommonMetadata().Labels)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test", WithLabels(lbls))
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})

	t.Run("success, with resourceVersion option", func(t *testing.T) {
		rv := "12345"
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.SpecObject())
			assert.Equal(t, rv, obj.CommonMetadata().ResourceVersion)
			assert.Equal(t, rv, options.ResourceVersion)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test", WithResourceVersion(rv))
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})
}

func TestSimpleStore_UpdateSubresource(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
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

	t.Run("empty subresource", func(t *testing.T) {
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Fail(t, "client.Update should not be called")
			return nil, nil
		}
		ret, err := store.UpdateSubresource(ctx, id, "", "")
		assert.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("subresource may not be empty"), err)
	})

	t.Run("error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.UpdateSubresource(ctx, id, SubresourceStatus, "")
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, string(SubresourceStatus), options.Subresource)
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, 2, obj.Subresources()[string(SubresourceStatus)])
			return retObj, nil
		}
		ret, err := store.UpdateSubresource(ctx, id, SubresourceStatus, 2)
		assert.Nil(t, err)
		assert.Equal(t, retObj.StaticMetadata(), ret.StaticMetadata)
		assert.Equal(t, retObj.CommonMetadata(), ret.CommonMetadata)
		assert.Equal(t, retObj.SpecObject(), ret.Spec)
	})
}

func TestSimpleStore_Delete(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
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

func TestSimpleStore_List(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
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
		assert.Equal(t, len(list.Items), len(ret))
		for i := 0; i < len(ret); i++ {
			assert.Equal(t, list.Items[i].Spec, ret[i].Spec)
			assert.Equal(t, list.Items[i].StaticMeta, ret[i].StaticMetadata)
			assert.Equal(t, list.Items[i].CommonMeta, ret[i].CommonMetadata)
			assert.Equal(t, list.Items[i].SubresourceMap, ret[i].Subresources)
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
		assert.Equal(t, len(list.Items), len(ret))
		for i := 0; i < len(ret); i++ {
			assert.Equal(t, list.Items[i].Spec, ret[i].Spec)
			assert.Equal(t, list.Items[i].StaticMeta, ret[i].StaticMetadata)
			assert.Equal(t, list.Items[i].CommonMeta, ret[i].CommonMetadata)
			assert.Equal(t, list.Items[i].SubresourceMap, ret[i].Subresources)
		}
	})
}

func getSimpleStoreTestSetup() (*SimpleStore[string], *mockClient) {
	client := &mockClient{}
	generator := &mockClientGenerator{
		ClientForFunc: func(schema Schema) (Client, error) {
			return client, nil
		},
	}
	schema := NewSimpleSchema("g", "v", &SimpleObject[string]{})
	store, _ := NewSimpleStore[string](schema, generator)
	return store, client
}
