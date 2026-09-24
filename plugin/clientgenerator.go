package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
)

// BuildClientGenerator returns a ClientGenerator that the plugin can use to access resources.
// Currently it's primary purpose is to workaround NamespaceAll listing for local development,
// which is not supported by standalone instances.
func BuildClientGenerator(kubeConfig rest.Config) resource.ClientGenerator {
	registry := k8s.NewClientRegistry(kubeConfig, k8s.ClientConfig{})

	// If we're using token exchange, then can we issue true multi-tenant requests.
	if os.Getenv("API_ACCESS_TOKEN_EXCHANGE_URL") != "" {
		return registry
	}

	// Otherwise, assume we're single tenant. In this case we must workaround the inability
	// to list/watch over all namespaces by pinning requests to the tenant namespace.
	return &namespacePinningClientGenerator{
		ClientGenerator: registry,
		resolver:        &namespaceResolver{kubeConfig: kubeConfig},
	}
}

var _ resource.ClientGenerator = (*namespacePinningClientGenerator)(nil)
var _ resource.Client = (*namespacePinningClient)(nil)

// namespacePinningClientGenerator wraps a resource.ClientGenerator so every Client it produces
// lists/watches against a namespace resolved once by GetNamespace, rather than the namespace the
// caller passes in. Get/Create/Update/Patch/Delete are left untouched: those always carry a real
// namespace already, either given deliberately by the caller or derived from an incoming
// already-routed request. List and Watch are the only calls an informer or poller issues without
// such a namespace on hand, since nothing routed the request for them first.
type namespacePinningClientGenerator struct {
	resource.ClientGenerator
	resolver *namespaceResolver
}

func (g *namespacePinningClientGenerator) ClientFor(kind resource.Kind) (resource.Client, error) {
	client, err := g.ClientGenerator.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	return &namespacePinningClient{Client: client, resolver: g.resolver}, nil
}

// namespacePinningClient wraps a resource.Client, resolving the namespace passed to List/ListInto/
// Watch via the shared namespaceResolver instead of trusting the caller's namespace argument.
type namespacePinningClient struct {
	resource.Client
	resolver *namespaceResolver
}

func (c *namespacePinningClient) List(ctx context.Context, ns string, options resource.ListOptions) (resource.ListObject, error) {
	if ns == resource.NamespaceAll {
		var err error
		ns, err = c.resolver.resolve(ctx)
		if err != nil {
			return nil, err
		}
	}
	return c.Client.List(ctx, ns, options)
}

func (c *namespacePinningClient) ListInto(ctx context.Context, ns string, options resource.ListOptions, into resource.ListObject) error {
	if ns == resource.NamespaceAll {
		var err error
		ns, err = c.resolver.resolve(ctx)
		if err != nil {
			return err
		}
	}
	return c.Client.ListInto(ctx, ns, options, into)
}

func (c *namespacePinningClient) Watch(ctx context.Context, ns string, options resource.WatchOptions) (resource.WatchResponse, error) {
	if ns == resource.NamespaceAll {
		var err error
		ns, err = c.resolver.resolve(ctx)
		if err != nil {
			return nil, err
		}
	}
	return c.Client.Watch(ctx, ns, options)
}

// namespaceResolver resolves and caches the namespace for the Grafana instance by reading
// /api/frontend/settings. This is temporary until a better solution is found, but there is
// currently no other way for a plugin obtain it.
type namespaceResolver struct {
	kubeConfig rest.Config

	done      atomic.Bool
	mtx       sync.Mutex
	namespace string
}

func (r *namespaceResolver) resolve(ctx context.Context) (string, error) {
	if r.done.Load() {
		return r.namespace, nil
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	namespace, err := getNamespace(ctx, r.kubeConfig)
	if err != nil {
		return namespace, err
	}

	r.namespace = namespace
	r.done.Store(true)

	return namespace, nil
}

func getNamespace(ctx context.Context, kubeConfig rest.Config) (string, error) {
	httpClient, err := rest.HTTPClientFor(&kubeConfig)
	if err != nil {
		return "", fmt.Errorf("frontend settings: building HTTP client: %w", err)
	}

	url := strings.TrimSuffix(kubeConfig.Host, "/") + "/api/frontend/settings"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("frontend settings: building request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("frontend settings: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("frontend settings: unexpected status %d", resp.StatusCode)
	}

	// frontendSettings is the subset of Grafana's /api/frontend/settings response that this
	// package cares about.
	var settings struct {
		Namespace string `json:"namespace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return "", fmt.Errorf("frontend settings: decoding response: %w", err)
	}
	if settings.Namespace == "" {
		return "", errors.New("frontend settings: response had no namespace")
	}

	return settings.Namespace, nil
}
