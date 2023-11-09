package resource

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// TODO: rewrite the godocs, this is all copied from crd/store.go

// SubresourceName is a string wrapper type for CRD subresource names
type SubresourceName string

// Subresource object names.
// As a "minimum supported set" in the SDK, we only present two predefined names,
// as only `status` and `scale` are allowed in CRDs,
// per https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#subresources
// Additional subresource names can be defined by implementers, but be aware of your storage system's restrictions.
const (
	SubresourceStatus = SubresourceName("status")
	SubresourceScale  = SubresourceName("scale")
)

type APIServerResponseError interface {
	error
	StatusCode() int
}

// Store presents Schema's resource Objects as a simple Key-Value store,
// abstracting the need to track clients or issue requests.
// If you wish to directly use a client managed by the store,
// the Client method returns the client used for a specific Schema.
type Store struct {
	clients ClientGenerator
	types   map[string]Schema
}

// NewStore creates a new SchemaStore, optionally initially registering all Schemas in the provided SchemaGroups
func NewStore(gen ClientGenerator, groups ...SchemaGroup) *Store {
	s := Store{
		clients: gen,
		types:   make(map[string]Schema),
	}
	for _, g := range groups {
		s.RegisterGroup(g)
	}
	return &s
}

// Register makes the store aware of a given Schema, and adds it to the list of `kind` values
// that can be supplied in calls. If a different schema with the same kind already exists, it will be overwritten.
func (s *Store) Register(sch Schema) {
	s.types[sch.Kind()] = sch
}

// RegisterGroup calls Register on each Schema in the provided SchemaGroup
func (s *Store) RegisterGroup(group SchemaGroup) {
	for _, sch := range group.Schemas() {
		s.Register(sch)
	}
}

// Get gets a resource with the provided kind and identifier
func (s *Store) Get(ctx context.Context, kind string, identifier Identifier) (Object, error) {
	client, err := s.getClient(kind)
	if err != nil {
		return nil, err
	}
	return client.Get(ctx, identifier)
}

// Add adds the provided resource.
// This method expects the provided Object's StaticMetadata to have the Name, Namespace, and Kind appropriately set.
// If they are not, no request will be issued to the underlying client, and an error will be returned.
func (s *Store) Add(ctx context.Context, obj Object) (Object, error) {
	if obj.StaticMetadata().Kind == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Kind must not be empty")
	}
	if obj.StaticMetadata().Namespace == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Namespace must not be empty")
	}
	if obj.StaticMetadata().Name == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Name must not be empty")
	}

	client, err := s.getClient(obj.StaticMetadata().Kind)
	if err != nil {
		return nil, err
	}

	return client.Create(ctx, Identifier{
		Namespace: obj.StaticMetadata().Namespace,
		Name:      obj.StaticMetadata().Name,
	}, obj, CreateOptions{})
}

// SimpleAdd is a variation of Add that has the caller explicitly supply Identifier and kind as arguments,
// which will overwrite whatever is set in the obj argument's metadata.
func (s *Store) SimpleAdd(ctx context.Context, kind string, identifier Identifier, obj Object) (Object, error) {
	client, err := s.getClient(kind)
	if err != nil {
		return nil, err
	}

	return client.Create(ctx, identifier, obj, CreateOptions{})
}

// Update updates the provided object.
// Keep in mind that an Update will completely overwrite the object,
// so nil or missing values will be removed, not ignored.
// It is usually best to use the result of a Get call, change the appropriate values, and then call Update with that.
// The update will fail if no ResourceVersion is provided, or if the ResourceVersion does not match the current one.
// It returns the updated Object from the storage system.
func (s *Store) Update(ctx context.Context, obj Object) (Object, error) {
	if obj.StaticMetadata().Kind == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Kind must not be empty")
	}
	if obj.StaticMetadata().Namespace == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Namespace must not be empty")
	}
	if obj.StaticMetadata().Name == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Name must not be empty")
	}

	md := obj.CommonMetadata()
	md.UpdateTimestamp = time.Now().UTC()
	obj.SetCommonMetadata(md)

	client, err := s.getClient(obj.StaticMetadata().Kind)
	if err != nil {
		return nil, err
	}

	return client.Update(ctx, Identifier{
		Namespace: obj.StaticMetadata().Namespace,
		Name:      obj.StaticMetadata().Name,
	}, obj, UpdateOptions{
		ResourceVersion: obj.CommonMetadata().ResourceVersion,
	})
}

