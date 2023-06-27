package k8s

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

// SchemalessClient implements resource.SchemalessClient and allows for working with Schemas as kubernetes
// Custom Resource Definitions without being tied to a particular Schema (or GroupVerson).
// Since the largest unit a kubernetes rest.Interface can work with is a GroupVersion,
// SchemalessClient is actually an arbitrary number of kubernetes REST clients under-the-hood.
type SchemalessClient struct {
	// config REST config used to generate new clients
	kubeConfig rest.Config

	clientConfig ClientConfig

	// clients is the actual k8s clients, groupversion -> client
	clients map[string]*groupVersionClient

	mutex sync.Mutex
}

// NewSchemalessClient creates a new SchemalessClient using the provided rest.Config and ClientConfig.
func NewSchemalessClient(kubeConfig rest.Config, clientConfig ClientConfig) *SchemalessClient {
	return &SchemalessClient{
		kubeConfig:   kubeConfig,
		clientConfig: clientConfig,
		clients:      make(map[string]*groupVersionClient),
	}
}

// Get gets a resource from kubernetes with the Kind and GroupVersion determined from the FullIdentifier,
// using the namespace and name in FullIdentifier. If identifier.Plural is present, it will use that,
// otherwise, LOWER(identifier.Kind) + s is used for the resource.
// The returned resource is marshaled into `into`.
func (s *SchemalessClient) Get(ctx context.Context, identifier resource.FullIdentifier, into resource.Object) error {
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}
	return client.get(ctx, resource.Identifier{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
	}, s.getPlural(identifier), into)
}

// Create creates a new resource, and marshals the storage response (the created object) into the `into` field.
func (s *SchemalessClient) Create(ctx context.Context, identifier resource.FullIdentifier, obj resource.Object,
	_ resource.CreateOptions, into resource.Object) error {
	if obj == nil {
		return fmt.Errorf("obj cannot be nil")
	}
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}

	obj.SetStaticMetadata(resource.StaticMetadata{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
		Group:     identifier.Group,
		Version:   identifier.Version,
		Kind:      identifier.Kind,
	})

	return client.create(ctx, s.getPlural(identifier), obj, into)
}

// Update updates an existing resource, and marshals the updated version into the `into` field
func (s *SchemalessClient) Update(ctx context.Context, identifier resource.FullIdentifier, obj resource.Object,
	options resource.UpdateOptions, into resource.Object) error {
	if obj == nil {
		return fmt.Errorf("obj cannot be nil")
	}
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}

	obj.SetStaticMetadata(resource.StaticMetadata{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
		Group:     identifier.Group,
		Version:   identifier.Version,
		Kind:      identifier.Kind,
	})
	if options.ResourceVersion == "" {
		existingMd, err := client.getMetadata(ctx, resource.Identifier{
			Namespace: identifier.Namespace,
			Name:      identifier.Name,
		}, s.getPlural(identifier))
		if err != nil {
			return err
		}

		md := obj.CommonMetadata()
		md.ResourceVersion = existingMd.ObjectMetadata.ResourceVersion
		obj.SetCommonMetadata(md)
	} else {
		md := obj.CommonMetadata()
		md.ResourceVersion = options.ResourceVersion
		obj.SetCommonMetadata(md)
	}

	if options.Subresource != "" {
		return client.updateSubresource(ctx, s.getPlural(identifier), options.Subresource, obj, into, options)
	}
	return client.update(ctx, s.getPlural(identifier), obj, into, options)
}

// Patch performs a JSON Patch on the provided resource, and marshals the updated version into the `into` field
func (s *SchemalessClient) Patch(ctx context.Context, identifier resource.FullIdentifier, patch resource.PatchRequest,
	options resource.PatchOptions, into resource.Object) error {
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}

	return client.patch(ctx, resource.Identifier{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
	}, s.getPlural(identifier), patch, into, options)
}

// Delete deletes a resource identified by identifier
func (s *SchemalessClient) Delete(ctx context.Context, identifier resource.FullIdentifier) error {
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}

	return client.delete(ctx, resource.Identifier{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
	}, s.getPlural(identifier))
}

// List lists all resources that satisfy identifier, ignoring `Name`. The response is marshaled into `into`
func (s *SchemalessClient) List(ctx context.Context, identifier resource.FullIdentifier,
	options resource.ListOptions, into resource.ListObject, exampleListItem resource.Object) error {
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	client, err := s.getClient(identifier)
	if err != nil {
		return err
	}

	return client.list(ctx, identifier.Namespace, s.getPlural(identifier), into, options,
		func(bytes []byte) (resource.Object, error) {
			into := exampleListItem.Copy()
			err := rawToObject(bytes, into)
			return into, err
		})
}

// Watch watches all resources that satisfy the identifier, ignoring `Name`.
// The WatchResponse's WatchEvent Objects are created by unmarshaling into an object created by calling
// example.Copy().
func (s *SchemalessClient) Watch(ctx context.Context, identifier resource.FullIdentifier, options resource.WatchOptions,
	exampleObject resource.Object) (resource.WatchResponse, error) {
	if exampleObject == nil {
		return nil, fmt.Errorf("exampleItem cannot be nil")
	}
	client, err := s.getClient(identifier)
	if err != nil {
		return nil, err
	}
	return client.watch(ctx, identifier.Namespace, s.getPlural(identifier), exampleObject, options)
}

func (s *SchemalessClient) getClient(identifier resource.FullIdentifier) (*groupVersionClient, error) {
	gv := schema.GroupVersion{
		Group:   identifier.Group,
		Version: identifier.Version,
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if c, ok := s.clients[gv.Identifier()]; ok {
		return c, nil
	}

	s.kubeConfig.GroupVersion = &gv
	client, err := rest.RESTClientFor(&s.kubeConfig)
	if err != nil {
		return nil, err
	}
	s.clients[gv.Identifier()] = &groupVersionClient{
		client:  client,
		version: identifier.Version,
		config:  s.clientConfig,
	}
	return s.clients[gv.Identifier()], nil
}

//nolint:revive
func (s *SchemalessClient) getPlural(identifier resource.FullIdentifier) string {
	if identifier.Plural != "" {
		return identifier.Plural
	}
	return fmt.Sprintf("%ss", strings.ToLower(identifier.Kind))
}
