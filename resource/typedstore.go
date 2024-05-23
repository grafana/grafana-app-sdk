package resource

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

// TypedStore is a single-Schema store where returned Objects from the underlying client are assumed
// to be of ObjectType. It is a thin convenience layer over using a raw ClientGenerator.ClientFor()-created
// Client for a Schema and doing type conversions in-code.
type TypedStore[ObjectType Object] struct {
	client Client
	sch    Schema
}

// NewTypedStore creates a new TypedStore. The ObjectType and Schema.ZeroValue()'s underlying type should match.
// If they do not, an error is returned.
func NewTypedStore[ObjectType Object](kind Kind, generator ClientGenerator) (*TypedStore[ObjectType], error) {
	schemaType := reflect.TypeOf(kind.ZeroValue())
	providedType := reflect.TypeOf(new(ObjectType)).Elem()
	// Get the actual underlying types
	// Do both at once, because there needs to be casting ability between them
	for schemaType.Kind() == reflect.Ptr && providedType.Kind() == reflect.Ptr {
		schemaType = schemaType.Elem()
		providedType = providedType.Elem()
	}
	if schemaType != providedType {
		return nil, fmt.Errorf(
			"underlying types of schema.ZeroValue() and provided ObjectType are not the same (%s != %s)",
			schemaType.Name(), providedType.Name())
	}
	client, err := generator.ClientFor(kind)
	if err != nil {
		return nil, fmt.Errorf("error getting client from generator: %w", err)
	}
	return &TypedStore[ObjectType]{
		client: client,
		sch:    &kind,
	}, nil
}

// Get returns a resource with the provided identifier
func (t *TypedStore[T]) Get(ctx context.Context, identifier Identifier) (T, error) {
	obj, err := t.client.Get(ctx, identifier)
	if err != nil {
		var n T
		return n, err
	}
	return t.cast(obj)
}

// Add creates a new resource. obj.GetName() must not be empty, and obj.GetNamespace() cannot be empty for namespace-scoped kinds.
// If they are not, no request is made to the underlying client, and an error is returned.
func (t *TypedStore[T]) Add(ctx context.Context, obj T) (T, error) {
	if t.sch.Scope() == ClusterScope {
		if obj.GetNamespace() != "" {
			var n T
			return n, fmt.Errorf("obj.GetNamespace() must be empty for cluster-scoped objects")
		}
	} else {
		if obj.GetNamespace() == "" {
			var n T
			return n, fmt.Errorf("obj.GetNamespace() must not be empty")
		}
	}
	if obj.GetName() == "" {
		var n T
		return n, fmt.Errorf("obj.GetName() must not be empty")
	}
	ret, err := t.client.Create(ctx, Identifier{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, obj, CreateOptions{})
	if err != nil {
		var n T
		return n, err
	}
	return t.cast(ret)
}

// Update updates an existing resource, and returns the updated version.
// Keep in mind that an Update will completely overwrite the object,
// so nil or missing values will be removed, not ignored.
// It is usually best to use the result of a Get call, change the appropriate values, and then call Update with that.
// The update will fail if no ResourceVersion is provided, or if the ResourceVersion does not match the current one.
// It returns the updated Object from the storage system.
func (t *TypedStore[T]) Update(ctx context.Context, identifier Identifier, obj T) (T, error) {
	md := obj.GetCommonMetadata()
	md.UpdateTimestamp = time.Now().UTC()
	obj.SetCommonMetadata(md)
	ret, err := t.client.Update(ctx, identifier, obj, UpdateOptions{})
	if err != nil {
		var n T
		return n, err
	}
	return t.cast(ret)
}

// Upsert updates an existing resource or creates a new one if none exists, and returns the new version.
// Keep in mind that an Upsert will completely overwrite the object,
// so nil or missing values will be removed, not ignored.
// It is usually best to use the result of a Get call, change the appropriate values, and then call Upsert with that.
// The update will fail if no ResourceVersion is provided, or if the ResourceVersion does not match the current one.
// It returns the updated Object from the storage system.
func (t *TypedStore[T]) Upsert(ctx context.Context, identifier Identifier, obj T) (T, error) {
	resp, err := t.client.Get(ctx, identifier)

	if err != nil {
		var n T
		cast, ok := err.(APIServerResponseError)
		if !ok {
			return n, err
		} else if cast.StatusCode() != http.StatusNotFound {
			return n, err
		}
	}
	var ret Object

	if resp != nil {
		md := obj.GetCommonMetadata()
		md.UpdateTimestamp = time.Now().UTC()
		obj.SetCommonMetadata(md)
		ret, err = t.client.Update(ctx, identifier, obj, UpdateOptions{})
	} else {
		ret, err = t.client.Create(ctx, Identifier{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}, obj, CreateOptions{})
	}
	if err != nil {
		var n T
		return n, err
	}

	return t.cast(ret)
}

// UpdateSubresource updates a subresource of an object.
// The provided obj parameter must have the specified subresource,
// and only that subresource will be updated in the storage system.
func (t *TypedStore[T]) UpdateSubresource(ctx context.Context, identifier Identifier,
	subresource SubresourceName, obj Object) (T, error) {
	ret, err := t.client.Update(ctx, identifier, obj, UpdateOptions{
		Subresource: string(subresource),
	})
	if err != nil {
		var n T
		return n, err
	}
	return t.cast(ret)
}

// Delete deletes a resource with the provided identifier
func (t *TypedStore[T]) Delete(ctx context.Context, identifier Identifier) error {
	return t.client.Delete(ctx, identifier)
}

// ForceDelete deletes a resource with the provided identifier, ignores 404 errors
func (t *TypedStore[T]) ForceDelete(ctx context.Context, identifier Identifier) error {
	err := t.client.Delete(ctx, identifier)

	if cast, ok := err.(APIServerResponseError); ok && cast.StatusCode() == http.StatusNotFound {
		return nil
	}

	return err
}

// List lists all resources in the provided namespace, optionally filtered by the provided filters.
// It will auto-list all pages in the response using the perPage limit for requests.
// To list a single page of results, use ListPage.
func (t *TypedStore[T]) List(ctx context.Context, namespace string, perPage int, filters ...string) (*TypedList[T], error) {
	resp, err := t.ListPage(ctx, namespace, ListOptions{
		Limit:        perPage,
		LabelFilters: filters,
	})
	if err != nil {
		return nil, err
	}
	if resp.Continue == "" {
		return resp, nil
	}
	for resp.Continue != "" {
		page, err := t.ListPage(ctx, namespace, ListOptions{
			Continue:     resp.Continue,
			Limit:        perPage,
			LabelFilters: filters,
		})
		if err != nil {
			return nil, err
		}
		resp.Continue = page.Continue
		resp.ResourceVersion = page.ResourceVersion
		resp.Items = append(resp.Items, page.Items...)
	}
	return resp, nil
}

// ListPage lists a single page of resources, with no auto-paging logic like List.
// This is semantically identical to calling Client().ListInto(ctx, namespace, options, &TypedList[T])
func (t *TypedStore[T]) ListPage(ctx context.Context, namespace string, options ListOptions) (*TypedList[T], error) {
	resp := &TypedList[T]{}
	err := t.client.ListInto(ctx, namespace, options, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Client returns the underlying Client for this store.
func (t *TypedStore[T]) Client() Client {
	return t.client
}

//nolint:revive
func (t *TypedStore[T]) cast(obj Object) (T, error) {
	cast, ok := obj.(T)
	if !ok {
		var n T
		return n, fmt.Errorf("unable to cast Object into provided type")
	}
	return cast, nil
}
