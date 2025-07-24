package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type TestGroup struct {
	kinds []Kind
}

func (t *TestGroup) Kinds() []Kind {
	return t.kinds
}

func TestNewStore(t *testing.T) {
	generator := &mockClientGenerator{
		ClientForFunc: func(kind Kind) (Client, error) {
			assert.Fail(t, "ClientFor should not be called on New")
			return nil, nil
		},
	}
	t.Run("no groups", func(t *testing.T) {
		store := NewStore(generator)
		require.NotNil(t, store)
		assert.Equal(t, generator, store.clients)
	})
	t.Run("register groups", func(t *testing.T) {
		g1s1 := Kind{NewSimpleSchema("g1", "1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("g1s1")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
		g1s2 := Kind{NewSimpleSchema("g1", "2", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("g1s2")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
		g2s1 := Kind{NewSimpleSchema("g2", "1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("g2s1")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
		g2s2 := Kind{NewSimpleSchema("g2", "2", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("g2s2")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
		g1 := &TestGroup{[]Kind{g1s1, g1s2}}
		g2 := &TestGroup{[]Kind{g2s1, g2s2}}

		store := NewStore(generator, g1, g2)
		require.NotNil(t, store)
		assert.Equal(t, generator, store.clients)
		assert.Equal(t, g1s1, store.types[g1s1.Kind()])
		assert.Equal(t, g1s2, store.types[g1s2.Kind()])
		assert.Equal(t, g2s1, store.types[g2s1.Kind()])
		assert.Equal(t, g2s2, store.types[g2s2.Kind()])
	})
}

func TestStore_List(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		list, err := store.List(context.TODO(), kind.Kind()+"no", StoreListOptions{})
		require.Nil(t, list)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{})
		require.Nil(t, list)
		assert.Equal(t, cerr, err)
	})

	t.Run("client list error", func(t *testing.T) {
		ns := "foo"
		cerr := fmt.Errorf("JE SUIS ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(ctx context.Context, namespace string, options ListOptions) (ListObject, error) {
			return nil, cerr
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{Namespace: ns})
		require.Nil(t, list)
		assert.Equal(t, cerr, err)
	})

	t.Run("list, no filters", func(t *testing.T) {
		ns := "foo"
		ret := &UntypedList{}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 0, options.Limit)
			return ret, nil
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{Namespace: ns})
		assert.Nil(t, err)
		assert.Equal(t, ret, list)
	})

	t.Run("list, no filters, two pages", func(t *testing.T) {
		ns := "foo"
		ret1 := &UntypedList{
			ListMeta: metav1.ListMeta{
				Continue: "continue",
			},
			Items: []Object{&UntypedObject{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}},
		}
		ret2 := &UntypedList{
			ListMeta: metav1.ListMeta{
				ResourceVersion: "123",
			},
			Items: []Object{&UntypedObject{ObjectMeta: metav1.ObjectMeta{Name: "bar"}}},
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 1, options.Limit)
			if options.Continue == "continue" {
				return ret2, nil
			}
			return ret1, nil
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{Namespace: ns, PerPage: 1})
		assert.Nil(t, err)
		assert.Equal(t, ret2.GetResourceVersion(), list.GetResourceVersion())
		assert.Equal(t, 2, len(list.GetItems()))
		assert.Equal(t, ret1.Items[0], list.GetItems()[0])
		assert.Equal(t, ret2.Items[0], list.GetItems()[1])
	})

	t.Run("list, with filters", func(t *testing.T) {
		ns := "foo"
		filters := []string{"a", "b", "c"}
		ret := &UntypedList{}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 0, options.Limit)
			assert.Equal(t, filters, options.LabelFilters)
			return ret, nil
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{Namespace: ns, Filters: filters})
		assert.Nil(t, err)
		assert.Equal(t, ret, list)
	})

	t.Run("list, with field selectors", func(t *testing.T) {
		ns := "foo"
		selectors := []string{"a", "b", "c"}
		ret := &UntypedList{}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, 0, options.Limit)
			assert.Equal(t, selectors, options.FieldSelectors)
			return ret, nil
		}
		list, err := store.List(ctx, kind.Kind(), StoreListOptions{Namespace: ns, FieldSelectors: selectors})
		assert.Nil(t, err)
		assert.Equal(t, ret, list)
	})
}

