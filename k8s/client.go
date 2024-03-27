package k8s

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Client is a kubernetes-specific implementation of resource.Client, using custom resource definitions.
// A Client is specific to the Schema it was created with.
// New Clients should only be created via the ClientRegistry.ClientFor method.
type Client struct {
	client *groupVersionClient
	schema resource.Schema
	config ClientConfig
}

// ClientConfig is the configuration object for creating Clients.
type ClientConfig struct {
	// CustomMetadataIsAnyType tells the Client if the custom metadata of an object can be of any type, or is limited to only strings.
	// By default, this is false, with which the client will assume custom metadata is only a string type,
	// and not invoke reflection to turn the type into a string when encoding to the underlying kubernetes annotation storage.
	// If set to true, the client will use reflection to get the type of each custom metadata field,
	// and convert it into a string (structs and lists will be converted into stringified JSON).
	// Keep in mind that the metadata bytes blob used in unmarshaling will always have custom metadata as string types,
	// regardless of how this value is set, so make sure your resource.Object implementations can handle
	// turning strings into non-string types when unmarshaling if you plan to have custom metadata keys which have non-string values.
	CustomMetadataIsAnyType bool

	MetricsConfig metrics.Config
}

// DefaultClientConfig returns a ClientConfig using defaults that assume you have used the SDK codegen tooling
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		CustomMetadataIsAnyType: false,
		MetricsConfig:           metrics.DefaultConfig(""),
	}
}

// List lists resources in the provided namespace.
// For resources with a schema.Scope() of ClusterScope, `namespace` must be resource.NamespaceAll
func (c *Client) List(
	ctx context.Context, namespace string, options resource.ListOptions,
) (resource.ListObject, error) {
	into := listImpl{
		items: make([]resource.Object, 0, options.Limit),
	}

	if err := c.ListInto(ctx, namespace, options, &into); err != nil {
		return nil, err
	}

	return &into, nil
}

// ListInto lists resources in the provided namespace, and unmarshals the response into the provided resource.ListObject
func (c *Client) ListInto(
	ctx context.Context, namespace string, options resource.ListOptions, into resource.ListObject,
) error {
	if c.schema.Scope() == resource.ClusterScope && namespace != resource.NamespaceAll {
		return fmt.Errorf(
			"cannot list resources with schema scope \"%s\" in namespace \"%s\", must be NamespaceAll (\"%s\")",
			resource.ClusterScope, namespace, resource.NamespaceAll,
		)
	}

	return c.client.list(
		ctx, namespace, c.schema.Plural(), into, options, func(bytes []byte) (resource.Object, error) {
			into := c.schema.ZeroValue()
			err := rawToObject(bytes, into)
			return into, err
		},
	)
}

// Get gets a resource of the client's internal Schema-derived kind, with the provided identifier
func (c *Client) Get(ctx context.Context, identifier resource.Identifier) (resource.Object, error) {
	into := c.schema.ZeroValue()
	err := c.GetInto(ctx, identifier, into)
	if err != nil {
		return nil, err
	}
	return into, nil
}

// GetInto gets a resource of the client's internal Schema-derived kind, with the provided identifier,
// and marshals it into `into`
func (c *Client) GetInto(ctx context.Context, identifier resource.Identifier, into resource.Object) error {
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	return c.client.get(ctx, identifier, c.schema.Plural(), into)
}

// Create creates a new resource, and returns the resulting created resource
func (c *Client) Create(ctx context.Context, identifier resource.Identifier, obj resource.Object,
	options resource.CreateOptions) (resource.Object, error) {
	into := c.schema.ZeroValue()
	err := c.CreateInto(ctx, identifier, obj, options, into)
	if err != nil {
		return nil, err
	}
	return into, nil
}

// CreateInto creates a new resource, and marshals the resulting created resource into `into`
func (c *Client) CreateInto(ctx context.Context, identifier resource.Identifier, obj resource.Object,
	_ resource.CreateOptions, into resource.Object) error {
	if obj == nil {
		return fmt.Errorf("obj cannot be nil")
	}
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	if c.schema.Scope() == resource.NamespacedScope && identifier.Namespace == resource.NamespaceAll {
		return fmt.Errorf("cannot create a resource with schema scope \"%s\" in NamespaceAll (\"%s\")", resource.NamespacedScope, resource.NamespaceAll)
	} else if c.schema.Scope() == resource.ClusterScope && identifier.Namespace != resource.NamespaceAll {
		return fmt.Errorf("cannot create a resource with schema scope \"%s\" in namespace \"%s\", must be NamespaceAll (\"%s\"",
			resource.ClusterScope, identifier.Namespace, resource.NamespaceAll)
	}
	// Check if we need to add metadata to the object
	obj.SetStaticMetadata(resource.StaticMetadata{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
		Group:     c.schema.Group(),
		Version:   c.schema.Version(),
		Kind:      c.schema.Kind(),
	})

	return c.client.create(ctx, c.schema.Plural(), obj, into)
}

