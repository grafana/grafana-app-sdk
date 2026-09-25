package plugin

import (
	"context"
	"os"

	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
)

// defaultNamespace is the namespace used when API_ACCESS_NAMESPACE is not set.
const defaultNamespace = "default"

// BuildClientGenerator returns a ClientGenerator that the plugin can use to access resources.
// Currently its primary purpose is to work around NamespaceAll listing for local development,
// which is not supported by standalone instances.
func BuildClientGenerator(kubeConfig rest.Config) resource.ClientGenerator {
	registry := k8s.NewClientRegistry(kubeConfig, k8s.ClientConfig{})

	// If we're using token exchange, then we can issue true multi-tenant requests.
	if os.Getenv("API_ACCESS_TOKEN_EXCHANGE_URL") != "" {
		return registry
	}

	// Otherwise, assume we're single tenant. In this case we must work around the inability
	// to list/watch over all namespaces by pinning requests to the tenant namespace.
	namespace := os.Getenv("API_ACCESS_NAMESPACE")
	if namespace == "" {
		namespace = defaultNamespace
	}
	return &namespacePinningClientGenerator{
		ClientGenerator: registry,
		namespace:       namespace,
	}
}

var _ resource.ClientGenerator = (*namespacePinningClientGenerator)(nil)
var _ resource.Client = (*namespacePinningClient)(nil)

// namespacePinningClientGenerator wraps a resource.ClientGenerator so every Client it produces
// lists/watches against a fixed namespace when the caller asks for NamespaceAll.
// Get/Create/Update/Patch/Delete are left untouched: those always carry a real namespace already.
type namespacePinningClientGenerator struct {
	resource.ClientGenerator
	namespace string
}

func (g *namespacePinningClientGenerator) ClientFor(kind resource.Kind) (resource.Client, error) {
	client, err := g.ClientGenerator.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	return &namespacePinningClient{Client: client, namespace: g.namespace}, nil
}

// namespacePinningClient wraps a resource.Client, replacing NamespaceAll in List/ListInto/Watch
// with the pinned namespace.
type namespacePinningClient struct {
	resource.Client
	namespace string
}

func (c *namespacePinningClient) pin(ns string) string {
	if ns == resource.NamespaceAll {
		return c.namespace
	}
	return ns
}

func (c *namespacePinningClient) List(ctx context.Context, ns string, options resource.ListOptions) (resource.ListObject, error) {
	return c.Client.List(ctx, c.pin(ns), options)
}

func (c *namespacePinningClient) ListInto(ctx context.Context, ns string, options resource.ListOptions, into resource.ListObject) error {
	return c.Client.ListInto(ctx, c.pin(ns), options, into)
}

func (c *namespacePinningClient) Watch(ctx context.Context, ns string, options resource.WatchOptions) (resource.WatchResponse, error) {
	return c.Client.Watch(ctx, c.pin(ns), options)
}
