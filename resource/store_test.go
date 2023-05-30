package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	generator := &mockClientGenerator{
		ClientForFunc: func(schema Schema) (Client, error) {
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
		g1 := NewSimpleSchemaGroup("g1", "1")
		g1s1 := g1.AddSchema(&SimpleObject[any]{}, WithKind("g1s1"))
		g1s2 := g1.AddSchema(&SimpleObject[any]{}, WithKind("g1s2"))
		g2 := NewSimpleSchemaGroup("g2", "1")
		g2s1 := g2.AddSchema(&SimpleObject[any]{}, WithKind("g2s1"))
		g2s2 := g2.AddSchema(&SimpleObject[any]{}, WithKind("g2s2"))

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
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		list, err := store.List(context.TODO(), schema.Kind()+"no", "")
		require.Nil(t, list)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return nil, cerr
		}
		list, err := store.List(ctx, schema.Kind(), "")
		require.Nil(t, list)
		assert.Equal(t, cerr, err)
	})

	t.Run("client list error", func(t *testing.T) {
		ns := "foo"
		cerr := fmt.Errorf("JE SUIS ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(ctx context.Context, namespace string, options ListOptions) (ListObject, error) {
			return nil, cerr
		}
		list, err := store.List(ctx, schema.Kind(), ns)
		require.Nil(t, list)
		assert.Equal(t, cerr, err)
	})

	t.Run("list, no filters", func(t *testing.T) {
		ns := "foo"
		ret := &mockListObject{}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			return ret, nil
		}
		list, err := store.List(ctx, schema.Kind(), ns)
		assert.Nil(t, err)
		assert.Equal(t, ret, list)
	})

	t.Run("list, with filters", func(t *testing.T) {
		ns := "foo"
		filters := []string{"a", "b", "c"}
		ret := &mockListObject{}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		client.ListFunc = func(c context.Context, namespace string, options ListOptions) (ListObject, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, ns, namespace)
			assert.Equal(t, filters, options.LabelFilters)
			return ret, nil
		}
		list, err := store.List(ctx, schema.Kind(), ns, filters...)
		assert.Nil(t, err)
		assert.Equal(t, ret, list)
	})
}

func TestStore_Get(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Get(ctx, schema.Kind()+"no", Identifier{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return nil, cerr
		}
		ret, err := store.Get(ctx, schema.Kind(), Identifier{})
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		obj, err := store.Get(ctx, schema.Kind(), Identifier{})
		assert.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		ret := &SimpleObject[any]{}
		client.GetFunc = func(c context.Context, identifier Identifier) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return ret, nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		obj, err := store.Get(ctx, schema.Kind(), id)
		assert.Nil(t, err)
		assert.Equal(t, ret, obj)
	})
}

