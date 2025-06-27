package subgraph

import (
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GraphQLSubgraphProvider is an optional interface that app providers can implement
// to provide GraphQL federation support. This allows apps to expose their kinds
// through GraphQL without requiring changes to the core App interfaces.
type GraphQLSubgraphProvider interface {
	// GetGraphQLSubgraph returns the GraphQL subgraph for this app.
	// The subgraph includes auto-generated GraphQL schema and resolvers
	// based on the app's managed kinds.
	GetGraphQLSubgraph() (GraphQLSubgraph, error)
}

// SubgraphProviderConfig contains configuration needed to create a GraphQL subgraph
// from an app provider's managed kinds and storage.
type SubgraphProviderConfig struct {
	// GroupVersion is the API group/version for this app's kinds
	GroupVersion schema.GroupVersion
	// Kinds are the resource kinds managed by this app
	Kinds []resource.Kind
	// StorageGetter provides storage access for resolving GraphQL queries
	StorageGetter func(gvr schema.GroupVersionResource) Storage
}

// CreateSubgraphFromConfig is a helper function that creates a GraphQL subgraph
// from the provided configuration. This is typically used by app providers
// in their GetGraphQLSubgraph implementation.
func CreateSubgraphFromConfig(config SubgraphProviderConfig) (GraphQLSubgraph, error) {
	return New(SubgraphConfig{
		GroupVersion:  config.GroupVersion,
		Kinds:         config.Kinds,
		StorageGetter: config.StorageGetter,
	})
}
