package subgraph

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
)

// GraphQLSubgraph represents a GraphQL subgraph provided by an App Platform app
type GraphQLSubgraph interface {
	// GetSchema returns the GraphQL schema for this subgraph
	GetSchema() *graphql.Schema

	// GetResolvers returns the resolver functions for this subgraph
	GetResolvers() ResolverMap

	// GetGroupVersion returns the Kubernetes group/version this subgraph handles
	GetGroupVersion() schema.GroupVersion

	// GetKinds returns the resource kinds handled by this subgraph
	GetKinds() []resource.Kind
}

// ResolverMap maps GraphQL field names to resolver functions
type ResolverMap map[string]interface{}

// SubgraphConfig holds configuration for creating a subgraph
type SubgraphConfig struct {
	GroupVersion    schema.GroupVersion
	Kinds           []resource.Kind
	StorageGetter   func(gvr schema.GroupVersionResource) Storage
	CustomResolvers ResolverMap
}

// Storage interface abstracts the underlying storage for resources
// This allows subgraphs to delegate to existing REST storage implementations
type Storage interface {
	Get(ctx context.Context, namespace, name string) (resource.Object, error)
	List(ctx context.Context, namespace string, options ListOptions) (resource.ListObject, error)
	Create(ctx context.Context, namespace string, obj resource.Object) (resource.Object, error)
	Update(ctx context.Context, namespace, name string, obj resource.Object) (resource.Object, error)
	Delete(ctx context.Context, namespace, name string) error
}

// ListOptions provides filtering and pagination options for list operations
type ListOptions struct {
	LabelSelector string
	FieldSelector string
	Limit         int64
	Continue      string
}

// subgraph is the default implementation of GraphQLSubgraph
type subgraph struct {
	schema       *graphql.Schema
	resolvers    ResolverMap
	groupVersion schema.GroupVersion
	kinds        []resource.Kind
}

// New creates a new GraphQL subgraph with the given configuration
func New(config SubgraphConfig) (GraphQLSubgraph, error) {
	// Create the subgraph instance
	sg := &subgraph{
		groupVersion: config.GroupVersion,
		kinds:        config.Kinds,
		resolvers:    make(ResolverMap),
	}

	// Generate schema and resolvers from kinds
	schema, resolvers, err := generateSchemaAndResolvers(config)
	if err != nil {
		return nil, err
	}

	sg.schema = schema
	sg.resolvers = resolvers

	// Add any custom resolvers
	for name, resolver := range config.CustomResolvers {
		sg.resolvers[name] = resolver
	}

	return sg, nil
}

func (s *subgraph) GetSchema() *graphql.Schema {
	return s.schema
}

func (s *subgraph) GetResolvers() ResolverMap {
	return s.resolvers
}

func (s *subgraph) GetGroupVersion() schema.GroupVersion {
	return s.groupVersion
}

func (s *subgraph) GetKinds() []resource.Kind {
	return s.kinds
}

// generateSchemaAndResolvers creates GraphQL schema and resolvers from resource kinds
// This uses the codegen package to generate schemas from CUE kinds
func generateSchemaAndResolvers(config SubgraphConfig) (*graphql.Schema, ResolverMap, error) {
	// Use the new GraphQL generator to create schema and resolvers
	generator := NewGraphQLGenerator(config.Kinds, config.GroupVersion, config.StorageGetter)

	schema, err := generator.GenerateSchema()
	if err != nil {
		return nil, nil, err
	}

	resolvers := generator.GenerateResolvers()
	return schema, resolvers, nil
}

// NewGraphQLGenerator creates a new GraphQL generator (import from codegen package)
// This is a helper function to avoid import cycles
func NewGraphQLGenerator(kinds []resource.Kind, gv schema.GroupVersion, storageGetter func(gvr schema.GroupVersionResource) Storage) GraphQLGeneratorInterface {
	// We'll implement this to delegate to the codegen package
	// For now, return a simple implementation
	return &simpleGenerator{
		kinds:         kinds,
		groupVersion:  gv,
		storageGetter: storageGetter,
	}
}

// GraphQLGeneratorInterface defines the interface for GraphQL generation
type GraphQLGeneratorInterface interface {
	GenerateSchema() (*graphql.Schema, error)
	GenerateResolvers() ResolverMap
}

// simpleGenerator is a temporary implementation until we can properly integrate with codegen
type simpleGenerator struct {
	kinds         []resource.Kind
	groupVersion  schema.GroupVersion
	storageGetter func(gvr schema.GroupVersionResource) Storage
}

func (g *simpleGenerator) GenerateSchema() (*graphql.Schema, error) {
	// Create a minimal schema for now
	queryFields := make(graphql.Fields)

	// Add a hello field to verify the schema works
	queryFields["hello"] = &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "Hello from " + g.groupVersion.String(), nil
		},
	}

	// Add basic fields for each kind
	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)

		// Add a simple query field for this kind
		queryFields[lowercaseKind] = &graphql.Field{
			Type: graphql.String,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				return fmt.Sprintf("Resource type: %s", kindName), nil
			},
		}
	}

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
	})
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

func (g *simpleGenerator) GenerateResolvers() ResolverMap {
	resolvers := make(ResolverMap)

	// Add hello resolver
	resolvers["hello"] = func(p graphql.ResolveParams) (interface{}, error) {
		return "Hello from " + g.groupVersion.String(), nil
	}

	// Add basic resolvers for each kind
	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)

		resolvers[lowercaseKind] = func(p graphql.ResolveParams) (interface{}, error) {
			return fmt.Sprintf("Resource type: %s", kindName), nil
		}
	}

	return resolvers
}

// HTTPHandler creates an HTTP handler for this subgraph
// This is useful for testing individual subgraphs
func HTTPHandler(sg GraphQLSubgraph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Basic GraphQL HTTP handler implementation
		// This will be enhanced with proper request parsing, context handling, etc.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": {"hello": "GraphQL subgraph is working"}}`))
	}
}
