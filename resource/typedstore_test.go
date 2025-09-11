package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewTypedStore(t *testing.T) {
	schema := Kind{NewSimpleSchema("g", "v", &TypedSpecObject[int]{}, &TypedList[*TypedSpecObject[int]]{}, WithKind("k")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	t.Run("type mismatch", func(t *testing.T) {
		store, err := NewTypedStore[*TypedSpecStatusObject[string, string]](schema, &mockClientGenerator{})
		assert.Nil(t, store)
		assert.Equal(t, fmt.Errorf("underlying types of schema.ZeroValue() and provided ObjectType are not the same (TypedSpecObject[int] != TypedSpecStatusObject[string,string])"), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("SOY ERROR")
		generator := &mockClientGenerator{
			ClientForFunc: func(k Kind) (Client, error) {
				assert.Equal(t, schema, k)
				return nil, cerr
			},
		}
		store, err := NewTypedStore[*TypedSpecObject[int]](schema, generator)
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
		store, err := NewTypedStore[*TypedSpecObject[int]](schema, generator)
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
		obj := &TypedSpecStatusObject[string, string]{
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
		assert.Equal(t, obj, ret)
	})
}

func TestTypedStore_Add(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	addObj := &TypedSpecStatusObject[string, string]{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "ns",
			Name:            "test",
			ResourceVersion: "123",
		},
		Spec: "bar",
	}
	retObj := &TypedSpecStatusObject[string, string]{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "123",
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
			assert.Equal(t, addObj.Name, identifier.Name)
			assert.Equal(t, addObj.Namespace, identifier.Namespace)
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
	updateObj := &TypedSpecStatusObject[string, string]{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "ns",
			Name:            "test",
			ResourceVersion: "abc",
		},
		Spec: "bar",
	}
	retObj := &TypedSpecStatusObject[string, string]{
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
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Update(ctx, id, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("no ResourceVersion", func(t *testing.T) {
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Fail(t, "client update should not be called")
			return nil, nil
		}
		obj := updateObj.Copy().(*TypedSpecStatusObject[string, string])
		obj.SetResourceVersion("")
		ret, err := store.Update(ctx, id, obj)
		assert.Nil(t, ret)
		assert.Equal(t, ErrMissingResourceVersion, err)
	})

	t.Run("success, no metadata options", func(t *testing.T) {
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, updateObj.Name, identifier.Name)
			assert.Equal(t, updateObj.Namespace, identifier.Namespace)
			assert.Equal(t, updateObj, obj)
			assert.Equal(t, updateObj.ResourceVersion, options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return retObj, nil
		}
		ret, err := store.Update(ctx, id, updateObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
}

func TestTypedStore_Upsert(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	updateObj := &TypedSpecStatusObject[string, string]{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "ns",
			Name:            "test",
			ResourceVersion: "abc",
		},
		Spec: "bar",
	}

	retObj := &TypedSpecStatusObject[string, string]{
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

	t.Run("error get", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Upsert(ctx, id, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("error get 503", func(t *testing.T) {
		cerr := apierrors.NewInternalError(fmt.Errorf("Internal Server Error"))
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Upsert(ctx, id, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("error update", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return updateObj, nil
		}
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		ret, err := store.Upsert(ctx, id, updateObj)
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, get 404", func(t *testing.T) {
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: "test", Resource: "test"}, identifier.Name)
		}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, updateObj.Name, identifier.Name)
			assert.Equal(t, updateObj.Namespace, identifier.Namespace)
			assert.Equal(t, updateObj, obj)
			return retObj, nil
		}
		ret, err := store.Upsert(ctx, id, updateObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
	t.Run("success, no metadata options", func(t *testing.T) {
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return updateObj, nil
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, updateObj.Name, identifier.Name)
			assert.Equal(t, updateObj.Namespace, identifier.Namespace)
			assert.Equal(t, updateObj, obj)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return retObj, nil
		}
		ret, err := store.Upsert(ctx, id, updateObj)
		assert.Nil(t, err)
		assert.Equal(t, retObj, ret)
	})
}

func TestTypedStore_UpdateSubresource(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	updateObj := &TypedSpecStatusObject[string, string]{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "ns",
			Name:            "test",
			ResourceVersion: "abc",
		},
		Status: "foo",
	}
	retObj := &TypedSpecStatusObject[string, string]{
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
			assert.Equal(t, updateObj.ResourceVersion, options.ResourceVersion)
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

func TestTypedStore_ForceDelete(t *testing.T) {
	store, client := getTypedStoreTestSetup()
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
		err := store.ForceDelete(ctx, id)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil
		}
		err := store.ForceDelete(ctx, id)
		assert.Nil(t, err)
	})

	t.Run("success with 404", func(t *testing.T) {
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return apierrors.NewNotFound(schema.GroupResource{Group: "test", Resource: "test"}, identifier.Name)
		}
		err := store.ForceDelete(ctx, id)
		assert.Nil(t, err)
	})
}

func TestTypedStore_List(t *testing.T) {
	store, client := getTypedStoreTestSetup()
	ctx := context.TODO()
	ns := "ns"
	list := &TypedList[*TypedSpecStatusObject[string, string]]{
		Items: []*TypedSpecStatusObject[string, string]{
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
		client.ListIntoFunc = func(ctx context.Context, namespace string, options ListOptions, into ListObject) error {
			return cerr
		}
		ret, err := store.List(ctx, StoreListOptions{Namespace: ns})
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, no pagination", func(t *testing.T) {
		client.ListIntoFunc = func(c context.Context, namespace string, options ListOptions, into ListObject) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 0, options.Limit)
			into.SetItems(list.GetItems())
			return nil
		}
		ret, err := store.List(ctx, StoreListOptions{Namespace: ns})
		assert.Nil(t, err)
		assert.Equal(t, len(list.GetItems()), len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret.Items[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret.Items[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Status, ret.Items[i].Status)
		}
	})

	t.Run("success, with two pages", func(t *testing.T) {
		client.ListIntoFunc = func(c context.Context, namespace string, options ListOptions, into ListObject) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 2, options.Limit)
			into.SetItems(list.GetItems())
			if options.Continue == "" {
				into.SetContinue("continue")
			}
			return nil
		}
		ret, err := store.List(ctx, StoreListOptions{Namespace: ns, PerPage: 2})
		assert.Nil(t, err)
		assert.Equal(t, len(list.GetItems())*2, len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i%2].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i%2].GetStaticMetadata(), ret.Items[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i%2].GetCommonMetadata(), ret.Items[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i%2].Status, ret.Items[i].Status)
		}
	})

	t.Run("success, with filters", func(t *testing.T) {
		filters := []string{"a", "b"}
		client.ListIntoFunc = func(c context.Context, namespace string, options ListOptions, into ListObject) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, filters, options.LabelFilters)
			into.SetItems(list.GetItems())
			return nil
		}
		ret, err := store.List(ctx, StoreListOptions{Namespace: ns, Filters: filters})
		assert.Nil(t, err)
		assert.Equal(t, len(list.GetItems()), len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret.Items[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret.Items[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Status, ret.Items[i].Status)
		}
	})
	t.Run("success, with field selectors", func(t *testing.T) {
		selectors := []string{"a", "b"}
		client.ListIntoFunc = func(c context.Context, namespace string, options ListOptions, into ListObject) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, selectors, options.FieldSelectors)
			into.SetItems(list.GetItems())
			return nil
		}
		ret, err := store.List(ctx, StoreListOptions{Namespace: ns, FieldSelectors: selectors})
		assert.Nil(t, err)
		assert.Equal(t, len(list.GetItems()), len(ret.Items))
		for i := 0; i < len(ret.Items); i++ {
			assert.Equal(t, list.Items[i].Spec, ret.Items[i].Spec)
			assert.Equal(t, list.Items[i].GetStaticMetadata(), ret.Items[i].GetStaticMetadata())
			assert.Equal(t, list.Items[i].GetCommonMetadata(), ret.Items[i].GetCommonMetadata())
			assert.Equal(t, list.Items[i].Status, ret.Items[i].Status)
		}
	})
}

func getTypedStoreTestSetup() (*TypedStore[*TypedSpecStatusObject[string, string]], *mockClient) {
	client := &mockClient{}
	generator := &mockClientGenerator{
		ClientForFunc: func(kind Kind) (Client, error) {
			return client, nil
		},
	}
	kind := Kind{NewSimpleSchema("g", "v", &TypedSpecStatusObject[string, string]{}, &TypedList[*TypedSpecStatusObject[string, string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store, _ := NewTypedStore[*TypedSpecStatusObject[string, string]](kind, generator)
	return store, client
}