// Update updates the provided resource, and returns the updated resource from kubernetes
func (c *Client) Update(ctx context.Context, identifier resource.Identifier, obj resource.Object,
	options resource.UpdateOptions) (resource.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("obj cannot be nil")
	}
	into := c.schema.ZeroValue()
	err := c.UpdateInto(ctx, identifier, obj, options, into)
	if err != nil {
		return nil, err
	}
	return into, nil
}

// UpdateInto updates the provided resource, and marshals the updated resource from kubernetes into `into`
func (c *Client) UpdateInto(ctx context.Context, identifier resource.Identifier, obj resource.Object,
	options resource.UpdateOptions, into resource.Object) error {
	if obj == nil {
		return fmt.Errorf("obj cannot be nil")
	}
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	obj.SetStaticMetadata(resource.StaticMetadata{
		Namespace: identifier.Namespace,
		Name:      identifier.Name,
		Group:     c.schema.Group(),
		Version:   c.schema.Version(),
		Kind:      c.schema.Kind(),
	})

	if options.ResourceVersion == "" {
		existingMd, err := c.client.getMetadata(ctx, identifier, c.schema.Plural())
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
		return c.client.updateSubresource(ctx, c.schema.Plural(), options.Subresource, obj, into, options)
	}
	return c.client.update(ctx, c.schema.Plural(), obj, into, options)
}

// Patch performs a JSON Patch on the provided resource, and returns the updated object
func (c *Client) Patch(ctx context.Context, identifier resource.Identifier, patch resource.PatchRequest,
	options resource.PatchOptions) (resource.Object, error) {
	into := c.schema.ZeroValue()
	err := c.PatchInto(ctx, identifier, patch, options, into)
	if err != nil {
		return nil, err
	}
	return into, nil
}

// PatchInto performs a JSON Patch on the provided resource, and marshals the updated version into the `into` field
func (c *Client) PatchInto(ctx context.Context, identifier resource.Identifier, patch resource.PatchRequest,
	options resource.PatchOptions, into resource.Object) error {
	return c.client.patch(ctx, identifier, c.schema.Plural(), patch, into, options)
}

// Delete deletes the specified resource
func (c *Client) Delete(ctx context.Context, identifier resource.Identifier) error {
	return c.client.delete(ctx, identifier, c.schema.Plural())
}

// Watch makes a watch request for the namespace, and returns a WatchResponse which wraps a kubernetes
// watch.Interface. The underlying watch.Interface can be accessed using KubernetesWatch()
func (c *Client) Watch(ctx context.Context, namespace string, options resource.WatchOptions) (
	resource.WatchResponse, error) {
	if c.schema.Scope() == resource.ClusterScope && namespace != resource.NamespaceAll {
		return nil, fmt.Errorf("cannot watch resources with schema scope \"%s\" in namespace \"%s\", must be NamespaceAll (\"%s\")",
			resource.ClusterScope, namespace, resource.NamespaceAll)
	}
	return c.client.watch(ctx, namespace, c.schema.Plural(), c.schema.ZeroValue(), options)
}

// PrometheusCollectors returns the prometheus collectors used by this Client
// for registration with a Prometheus exporter.
func (c *Client) PrometheusCollectors() []prometheus.Collector {
	return c.client.metrics()
}

// RESTClient returns the underlying rest.Interface used to communicate with kubernetes
func (c *Client) RESTClient() rest.Interface {
	return c.client.client
}

type listImpl struct {
	lmd   resource.ListMetadata
	items []resource.Object
}

func (l *listImpl) ListMetadata() resource.ListMetadata {
	return l.lmd
}

func (l *listImpl) SetListMetadata(md resource.ListMetadata) {
	l.lmd = md
}

func (l *listImpl) ListItems() []resource.Object {
	return l.items
}

func (l *listImpl) SetItems(items []resource.Object) {
	l.items = items
}

func (l *listImpl) Clear() {
	l.lmd = resource.ListMetadata{}
	clear(l.items)
	l.items = l.items[:0]
}

func (l *listImpl) AppendItem(item resource.Object) {
	if l.items == nil {
		l.items = make([]resource.Object, 0, 2)
	}

	l.items = append(l.items, item)
}
