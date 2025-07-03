package gateway

import (
	"fmt"
	"github.com/grafana/grafana-app-sdk/app"
	codecgen "github.com/grafana/grafana-app-sdk/graphql/codegen"
	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
)

// AppProviderRegistry maintains a registry of app providers that can be
// discovered for GraphQL subgraph support.
type AppProviderRegistry struct {
	providers map[string]app.Provider
	gateway   *FederatedGateway
}

// NewAppProviderRegistry creates a new registry for app providers
func NewAppProviderRegistry() *AppProviderRegistry {
	return &AppProviderRegistry{
		providers: make(map[string]app.Provider),
		gateway:   NewFederatedGateway(GatewayConfig{}),
	}
}

// RegisterProvider registers an app provider with the registry.
// If the provider implements GraphQLSubgraphProvider, it will be
// automatically discovered and registered with the federated gateway.
func (r *AppProviderRegistry) RegisterProvider(name string, provider app.Provider) error {
	r.providers[name] = provider

	// Check if the provider supports GraphQL subgraphs
	if graphqlProvider, ok := provider.(subgraph.GraphQLSubgraphProvider); ok {
		if err := r.registerGraphQLSubgraph(name, graphqlProvider); err != nil {
			log.Printf("Warning: Failed to register GraphQL subgraph for provider %s: %v", name, err)
			// Don't fail the registration if GraphQL setup fails
		}
	}

	return nil
}

// registerGraphQLSubgraph registers a GraphQL subgraph from a provider
func (r *AppProviderRegistry) registerGraphQLSubgraph(name string, provider subgraph.GraphQLSubgraphProvider) error {
	// Get the subgraph from the provider
	sg, err := provider.GetGraphQLSubgraph()
	if err != nil {
		return fmt.Errorf("failed to get GraphQL subgraph from provider %s: %w", name, err)
	}

	// Determine the group version from the subgraph
	gv := sg.GetGroupVersion()

	// Register the subgraph with the federated gateway
	if err := r.gateway.RegisterSubgraph(gv, sg); err != nil {
		return fmt.Errorf("failed to register subgraph for provider %s: %w", name, err)
	}

	log.Printf("Successfully registered GraphQL subgraph for provider %s (group: %s, version: %s)",
		name, gv.Group, gv.Version)
	return nil
}

// GetProvider retrieves a registered provider by name
func (r *AppProviderRegistry) GetProvider(name string) (app.Provider, bool) {
	provider, exists := r.providers[name]
	return provider, exists
}

// GetFederatedGateway returns the federated gateway with all registered subgraphs
func (r *AppProviderRegistry) GetFederatedGateway() *FederatedGateway {
	return r.gateway
}

// DiscoverAndRegisterSubgraphs manually discovers and registers GraphQL subgraphs
// from all registered providers. This is useful for dynamic registration scenarios.
func (r *AppProviderRegistry) DiscoverAndRegisterSubgraphs() error {
	var errors []error

	for name, provider := range r.providers {
		if graphqlProvider, ok := provider.(subgraph.GraphQLSubgraphProvider); ok {
			if err := r.registerGraphQLSubgraph(name, graphqlProvider); err != nil {
				errors = append(errors, fmt.Errorf("provider %s: %w", name, err))
			}
		}
	}

	if len(errors) > 0 {
		// Return the first error, but log all errors
		for i, err := range errors {
			if i == 0 {
				log.Printf("GraphQL subgraph registration errors:")
			}
			log.Printf("  - %v", err)
		}
		return errors[0]
	}

	return nil
}

// GetRegisteredSubgraphs returns information about all registered subgraphs
func (r *AppProviderRegistry) GetRegisteredSubgraphs() []SubgraphInfo {
	var infos []SubgraphInfo

	for _, sg := range r.gateway.subgraphs {
		// Convert resource.Kind slice to string slice
		kinds := make([]string, len(sg.GetKinds()))
		for i, kind := range sg.GetKinds() {
			kinds[i] = kind.Kind()
		}

		// Get the actual GroupVersion from the subgraph
		gv := sg.GetGroupVersion()

		infos = append(infos, SubgraphInfo{
			GroupVersion: gv,
			Kinds:        kinds,
			NumResolvers: len(sg.GetResolvers()),
		})
	}

	return infos
}

// SubgraphInfo provides information about a registered subgraph
type SubgraphInfo struct {
	GroupVersion schema.GroupVersion `json:"groupVersion"`
	Kinds        []string            `json:"kinds"`
	NumResolvers int                 `json:"numResolvers"`
}

// AutoDiscovery provides a simple way to set up auto-discovery of GraphQL subgraphs
// from a list of app providers.
func AutoDiscovery(providers ...app.Provider) (*AppProviderRegistry, error) {
	registry := NewAppProviderRegistry()

	for i, provider := range providers {
		name := fmt.Sprintf("provider-%d", i)

		// If provider has a name method, use that instead
		if namedProvider, ok := provider.(interface{ Name() string }); ok {
			name = namedProvider.Name()
		}

		if err := registry.RegisterProvider(name, provider); err != nil {
			return nil, fmt.Errorf("failed to register provider %s: %w", name, err)
		}
	}

	return registry, nil
}

// GetMeshStyleGateway returns a new MeshStyleGateway with all registered subgraphs
func (r *AppProviderRegistry) GetMeshStyleGateway() *MeshStyleGateway {
	// Create a new MeshStyleGateway
	meshGateway := NewMeshStyleGateway(MeshGatewayConfig{})

	// Register all GraphQL-capable providers with the mesh gateway
	for name, provider := range r.providers {
		if graphqlProvider, ok := provider.(subgraph.GraphQLSubgraphProvider); ok {
			// Get the subgraph from the provider
			sg, err := graphqlProvider.GetGraphQLSubgraph()
			if err != nil {
				log.Printf("Warning: Failed to get GraphQL subgraph from provider %s: %v", name, err)
				continue
			}

			// Check if it's a CUE-aware subgraph
			if cueAwareSg, ok := sg.(CUEAwareSubgraph); ok {
				// Register with the mesh gateway
				gv := sg.GetGroupVersion()
				if err := meshGateway.RegisterSubgraph(gv, cueAwareSg); err != nil {
					log.Printf("Warning: Failed to register subgraph for provider %s: %v", name, err)
				}
			} else {
				// Create a wrapper for non-CUE-aware subgraphs
				wrapper := &cueAwareSubgraphWrapper{
					GraphQLSubgraph: sg,
					relationships:   []codecgen.MeshRelationshipConfig{}, // Empty for now
				}
				gv := sg.GetGroupVersion()
				if err := meshGateway.RegisterSubgraph(gv, wrapper); err != nil {
					log.Printf("Warning: Failed to register wrapped subgraph for provider %s: %v", name, err)
				}
			}
		}
	}

	return meshGateway
}

// cueAwareSubgraphWrapper wraps a regular GraphQLSubgraph to make it CUE-aware
type cueAwareSubgraphWrapper struct {
	subgraph.GraphQLSubgraph
	relationships []codecgen.MeshRelationshipConfig
}

// GetRelationships implements the CUEAwareSubgraph interface
func (w *cueAwareSubgraphWrapper) GetRelationships() []codecgen.MeshRelationshipConfig {
	return w.relationships
}