func TestStore_Add(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()
	obj := &SimpleObject[any]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Kind:      schema.Kind(),
				Namespace: "ns",
				Name:      "test",
			},
		},
	}

	t.Run("empty kind", func(t *testing.T) {
		ret, err := store.Add(ctx, &SimpleObject[any]{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Kind must not be empty"), err)
	})

	t.Run("empty namespace", func(t *testing.T) {
		ret, err := store.Add(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind: schema.Kind() + "no",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Namespace must not be empty"), err)
	})

	t.Run("empty name", func(t *testing.T) {
		ret, err := store.Add(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind:      schema.Kind() + "no",
					Namespace: "ns",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Name must not be empty"), err)
	})

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Add(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind:      schema.Kind() + "no",
					Namespace: "ns",
					Name:      "test",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
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
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		ret, err := store.Add(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &SimpleObject[int]{}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.StaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.StaticMetadata().Name, identifier.Name)
			return resp, nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
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
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()
	obj := &SimpleObject[any]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Kind:      schema.Kind(),
				Namespace: "ns",
				Name:      "test",
			},
		},
	}

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.SimpleAdd(ctx, schema.Kind()+"no", Identifier{}, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return nil, cerr
		}
		ret, err := store.SimpleAdd(ctx, schema.Kind(), Identifier{}, obj.Copy())
		require.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		ret, err := store.SimpleAdd(ctx, schema.Kind(), Identifier{}, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		resp := &SimpleObject[any]{}
		client.CreateFunc = func(c context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return resp, nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		ret, err := store.SimpleAdd(ctx, schema.Kind(), id, obj.Copy())
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_Update(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()
	obj := &SimpleObject[any]{
		BasicMetadataObject: BasicMetadataObject{
			StaticMeta: StaticMetadata{
				Kind:      schema.Kind(),
				Namespace: "ns",
				Name:      "test",
			},
		},
	}

	t.Run("empty kind", func(t *testing.T) {
		ret, err := store.Update(ctx, &SimpleObject[any]{})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Kind must not be empty"), err)
	})

	t.Run("empty namespace", func(t *testing.T) {
		ret, err := store.Update(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind: schema.Kind() + "no",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Namespace must not be empty"), err)
	})

	t.Run("empty name", func(t *testing.T) {
		ret, err := store.Update(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind:      schema.Kind() + "no",
					Namespace: "ns",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("obj.StaticMetadata().Name must not be empty"), err)
	})

	t.Run("unregistered Schema", func(t *testing.T) {
		ret, err := store.Update(ctx, &SimpleObject[any]{
			BasicMetadataObject: BasicMetadataObject{
				StaticMeta: StaticMetadata{
					Kind:      schema.Kind() + "no",
					Namespace: "ns",
					Name:      "test",
				},
			},
		})
		require.Nil(t, ret)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
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
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		ret, err := store.Update(ctx, obj.Copy())
		assert.Nil(t, ret)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &SimpleObject[int]{}
		client.UpdateFunc = func(c context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, obj.StaticMetadata().Namespace, identifier.Namespace)
			assert.Equal(t, obj.StaticMetadata().Name, identifier.Name)
			assert.Equal(t, "", options.ResourceVersion)
			assert.Equal(t, "", options.Subresource)
			return resp, nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
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
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()

	t.Run("empty kind", func(t *testing.T) {
		obj, err := store.UpdateSubresource(ctx, "", Identifier{}, "", nil)
		require.Nil(t, obj)
		assert.Equal(t, fmt.Errorf("resource kind '' is not registered in store"), err)
	})

	t.Run("empty subresourceName", func(t *testing.T) {
		obj, err := store.UpdateSubresource(ctx, schema.Kind(), Identifier{}, "", nil)
		require.Nil(t, obj)
		assert.Equal(t, fmt.Errorf("subresourceName cannot be empty"), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return nil, cerr
		}
		obj, err := store.UpdateSubresource(ctx, schema.Kind(), Identifier{}, "status", nil)
		require.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.UpdateFunc = func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error) {
			return nil, cerr
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		obj, err := store.UpdateSubresource(ctx, schema.Kind(), Identifier{}, "status", nil)
		assert.Nil(t, obj)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		resp := &SimpleObject[int]{}
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
			assert.Equal(t, 1, obj.Subresources()["status"])
			return resp, nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		ret, err := store.UpdateSubresource(ctx, schema.Kind(), id, "status", 1)
		assert.Nil(t, err)
		assert.Equal(t, resp, ret)
	})
}

func TestStore_Delete(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	ctx := context.TODO()

	t.Run("unregistered Schema", func(t *testing.T) {
		err := store.Delete(ctx, schema.Kind()+"no", Identifier{})
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("ClientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("I AM ERROR")
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return nil, cerr
		}
		err := store.Delete(ctx, schema.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("client error", func(t *testing.T) {
		cerr := fmt.Errorf("JE SUIS ERROR")
		client.DeleteFunc = func(c context.Context, identifier Identifier) error {
			return cerr
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		err := store.Delete(ctx, schema.Kind(), Identifier{})
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		id := Identifier{
			Namespace: "foo",
			Name:      "bar",
		}
		client.DeleteFunc = func(c context.Context, identifier Identifier) error {
			assert.Equal(t, ctx, c)
			assert.Equal(t, id, identifier)
			return nil
		}
		generator.ClientForFunc = func(schema Schema) (Client, error) {
			return client, nil
		}
		err := store.Delete(ctx, schema.Kind(), id)
		assert.Nil(t, err)
	})
}

func TestStore_Client(t *testing.T) {
	client := &mockClient{}
	generator := &mockClientGenerator{}
	store := NewStore(generator)
	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)

	t.Run("unregistered kind", func(t *testing.T) {
		c, err := store.Client(schema.Kind() + "no")
		assert.Nil(t, c)
		assert.Equal(t, fmt.Errorf("resource kind '%sno' is not registered in store", schema.Kind()), err)
	})

	t.Run("clientGenerator error", func(t *testing.T) {
		cerr := fmt.Errorf("ICH BIN ERROR")
		generator.ClientForFunc = func(sch Schema) (Client, error) {
			return nil, cerr
		}
		c, err := store.Client(schema.Kind())
		assert.Nil(t, c)
		assert.Equal(t, cerr, err)
	})

	t.Run("success", func(t *testing.T) {
		generator.ClientForFunc = func(sch Schema) (Client, error) {
			assert.Equal(t, schema, sch)
			return client, nil
		}
		c, err := store.Client(schema.Kind())
		assert.Nil(t, err)
		assert.Equal(t, client, c)
	})
}

func TestStore_Register(t *testing.T) {
	generator := &mockClientGenerator{}
	store := NewStore(generator)

	// No schema
	generator.ClientForFunc = func(schema Schema) (Client, error) {
		assert.Fail(t, "no calls should be made to ClientFor")
		return nil, nil
	}
	c, err := store.Client("test")
	assert.Nil(t, c)
	assert.Equal(t, fmt.Errorf("resource kind 'test' is not registered in store"), err)

	schema := NewSimpleSchema("g1", "v1", &SimpleObject[any]{}, WithKind("test"))
	store.Register(schema)
	generator.ClientForFunc = func(sch Schema) (Client, error) {
		assert.Equal(t, schema, sch)
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
	generator.ClientForFunc = func(schema Schema) (Client, error) {
		assert.Fail(t, "no calls should be made to ClientFor")
		return nil, nil
	}
	c, err := store.Client("test")
	assert.Nil(t, c)
	assert.Equal(t, fmt.Errorf("resource kind 'test' is not registered in store"), err)

	group := NewSimpleSchemaGroup("g1", "v1")
	schema := group.AddSchema(&SimpleObject[any]{}, WithKind("test"))
	store.RegisterGroup(group)
	generator.ClientForFunc = func(sch Schema) (Client, error) {
		assert.Equal(t, schema, sch)
		return &mockClient{}, nil
	}
	c, err = store.Client("test")
	assert.NotNil(t, c)
	assert.Nil(t, err)
}

type mockListObject struct {
	ListMeta ListMetadata
	List     []Object
}

func (l *mockListObject) ListItems() []Object {
	return l.List
}

func (l *mockListObject) SetItems(o []Object) {

}

func (l *mockListObject) ListMetadata() ListMetadata {
	return l.ListMeta
}

func (l *mockListObject) SetListMetadata(m ListMetadata) {

}

type mockClientGenerator struct {
	ClientForFunc func(Schema) (Client, error)
}

func (g *mockClientGenerator) ClientFor(s Schema) (Client, error) {
	if g.ClientForFunc != nil {
		return g.ClientForFunc(s)
	}
	return nil, nil
}

type mockClient struct {
	GetFunc        func(ctx context.Context, identifier Identifier) (Object, error)
	GetIntoFunc    func(ctx context.Context, identifier Identifier, into Object) error
	CreateFunc     func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions) (Object, error)
	CreateIntoFunc func(ctx context.Context, identifier Identifier, obj Object, options CreateOptions, into Object) error
	UpdateFunc     func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions) (Object, error)
	UpdateIntoFunc func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error
	PatchFunc      func(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions) (Object, error)
	PatchIntoFunc  func(ctx context.Context, identifier Identifier, patch PatchRequest, options PatchOptions, into Object) error
	DeleteFunc     func(ctx context.Context, identifier Identifier) error
	ListFunc       func(ctx context.Context, namespace string, options ListOptions) (ListObject, error)
	ListIntoFunc   func(ctx context.Context, namespace string, options ListOptions, into ListObject) error
	WatchFunc      func(ctx context.Context, namespace string, options WatchOptions) (WatchResponse, error)
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
func (c *mockClient) Delete(ctx context.Context, identifier Identifier) error {
	if c.DeleteFunc != nil {
		return c.DeleteFunc(ctx, identifier)
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
