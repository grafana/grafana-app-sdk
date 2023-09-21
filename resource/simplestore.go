package resource

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// SimpleStoreResource is a type used by SimpleStore to return concrete data, rather than an Object.
// All data from an underlying Object returned by a client is preserved in this struct,
// But with a concrete type for the SpecObject() (presented here as Spec).
// TODO: concrete ObjectMetadata type in addition to SpecType? Would make using this a mite more complicated with MetadataOption
type SimpleStoreResource[SpecType any] struct {
	Spec           SpecType       `json:"spec"`
	StaticMetadata StaticMetadata `json:"staticMetadata"`
	CommonMetadata CommonMetadata `json:"objectMetadata"`
	CustomMetadata CustomMetadata `json:"customMetadata"`
	Subresources   map[string]any `json:"subresources"`
}

// ObjectMetadataOption is a function which updates an ObjectMetadata
type ObjectMetadataOption func(o *CommonMetadata)

// WithLabels sets the labels of an ObjectMetadata
func WithLabels(labels map[string]string) ObjectMetadataOption {
	return func(o *CommonMetadata) {
		o.Labels = labels
	}
}

// WithLabel sets a specific key in the labels of an ObjectMetadata
func WithLabel(key, value string) ObjectMetadataOption {
	return func(o *CommonMetadata) {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[key] = value
	}
}

// WithResourceVersion sets the ResourceVersion to the supplied resourceVersion.
// This allows you to ensure that an update will fail if the version in the store doesn't match the one you supplied.
func WithResourceVersion(resourceVersion string) ObjectMetadataOption {
	return func(o *CommonMetadata) {
		o.ResourceVersion = resourceVersion
	}
}

// SimpleStore provides an easy key/value store interface for a specific Schema,
// allowing the user to work with the actual type in the Schema Object's spec,
// without casting in and out of the Object interface.
// It should be instantiated with NewSimpleStore.
type SimpleStore[SpecType any] struct {
	client Client
}

// NewSimpleStore creates a new SimpleStore for the provided Schema.
// It will error if the type of the Schema.ZeroValue().SpecObject() does not match the provided SpecType.
// It will also error if a client cannot be created from the generator, as unlike Store, the client is generated once
// and reused for all subsequent calls.
func NewSimpleStore[SpecType any](schema Schema, generator ClientGenerator) (*SimpleStore[SpecType], error) {
	if reflect.TypeOf(schema.ZeroValue().SpecObject()) != reflect.TypeOf(new(SpecType)).Elem() {
		return nil, fmt.Errorf(
			"SpecType '%s' does not match underlying schema.ZeroValue().SpecObject() type '%s'",
			reflect.TypeOf(new(SpecType)).Elem(),
			reflect.TypeOf(schema.ZeroValue().SpecObject()))
	}

	client, err := generator.ClientFor(schema)
	if err != nil {
		return nil, fmt.Errorf("error getting client from generator: %w", err)
	}

	return &SimpleStore[SpecType]{
		client: client,
	}, nil
}

// List returns a list of all resources of the Schema type in the provided namespace,
// optionally matching the provided filters.
func (s *SimpleStore[T]) List(ctx context.Context, namespace string, filters ...string) (
	[]SimpleStoreResource[T], error) {
	listObj, err := s.client.List(ctx, namespace, ListOptions{
		LabelFilters: filters,
	})
	if err != nil {
		return nil, err
	}
	items := listObj.ListItems()
	list := make([]SimpleStoreResource[T], len(items))
	for idx, item := range items {
		converted, err := s.cast(item)
		if err != nil {
			return nil, err
		}
		list[idx] = *converted
	}
	return list, nil
}

// Get gets an object with the provided identifier
func (s *SimpleStore[T]) Get(ctx context.Context, identifier Identifier) (*SimpleStoreResource[T], error) {
	obj, err := s.client.Get(ctx, identifier)
	if err != nil {
		return nil, err
	}
	return s.cast(obj)
}

// Add creates a new object
func (s *SimpleStore[T]) Add(ctx context.Context, identifier Identifier, obj T, opts ...ObjectMetadataOption) (
	*SimpleStoreResource[T], error) {
	object := SimpleObject[T]{
		Spec: obj,
	}
	for _, opt := range opts {
		opt(&object.CommonMeta)
	}
	ret, err := s.client.Create(ctx, identifier, &object, CreateOptions{})
	if err != nil {
		return nil, err
	}
	return s.cast(ret)
}

// Update updates the object with the provided identifier.
// If the WithResourceVersion option is used, the update will fail if the object's ResourceVersion in the store
// doesn't match the one provided in WithResourceVersion.
func (s *SimpleStore[T]) Update(ctx context.Context, identifier Identifier, obj T, opts ...ObjectMetadataOption) (
	*SimpleStoreResource[T], error) {
	object := SimpleObject[T]{
		Spec: obj,
	}
	// Before we can run the opts on the metadata, we need the current metadata
	// TODO: should this whole thing instead be serialized to a patch?
	// It could affect expected behavior, though, as WithResourceVersion makes sure it matches the RV you supply
	current, err := s.Get(ctx, identifier)
	if err != nil {
		return nil, err
	}
	object.CommonMeta = current.CommonMetadata
	customFields := current.CustomMetadata.MapFields()
	if len(customFields) > 0 {
		object.CustomMeta = make(SimpleCustomMetadata)
		for f, v := range customFields {
			object.CustomMeta[f] = v
		}
	}
	for _, opt := range opts {
		opt(&object.CommonMeta)
	}
	updateOptions := UpdateOptions{}
	if object.CommonMeta.ResourceVersion != "" {
		updateOptions.ResourceVersion = object.CommonMeta.ResourceVersion
	}
	object.CommonMeta.UpdateTimestamp = time.Now().UTC()
	ret, err := s.client.Update(ctx, identifier, &object, updateOptions)
	if err != nil {
		return nil, err
	}
	return s.cast(ret)
}

// UpdateSubresource updates a named subresource. Type compatibility is not checked for subresources.
// If the WithResourceVersion option is used, the update will fail if the object's ResourceVersion in the store
// doesn't match the one provided in WithResourceVersion.
func (s *SimpleStore[T]) UpdateSubresource(ctx context.Context, identifier Identifier, subresource SubresourceName,
	obj any) (*SimpleStoreResource[T], error) {
	if subresource == "" {
		return nil, fmt.Errorf("subresource may not be empty")
	}
	object := SimpleObject[T]{
		SubresourceMap: map[string]any{
			string(subresource): obj,
		},
	}
	ret, err := s.client.Update(ctx, identifier, &object, UpdateOptions{
		Subresource: string(subresource),
	})
	if err != nil {
		return nil, err
	}
	return s.cast(ret)
}

// Delete deletes a resource with the given identifier.
func (s *SimpleStore[T]) Delete(ctx context.Context, identifier Identifier) error {
	return s.client.Delete(ctx, identifier)
}

//nolint:revive
func (s *SimpleStore[T]) cast(obj Object) (*SimpleStoreResource[T], error) {
	spec, ok := obj.SpecObject().(T)
	if !ok {
		return nil, fmt.Errorf("returned object could not be cast to store's type")
	}
	return &SimpleStoreResource[T]{
		Spec:           spec,
		StaticMetadata: obj.StaticMetadata(),
		CommonMetadata: obj.CommonMetadata(),
		CustomMetadata: obj.CustomMetadata(),
		Subresources:   obj.Subresources(),
	}, nil
}
