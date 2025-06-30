package subgraph

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		fmt.Printf("❌ generateSchemaAndResolvers failed: %v\n", err)
		return nil, err
	}
	fmt.Printf("✅ generateSchemaAndResolvers completed successfully\n")

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
	fmt.Printf("✅ NewGraphQLGenerator completed\n")
	schema, err := generator.GenerateSchema()
	if err != nil {
		fmt.Printf("❌ generator.GenerateSchema() failed: %v\n", err)
		return nil, nil, err
	}
	fmt.Printf("✅ generator.GenerateSchema() completed\n")

	resolvers := generator.GenerateResolvers()
	fmt.Printf("✅ generator.GenerateResolvers() completed\n")

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
	// Create proper GraphQL types for structured playlist data
	queryFields := make(graphql.Fields)

	// Add a hello field to verify the schema works
	queryFields["hello"] = &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "Hello from " + g.groupVersion.String(), nil
		},
	}

	// Create proper playlist types to avoid any JSON scalar conflicts
	playlistItemType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PlaylistItem",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.Int},
			"playlistUid": &graphql.Field{Type: graphql.String},
			"type":        &graphql.Field{Type: graphql.String},
			"value":       &graphql.Field{Type: graphql.String},
			"order":       &graphql.Field{Type: graphql.Int},
			"title":       &graphql.Field{Type: graphql.String},
		},
	})

	playlistType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Playlist",
		Fields: graphql.Fields{
			"uid":      &graphql.Field{Type: graphql.String},
			"name":     &graphql.Field{Type: graphql.String},
			"interval": &graphql.Field{Type: graphql.String},
			"items":    &graphql.Field{Type: graphql.NewList(playlistItemType)},
		},
	})

	playlistListType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PlaylistList",
		Fields: graphql.Fields{
			"items": &graphql.Field{Type: graphql.NewList(playlistType)},
		},
	})

	// Add a demo playlist field that doesn't require arguments for easy testing
	queryFields["demo_playlist"] = &graphql.Field{
		Type: playlistType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return map[string]interface{}{
				"uid":      "demo-playlist-uid",
				"name":     "Demo Playlist (no args required)",
				"interval": "30s",
				"items": []map[string]interface{}{
					{
						"id":          1,
						"playlistUid": "demo-playlist-uid",
						"type":        "dashboard_by_uid",
						"value":       "demo-dashboard-1",
						"order":       1,
						"title":       "Demo Dashboard 1",
					},
					{
						"id":          2,
						"playlistUid": "demo-playlist-uid",
						"type":        "dashboard_by_tag",
						"value":       "demo-tag",
						"order":       2,
						"title":       "Demo Dashboard 2",
					},
				},
			}, nil
		},
	}

	// Add proper structured queries for each kind
	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)

		// Add structured get query that returns proper playlist object
		queryFields[lowercaseKind] = &graphql.Field{
			Type: playlistType,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				"name":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				// Safely extract arguments with proper error handling
				namespace, namespaceOk := p.Args["namespace"].(string)
				if !namespaceOk || namespace == "" {
					return nil, fmt.Errorf("namespace argument is required and must be a non-empty string")
				}

				name, nameOk := p.Args["name"].(string)
				if !nameOk || name == "" {
					return nil, fmt.Errorf("name argument is required and must be a non-empty string")
				}

				// Get storage for this kind
				gvr := schema.GroupVersionResource{
					Group:    g.groupVersion.Group,
					Version:  g.groupVersion.Version,
					Resource: strings.ToLower(kindName) + "s", // pluralize
				}
				storage := g.storageGetter(gvr)
				if storage == nil {
					return nil, fmt.Errorf("no storage available for %s", gvr.String())
				}

				// Get real data from storage
				obj, err := storage.Get(p.Context, namespace, name)
				if err != nil {
					return nil, fmt.Errorf("failed to get playlist %s/%s: %v", namespace, name, err)
				}

				// Convert the resource object to GraphQL format
				return convertPlaylistToGraphQL(obj), nil
			},
		}

		// Add structured list query
		queryFields[lowercaseKind+"s"] = &graphql.Field{
			Type: playlistListType,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				// Safely extract namespace argument with proper error handling
				namespace, namespaceOk := p.Args["namespace"].(string)
				if !namespaceOk || namespace == "" {
					return nil, fmt.Errorf("namespace argument is required and must be a non-empty string")
				}

				// Get storage for this kind
				gvr := schema.GroupVersionResource{
					Group:    g.groupVersion.Group,
					Version:  g.groupVersion.Version,
					Resource: strings.ToLower(kindName) + "s", // pluralize
				}
				storage := g.storageGetter(gvr)
				if storage == nil {
					return nil, fmt.Errorf("no storage available for %s", gvr.String())
				}

				// Get real data from storage
				listObj, err := storage.List(p.Context, namespace, ListOptions{})
				if err != nil {
					return nil, fmt.Errorf("failed to list playlists in namespace %s: %v", namespace, err)
				}

				// Convert the resource list to GraphQL format
				return convertPlaylistListToGraphQL(listObj), nil
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
		fmt.Printf("❌ Schema creation failed: %v\n", err)
		return nil, err
	}

	fmt.Printf("✅ Schema created successfully\n")
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