// UpdateSubresource updates a subresource of an object.
// The provided obj parameter should be the subresource object, not the entire object.
// No checks are made that the provided object matches the subresource's definition.
func (s *Store) UpdateSubresource(
	ctx context.Context, kind string, identifier Identifier, subresourceName SubresourceName, obj any,
) (Object, error) {
	client, err := s.getClient(kind)
	if err != nil {
		return nil, err
	}
	if subresourceName == "" {
		return nil, fmt.Errorf("subresourceName cannot be empty")
	}

	toUpdate := SimpleObject[any]{
		SubresourceMap: map[string]any{
			string(subresourceName): obj,
		},
	}

	return client.Update(ctx, identifier, &toUpdate, UpdateOptions{
		Subresource: string(subresourceName),
	})
}

// Upsert updates/creates the provided object.
// Keep in mind that an Upsert will completely overwrite the object,
// so nil or missing values will be removed, not ignored.
// It is usually best to use the result of a Get call, change the appropriate values, and then call Update with that.
// The update will fail if no ResourceVersion is provided, or if the ResourceVersion does not match the current one.
// It returns the updated/created Object from the storage system.
func (s *Store) Upsert(ctx context.Context, obj Object) (Object, error) {
	if obj.StaticMetadata().Kind == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Kind must not be empty")
	}
	if obj.StaticMetadata().Namespace == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Namespace must not be empty")
	}
	if obj.StaticMetadata().Name == "" {
		return nil, fmt.Errorf("obj.StaticMetadata().Name must not be empty")
	}

	client, err := s.getClient(obj.StaticMetadata().Kind)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, obj.StaticMetadata().Identifier())

	if err != nil {
		cast, ok := err.(APIServerResponseError)
		if !ok {
			return nil, err
		} else if cast.StatusCode() != http.StatusNotFound {
			return nil, err
		}
	}

	if resp != nil {
		md := obj.CommonMetadata()
		md.UpdateTimestamp = time.Now().UTC()
		obj.SetCommonMetadata(md)
		return client.Update(ctx, Identifier{
			Namespace: obj.StaticMetadata().Namespace,
			Name:      obj.StaticMetadata().Name,
		}, obj, UpdateOptions{
			ResourceVersion: obj.CommonMetadata().ResourceVersion,
		})
	}
	return client.Create(ctx, Identifier{
		Namespace: obj.StaticMetadata().Namespace,
		Name:      obj.StaticMetadata().Name,
	}, obj, CreateOptions{})
}

// Delete deletes a resource with the given Identifier and kind.
func (s *Store) Delete(ctx context.Context, kind string, identifier Identifier) error {
	client, err := s.getClient(kind)
	if err != nil {
		return err
	}

	return client.Delete(ctx, identifier)
}

// ForceDelete deletes a resource with the given Identifier and kind, ignores client 404 errors.
func (s *Store) ForceDelete(ctx context.Context, kind string, identifier Identifier) error {
	client, err := s.getClient(kind)
	if err != nil {
		return err
	}

	err = client.Delete(ctx, identifier)

	if cast, ok := err.(APIServerResponseError); ok && cast.StatusCode() == http.StatusNotFound {
		return nil
	}
	return err
}

// List lists all resources of kind in the provided namespace, with optional label filters.
func (s *Store) List(ctx context.Context, kind string, namespace string, filters ...string) (ListObject, error) {
	client, err := s.getClient(kind)
	if err != nil {
		return nil, err
	}

	return client.List(ctx, namespace, ListOptions{
		LabelFilters: filters,
	})
}

// Client returns a Client for the provided kind, if that kind is tracked by the Store
func (s *Store) Client(kind string) (Client, error) {
	client, err := s.getClient(kind)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (s *Store) getClient(kind string) (Client, error) {
	schema, ok := s.types[kind]
	if !ok {
		return nil, fmt.Errorf("resource kind '%s' is not registered in store", kind)
	}
	client, err := s.clients.ClientFor(schema)
	if err != nil {
		return nil, err
	}
	return client, nil
}