func TestStore_Get(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Get(ctx, kind.Kind()+"no", Identifier{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		ret, err := store.Get(ctx, kind.Kind(), Identifier{})
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		obj, err := store.Get(ctx, kind.Kind(), Identifier{})
		assert.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		ret := &TypedSpecObject[any]{}
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return ret, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		obj, err := store.Get(ctx, kind.Kind(), id)
		assert.Nil(t, err)
		assert.Equal(t, ret, obj)
	})
}

func TestStore_Add(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()
	obj := &TypedSpecObject[any]{
		TypeMeta: metav1.TypeMeta{
			Kind: kind.Kind(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test",
		},
	}

	t.Run("empty kind", func(t *testing.T) {
		ret, err := store.Add(ctx, &TypedSpecObject[any]{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetStaticMetadata().Kind must not be empty"), err)
	})

	t.Run("empty namespace", func(t *testing.T) {
		ret, err := store.Add(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetNamespace() must not be empty"), err)
	})

	t.Run("empty name", func(t *testing.T) {
		ret, err := store.Add(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetName() must not be empty"), err)
	})

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Add(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "test",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		ret, err := store.Add(ctx, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.CreateFunc = func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Add(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &TypedSpecObject[int]{}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Add(ctx, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_SimpleAdd(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()
	obj := &TypedSpecObject[any]{
		TypeMeta: metav1.TypeMeta{
			Kind: kind.Kind(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test",
		},
	}

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.SimpleAdd(ctx, kind.Kind()+"no", Identifier{}, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		ret, err := store.SimpleAdd(ctx, kind.Kind(), Identifier{}, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.SimpleAdd(ctx, kind.Kind(), Identifier{}, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		resp := &TypedSpecObject[any]{}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.SimpleAdd(ctx, kind.Kind(), id, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_Update(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()
	obj := &TypedSpecObject[any]{
		TypeMeta: metav1.TypeMeta{
			Kind: kind.Kind(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test",
		},
	}

	t.Run("empty kind", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetStaticMetadata().Kind must not be empty"), err)
	})

	t.Run("empty namespace", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetNamespace() must not be empty"), err)
	})

	t.Run("empty name", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetName() must not be empty"), err)
	})

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "test",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		ret, err := store.Update(ctx, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Update(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &TypedSpecObject[int]{}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Update(ctx, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_UpdateSubresource(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()

	t.Run("empty kind", func(t *testing.T) {
		obj, err := store.UpdateSubresource(ctx, "", Identifier{}, "", nil)
		require.Nil(t, obj)
		assert.Equal(t, fmt.Errorf("resource kind '' is not registered in store"), err)
	})

	t.Run("empty subresourceName", func(t *testing.T) {
		obj, err := store.UpdateSubresource(ctx, kind.Kind(), Identifier{}, "", nil)
		require.Nil(t, obj)
		assert.Equal(t, fmt.Errorf("subresourceName cannot be empty"), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		obj, err := store.UpdateSubresource(ctx, kind.Kind(), Identifier{}, "status", nil)
		require.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		obj, err := store.UpdateSubresource(ctx, kind.Kind(), Identifier{}, "status", nil)
		assert.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &TypedSpecObject[int]{}
		id := Identifier{
			Namespace: "ns",
			Name:      "test",
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id.Namespace, identifier.Namespace)
			assert.Equal(t, id.Name, identifier.Name)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "status", options.Subresource)
			var expectedStatus json.RawMessage
			expectedStatus, _ = json.Marshal(1)
			actualStatus := obj.GetSubresources()["status"]
			assert.Equal(t, expectedStatus, actualStatus)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.UpdateSubresource(ctx, kind.Kind(), id, "status", 1)
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_Upsert(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()
	obj := &TypedSpecObject[any]{
		TypeMeta: metav1.TypeMeta{
			Kind: kind.Kind(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test",
		},
	}

	t.Run("empty kind", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetStaticMetadata().Kind must not be empty"), err)
	})

	t.Run("empty namespace", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetNamespace() must not be empty"), err)
	})

	t.Run("empty name", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.GetName() must not be empty"), err)
	})

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Update(ctx, &TypedSpecObject[any]{
			TypeMeta: metav1.TypeMeta{
				Kind: kind.Kind() + "no",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "test",
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client get error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client get error http 500", func(t *testing.T) {
		cerr := apierrors.NewInternalError(fmt.Errorf("Internal Server Error"))

		client.GetFunc = func(ctx context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client update error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			return obj, nil
		}
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success, get 404", func(t *testing.T) {
		resp := &TypedSpecObject[int]{}
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: "test", Resource: "test"}, identifier.Name)
		}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
	t.Run("success", func(t *testing.T) {
		resp := &TypedSpecObject[int]{}
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			return resp, nil
		}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.GetStaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.GetStaticMetadata().Name, identifier.Name)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return resp, nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		ret, err := store.Upsert(ctx, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_Delete(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("kind")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		err := store.Delete(ctx, kind.Kind()+"no", Identifier{})
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		err := store.Delete(ctx, kind.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			return cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		err := store.Delete(ctx, kind.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		err := store.Delete(ctx, kind.Kind(), id)
		assert.Nil(t, err)
	})
}

func TestStore_ForceDelete(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		err := store.Delete(ctx, kind.Kind()+"no", Identifier{})
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		err := store.ForceDelete(ctx, kind.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			return cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		err := store.ForceDelete(ctx, kind.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("success with 404", func(t *testing.T) {
		cerr := apierrors.NewNotFound(schema.GroupResource{Group: "test", Resource: "test"}, Identifier{}.Name)
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			return cerr
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		err := store.ForceDelete(ctx, kind.Kind(), Identifier{})
		assert.Equal(t, nil, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		client.DeleteFunc = func(c context.Context, identifier Identifier, _ DeleteOptions) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil
		}
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return client, nil
		}
		err := store.ForceDelete(ctx, kind.Kind(), id)
		assert.Nil(t, err)
	})
}

func TestStore_Client(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)

	t.Run("unregistered kind", func(t *testing.T) {
		c, err := store.Client(kind.Kind() + "no")
		assert.Nil(t, c)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", kind.Kind()), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("ICH BIN ERROR")
		generator.ClientForFunc = func(kind Kind) (Client, error) {
			return nil, cerr
		}
		c, err := store.Client(kind.Kind())
		assert.Nil(t, c)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		generator.ClientForFunc = func(knd Kind) (Client, error) {
			assert.Equal(t, kind, knd)
			return client, nil
		}
		c, err := store.Client(kind.Kind())
		assert.Nil(t, err)
		assert.Equal(t, client, c)
	})
}

func TestStore_Register(t *testing.T) {
	generator := &mockClientGenerator{}
	store := NewStore(generator)

	// No schema
	generator.ClientForFunc = func(kind Kind) (Client, error) {
		assert.Fail(t, "no calls should be made to ClientFor")
		return nil, nil
	}
	c, err := store.Client("test")
	assert.Nil(t, c)
	assert.Equal(t, fmt.Errorf("resource kind 'test' is not registered in store"), err)

	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	store.Register(kind)
	generator.ClientForFunc = func(knd Kind) (Client, error) {
		assert.Equal(t, kind, knd)
		return &mockClient{}, nil
	}
	c, err = store.Client("test")
	assert.NotNil(t, c)
	assert.Nil(t, err)
}

func TestStore_RegisterGroup(t *testing.T) {
	generator := &mockClientGenerator{}
	store := NewStore(generator)

	// No schema
	generator.ClientForFunc = func(kind Kind) (Client, error) {
		assert.Fail(t, "no calls should be made to ClientFor")
		return nil, nil
	}
	c, err := store.Client("test")
	assert.Nil(t, c)
	assert.Equal(t, fmt.Errorf("resource kind 'test' is not registered in store"), err)

	kind := Kind{NewSimpleSchema("g1", "v1", &TypedSpecObject[any]{}, &TypedList[*TypedSpecObject[string]]{}, WithKind("test")), map[KindEncoding]Codec{KindEncodingJSON: &JSONCodec{}}}
	group := &TestGroup{[]Kind{kind}}
	store.RegisterGroup(group)
	generator.ClientForFunc = func(knd Kind) (Client, error) {
		assert.Equal(t, kind, knd)
		return &mockClient{}, nil
	}
	c, err = store.Client("test")
	assert.NotNil(t, c)
	assert.Nil(t, err)
}

type mockClientGenerator struct {
	ClientForFunc func(Kind) (Client, error)
}

func (g *mockClientGenerator) ClientFor(s Kind) (Client, error) {
	if g.ClientForFunc != nil {
		return g.ClientForFunc(s)
	}
	return nil, nil
}

type mockClient struct {
	GetFunc                func(ctx context.Context, identifier Identifier) (Object, error)
	GetIntoFunc            func(ctx context.Context, identifier Identifier, into Object) error
	CreateFunc             func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error)
	CreateIntoFunc         func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions, into Object) error
	UpdateFunc             func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error)
	UpdateIntoFunc         func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error
	PatchFunc              func(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions) (Object, error)
	PatchIntoFunc          func(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions, into Object) error
	DeleteFunc             func(ctx context.Context, identifier Identifier, options DeleteOptions) error
	ListFunc               func(ctx context.Context, namespace string, options ListOptions) (ListObject, error)
	ListIntoFunc           func(ctx context.Context, namespace string, options ListOptions, into ListObject) error
	WatchFunc              func(ctx context.Context, namespace string, options WatchOptions) (WatchResponse, error)
	SubresourceRequestFunc func(ctx context.Context, identifier Identifier, options CustomRouteRequestOptions) ([]byte, error)
}

func (c *mockClient) Get(ctx context.Context, identifier Identifier) (Object, error) {
	if c.GetFunc != nil {
		return c.GetFunc(ctx, identifier)
	}
	return nil, nil
}
func (c *mockClient) GetInto(ctx context.Context, identifier Identifier, into Object) error {
	if c.GetIntoFunc != nil {
		return c.GetIntoFunc(ctx, identifier, into)
	}
	return nil
}
func (c *mockClient) Create(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
	if c.CreateFunc != nil {
		return c.CreateFunc(ctx, identifier, obj, options)
	}
	return nil, nil
}
func (c *mockClient) CreateInto(ctx context.Context, identifier Identifier, obj Object, options CreateOptions, into Object) error {
	if c.CreateIntoFunc != nil {
		return c.CreateIntoFunc(ctx, identifier, obj, options, into)
	}
	return nil
}
func (c *mockClient) Update(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
	if c.UpdateFunc != nil {
		return c.UpdateFunc(ctx, identifier, obj, options)
	}
	return nil, nil
}
func (c *mockClient) UpdateInto(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
	if c.UpdateIntoFunc != nil {
		return c.UpdateIntoFunc(ctx, identifier, obj, options, into)
	}
	return nil
}
func (c *mockClient) Patch(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions) (Object, error) {
	if c.PatchFunc != nil {
		return c.PatchFunc(ctx, identifier, patch, options)
	}
	return nil, nil
}
func (c *mockClient) PatchInto(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions, into Object) error {
	if c.PatchIntoFunc != nil {
		return c.PatchIntoFunc(ctx, identifier, patch, options, into)
	}
	return nil
}
func (c *mockClient) Delete(ctx context.Context, identifier Identifier, options DeleteOptions) error {
	if c.DeleteFunc != nil {
		return c.DeleteFunc(ctx, identifier, options)
	}
	return nil
}
func (c *mockClient) List(ctx context.Context, namespace string, options ListOptions) (ListObject, error) {
	if c.ListFunc != nil {
		return c.ListFunc(ctx, namespace, options)
	}
	return nil, nil
}
func (c *mockClient) ListInto(ctx context.Context, namespace string, options ListOptions, into ListObject) error {
	if c.ListIntoFunc != nil {
		return c.ListIntoFunc(ctx, namespace, options, into)
	}
	return nil
}
func (c *mockClient) Watch(ctx context.Context, namespace string, options WatchOptions) (WatchResponse, error) {
	if c.WatchFunc != nil {
		return c.WatchFunc(ctx, namespace, options)
	}
	return nil, nil
}
func (c *mockClient) SubresourceRequest(ctx context.Context, identifier Identifier, options CustomRouteRequestOptions) ([]byte, error) {
	if c.SubresourceRequestFunc != nil {
		return c.SubresourceRequestFunc(ctx, identifier, options)
	}
	return nil, nil
}