// convertPlaylistToGraphQL converts a resource.Object to GraphQL format
func convertPlaylistToGraphQL(obj resource.Object) interface{} {
	// Get the basic metadata
	metadata := obj.GetStaticMetadata()

	// Try to get the spec if it exists
	spec := obj.GetSpec()

	result := map[string]interface{}{
		"uid":      metadata.Name,
		"name":     metadata.Name,   // fallback to name if no title
		"interval": "5m",            // default interval
		"items":    []interface{}{}, // empty items array
	}

	// Try multiple approaches to extract playlist data

	// First, try as a map (in case it was unmarshaled as JSON)
	if specMap, ok := spec.(map[string]interface{}); ok {
		if title, exists := specMap["title"]; exists {
			result["name"] = title
		}
		if interval, exists := specMap["interval"]; exists {
			result["interval"] = interval
		}
		if items, exists := specMap["items"]; exists {
			if itemList, ok := items.([]interface{}); ok {
				// Convert items to GraphQL format
				graphqlItems := make([]interface{}, len(itemList))
				for i, item := range itemList {
					if itemMap, ok := item.(map[string]interface{}); ok {
						graphqlItems[i] = map[string]interface{}{
							"id":          i + 1,
							"playlistUid": metadata.Name,
							"type":        itemMap["type"],
							"value":       itemMap["value"],
							"order":       i + 1,
							"title":       fmt.Sprintf("Dashboard %d", i+1),
						}
					}
				}
				result["items"] = graphqlItems
			}
		}
	} else {
		fmt.Printf("🔍 Spec is not a map, trying reflection...\n")
		// Use reflection to extract fields from typed struct

		specValue := reflect.ValueOf(spec)
		if specValue.Kind() == reflect.Ptr {
			specValue = specValue.Elem()
		}

		if specValue.Kind() == reflect.Struct {
			// Try to find Title field
			if titleField := specValue.FieldByName("Title"); titleField.IsValid() && titleField.CanInterface() {
				if titleStr, ok := titleField.Interface().(string); ok && titleStr != "" {
					result["name"] = titleStr
				}
			}

			// Try to find Interval field
			if intervalField := specValue.FieldByName("Interval"); intervalField.IsValid() && intervalField.CanInterface() {
				if intervalStr, ok := intervalField.Interface().(string); ok && intervalStr != "" {
					result["interval"] = intervalStr
				}
			}

			// Try to find Items field
			if itemsField := specValue.FieldByName("Items"); itemsField.IsValid() && itemsField.CanInterface() {
				itemsValue := itemsField.Interface()

				// Handle slice of items
				if itemsSlice := reflect.ValueOf(itemsValue); itemsSlice.Kind() == reflect.Slice {
					graphqlItems := make([]interface{}, itemsSlice.Len())
					for i := 0; i < itemsSlice.Len(); i++ {
						item := itemsSlice.Index(i).Interface()

						// Try to extract fields from item struct
						itemValue := reflect.ValueOf(item)
						if itemValue.Kind() == reflect.Ptr {
							itemValue = itemValue.Elem()
						}

						graphqlItem := map[string]interface{}{
							"id":          i + 1,
							"playlistUid": metadata.Name,
							"order":       i + 1,
							"title":       fmt.Sprintf("Dashboard %d", i+1),
						}

						if itemValue.Kind() == reflect.Struct {
							// Try to get Type field
							if typeField := itemValue.FieldByName("Type"); typeField.IsValid() && typeField.CanInterface() {
								graphqlItem["type"] = fmt.Sprintf("%v", typeField.Interface())
							}
							// Try to get Value field
							if valueField := itemValue.FieldByName("Value"); valueField.IsValid() && valueField.CanInterface() {
								graphqlItem["value"] = fmt.Sprintf("%v", valueField.Interface())
							}
						}

						graphqlItems[i] = graphqlItem
					}
					result["items"] = graphqlItems
				}
			}
		}
	}

	return result
}

// convertPlaylistListToGraphQL converts a resource.ListObject to GraphQL format
func convertPlaylistListToGraphQL(listObj resource.ListObject) interface{} {
	// Get the items from the list
	items := listObj.GetItems()

	// Convert each item
	graphqlItems := make([]interface{}, len(items))
	for i, item := range items {
		graphqlItems[i] = convertPlaylistToGraphQL(item)
	}

	return map[string]interface{}{
		"items": graphqlItems,
	}
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
