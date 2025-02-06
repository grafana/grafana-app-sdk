package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewSimpleStore(t *testing.T) {
	schema := Kind{NewSimpleSchema("g", "v", &TypedSpecObject[int]{}, &TypedList[*TypedSpecObject[int]]{}, WithKind("k")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	t.Run("type mismatch", func(t *testing.T) {
		store, err := NewSimpleStore[string](schema, &mockClientGenerator{})
		assert.Nil(t, store)
		assert.Equal(t, fmt.Errorf("SpecType 'string' does not match underlying schema.ZeroValue().SpecObject() type 'int'"), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("SOY ERROR")
		generator := &mockClientGenerator{
			ClientForFunc: func(k Kind) (Client, error) {
				assert.Equal(t, schema, k)
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
			ClientForFunc: func(k Kind) (Client, error) {
				assert.Equal(t, schema, k)
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
		obj := &TypedSpecObject[string]{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test",
				ResourceVersion: "123",
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
		assert.Equal(t, obj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, obj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, obj.Spec, ret.Spec)
	})
}

func TestSimpleStore_Add(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
	retObj := &TypedSpecObject[string]{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "123",
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
			assert.Equal(t, "test", obj.GetSpec())
			return retObj, nil
		}
		ret, err := store.Add(ctx, id, "test")
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
	})

	t.Run("success, with metadata options", func(t *testing.T) {
		lbls := map[string]string{
			"field": "value",
		}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.GetSpec())
			assert.Equal(t, lbls, obj.GetCommonMetadata().Labels)
			return retObj, nil
		}
		ret, err := store.Add(ctx, id, "test", WithLabels(lbls))
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
	})
}

func TestSimpleStore_Update(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
	retObj := &TypedSpecObject[string]{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "123",
		},
		Spec: "foo",
	}
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}

	t.Run("get error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Update(ctx, id, "")
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("update error", func(t *testing.T) {
		cerr := fmt.Errorf("SOY ERROR")
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return retObj, nil
		}
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Update(ctx, id, "")
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return retObj, nil
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.GetSpec())
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test")
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
	})

	t.Run("success, with metadata options", func(t *testing.T) {
		lbls := map[string]string{
			"field": "value",
		}
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return retObj, nil
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.GetSpec())
			assert.Equal(t, lbls, obj.GetCommonMetadata().Labels)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test", WithLabels(lbls))
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
	})

	t.Run("success, with resourceVersion option", func(t *testing.T) {
		rv := "12345"
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return retObj, nil
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, "test", obj.GetSpec())
			assert.Equal(t, rv, obj.GetCommonMetadata().ResourceVersion)
			assert.Equal(t, rv, options.ResourceVersion)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, "test", WithResourceVersion(rv))
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
	})
}

func TestSimpleStore_UpdateSubresource(t *testing.T) {
	store, client := getSimpleStoreTestSetup()
	ctx := context.TODO()
	id := Identifier{
		Namespace: "ns",
		Name:      "test",
	}
	retObj := &TypedSpecObject[string]{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "123",
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
			fmt.Println("obj: ", obj)
			fmt.Println("opts: ", options)
			assert.Equal(t, string(SubresourceStatus), options.Subresource)
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			assert.Equal(t, 2, obj.GetSubresources()[string(SubresourceStatus)])
			return retObj, nil
		}
		ret, err := store.UpdateSubresource(ctx, id, SubresourceStatus, 2)
		assert.Nil(t, err)
		assert.Equal(t, retObj.GetStaticMetadata(), ret.GetStaticMetadata())
		assert.Equal(t, retObj.GetCommonMetadata(), ret.GetCommonMetadata())
		assert.Equal(t, retObj.Spec, ret.Spec)
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
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return cerr
		}
		err := store.Delete(ctx, id)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
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
	list := &TypedList[*TypedObject[string, MapSubresourceCatalog]]{
		Items: []*TypedObject[string, MapSubresourceCatalog]{
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
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Subresources, ret[i].Subresources)
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
		ret, err := store.ListWithFiltersAndSelectors(ctx, ns, filters, nil)
		assert.Nil(t, err)
		assert.Equal(t, len(list.Items), len(ret))
		for i := 0; i < len(ret); i++ {
			assert.Equal(t, list.Items[i].Spec, ret[i].Spec)
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Subresources, ret[i].Subresources)
		}
	})

	t.Run("success, with field selectors", func(t *testing.T) {
		selectors := []string{"a", "b"}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, selectors, options.FieldSelectors)
			return list, nil
		}
		ret, err := store.ListWithFiltersAndSelectors(ctx, ns, nil, selectors)
		assert.Nil(t, err)
		assert.Equal(t, len(list.Items), len(ret))
		for i := 0; i < len(ret); i++ {
			assert.Equal(t, list.Items[i].Spec, ret[i].Spec)
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Subresources, ret[i].Subresources)
		}
	})
}

func getSimpleStoreTestSetup() (*SimpleStore[string], *mockClient) {
	client := &mockClient{}
	generator := &mockClientGenerator{
		ClientForFunc: func(kind Kind) (Client, error) {
			return client, nil
		},
	}
	kind := Kind{NewSimpleSchema("g", "v", &TypedSpecObject[string]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store, _ := NewSimpleStore[string](kind, generator)
	return store, client
}
