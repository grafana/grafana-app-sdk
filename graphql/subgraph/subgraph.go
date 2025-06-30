package subgraph

import (
	"context"
	"encoding/json"
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
	queryFields := make(graphql.Fields)

	// Add a hello field to verify the schema works
	queryFields["hello"] = &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "Hello from " + g.groupVersion.String(), nil
		},
	}

	// Generate types and fields dynamically for each kind
	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)

		// Create generic resource types
		resourceType := g.createResourceType(kind)
		resourceListType := g.createResourceListType(kind, resourceType)

		// Add get query for single resource
		queryFields[lowercaseKind] = g.createGetField(kind, resourceType)

		// Add list query for multiple resources
		queryFields[lowercaseKind+"s"] = g.createListField(kind, resourceListType)
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

// createResourceType creates a generic GraphQL type for any resource kind
func (g *simpleGenerator) createResourceType(kind resource.Kind) *graphql.Object {
	kindName := kind.Kind()

	// Create standard Kubernetes metadata type
	metadataType := g.getOrCreateMetadataType()

	// Base fields that all resources have
	baseFields := graphql.Fields{
		"metadata": &graphql.Field{Type: metadataType},
		"spec":     &graphql.Field{Type: graphql.String}, // JSON scalar fallback
	}

	// Add resource-specific fields based on kind
	resourceFields := g.getResourceSpecificFields(kind)
	for fieldName, field := range resourceFields {
		baseFields[fieldName] = field
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   kindName,
		Fields: baseFields,
	})
}

// getResourceSpecificFields returns additional fields for specific resource types
func (g *simpleGenerator) getResourceSpecificFields(kind resource.Kind) graphql.Fields {
	kindName := strings.ToLower(kind.Kind())

	switch kindName {
	case "playlist":
		return g.getPlaylistFields()
	default:
		// For other resource types, return empty - they'll use the generic metadata + spec approach
		return graphql.Fields{}
	}
}

// getPlaylistFields returns playlist-specific GraphQL fields
func (g *simpleGenerator) getPlaylistFields() graphql.Fields {
	// Create playlist item type
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

	return graphql.Fields{
		"uid":      &graphql.Field{Type: graphql.String},
		"name":     &graphql.Field{Type: graphql.String},
		"interval": &graphql.Field{Type: graphql.String},
		"items":    &graphql.Field{Type: graphql.NewList(playlistItemType)},
	}
}

// createResourceListType creates a generic list type for any resource kind
func (g *simpleGenerator) createResourceListType(kind resource.Kind, itemType *graphql.Object) *graphql.Object {
	kindName := kind.Kind()

	return graphql.NewObject(graphql.ObjectConfig{
		Name: kindName + "List",
		Fields: graphql.Fields{
			"items": &graphql.Field{Type: graphql.NewList(itemType)},
		},
	})
}

// getOrCreateMetadataType creates the standard Kubernetes ObjectMeta type
func (g *simpleGenerator) getOrCreateMetadataType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "ObjectMeta",
		Fields: graphql.Fields{
			"name":              &graphql.Field{Type: graphql.String},
			"namespace":         &graphql.Field{Type: graphql.String},
			"uid":               &graphql.Field{Type: graphql.String},
			"resourceVersion":   &graphql.Field{Type: graphql.String},
			"generation":        &graphql.Field{Type: graphql.Int},
			"creationTimestamp": &graphql.Field{Type: graphql.String},
			"labels":            &graphql.Field{Type: graphql.String}, // JSON scalar for now
			"annotations":       &graphql.Field{Type: graphql.String}, // JSON scalar for now
		},
	})
}

// createGetField creates a get field for retrieving a single resource
func (g *simpleGenerator) createGetField(kind resource.Kind, resourceType *graphql.Object) *graphql.Field {
	kindName := kind.Kind()

	return &graphql.Field{
		Type: resourceType,
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
				return nil, fmt.Errorf("failed to get %s %s/%s: %v", kindName, namespace, name, err)
			}

			// Convert the resource object to GraphQL format
			return g.convertResourceToGraphQL(obj), nil
		},
	}
}

// createListField creates a list field for retrieving multiple resources
func (g *simpleGenerator) createListField(kind resource.Kind, listType *graphql.Object) *graphql.Field {
	kindName := kind.Kind()

	return &graphql.Field{
		Type: listType,
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
				return nil, fmt.Errorf("failed to list %s in namespace %s: %v", kindName, namespace, err)
			}

			// Convert the resource list to GraphQL format
			return g.convertResourceListToGraphQL(listObj), nil
		},
	}
}

// createDemoData creates demo data for any resource kind
func (g *simpleGenerator) createDemoData(kind resource.Kind) interface{} {
	kindName := kind.Kind()
	lowercaseKind := strings.ToLower(kindName)

	// Base data structure
	baseData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              fmt.Sprintf("demo-%s", lowercaseKind),
			"namespace":         "default",
			"uid":               fmt.Sprintf("demo-%s-uid", lowercaseKind),
			"resourceVersion":   "1",
			"generation":        1,
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"labels":            "{}",
			"annotations":       "{}",
		},
		"spec": fmt.Sprintf(`{"title": "Demo %s", "description": "This is a demo %s for testing"}`, kindName, kindName),
	}

	// Add resource-specific demo fields
	switch lowercaseKind {
	case "playlist":
		baseData["uid"] = "demo-playlist-uid"
		baseData["name"] = "Demo Playlist (no args required)"
		baseData["interval"] = "30s"
		baseData["items"] = []map[string]interface{}{
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
		}
	}

	return baseData
}

// convertResourceToGraphQL converts any resource.Object to GraphQL format
func (g *simpleGenerator) convertResourceToGraphQL(obj resource.Object) interface{} {
	// Get the basic metadata
	staticMetadata := obj.GetStaticMetadata()
	commonMetadata := obj.GetCommonMetadata()

	// Get the spec as JSON string for generic fallback
	var specJSON string
	if spec := obj.GetSpec(); spec != nil {
		if specBytes, err := json.Marshal(spec); err == nil {
			specJSON = string(specBytes)
		} else {
			specJSON = fmt.Sprintf(`{"error": "failed to serialize spec: %v"}`, err)
		}
	} else {
		specJSON = "{}"
	}

	// Base result structure
	result := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              staticMetadata.Name,
			"namespace":         staticMetadata.Namespace,
			"uid":               commonMetadata.UID,
			"resourceVersion":   commonMetadata.ResourceVersion,
			"generation":        commonMetadata.Generation,
			"creationTimestamp": commonMetadata.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
			"labels":            g.serializeMapToJSON(commonMetadata.Labels),
			"annotations":       g.serializeAnyMapToJSON(commonMetadata.ExtraFields),
		},
		"spec": specJSON,
	}

	// Add resource-specific fields
	resourceSpecificFields := g.convertResourceSpecificFields(obj)
	for fieldName, fieldValue := range resourceSpecificFields {
		result[fieldName] = fieldValue
	}

	return result
}

// convertResourceSpecificFields extracts resource-specific fields from the spec
func (g *simpleGenerator) convertResourceSpecificFields(obj resource.Object) map[string]interface{} {
	staticMetadata := obj.GetStaticMetadata()
	kindName := strings.ToLower(staticMetadata.Kind)

	// Try multiple variations of playlist kind detection
	if kindName == "playlist" || strings.Contains(kindName, "playlist") || staticMetadata.Kind == "Playlist" {
		return g.convertPlaylistFields(obj)
	}

	// FALLBACK: Check if this looks like a playlist based on the spec structure
	if spec := obj.GetSpec(); spec != nil {
		specValue := reflect.ValueOf(spec)
		if specValue.Kind() == reflect.Ptr {
			specValue = specValue.Elem()
		}
		if specValue.Kind() == reflect.Struct {
			// Check for playlist-specific fields
			if specValue.FieldByName("Title").IsValid() && specValue.FieldByName("Interval").IsValid() && specValue.FieldByName("Items").IsValid() {
				return g.convertPlaylistFields(obj)
			}
		}
	}
	return map[string]interface{}{}
}

// convertPlaylistFields extracts playlist-specific fields from the resource
func (g *simpleGenerator) convertPlaylistFields(obj resource.Object) map[string]interface{} {
	metadata := obj.GetStaticMetadata()
	spec := obj.GetSpec()

	result := map[string]interface{}{
		"uid":      metadata.Name,   // fallback to name
		"name":     metadata.Name,   // fallback to name
		"interval": "5m",            // default interval
		"items":    []interface{}{}, // empty items array
	}

	// Try to extract playlist-specific data from spec
	if spec != nil {
		// Use reflection to extract fields from typed struct (this is the primary path)
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
		} else {
			// Fallback: try as a map (in case it was unmarshaled as JSON)
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
			}
		}
	}

	return result
}

// convertResourceListToGraphQL converts any resource.ListObject to GraphQL format
func (g *simpleGenerator) convertResourceListToGraphQL(listObj resource.ListObject) interface{} {
	// Get the items from the list
	items := listObj.GetItems()

	// Convert each item
	graphqlItems := make([]interface{}, len(items))
	for i, item := range items {
		graphqlItems[i] = g.convertResourceToGraphQL(item)
	}

	return map[string]interface{}{
		"items": graphqlItems,
	}
}

// serializeMapToJSON converts a map to JSON string, handling nil maps
func (g *simpleGenerator) serializeMapToJSON(m map[string]string) string {
	if m == nil || len(m) == 0 {
		return "{}"
	}

	if jsonBytes, err := json.Marshal(m); err == nil {
		return string(jsonBytes)
	}

	return "{}"
}

// serializeAnyMapToJSON converts a map[string]any to JSON string, handling nil maps
func (g *simpleGenerator) serializeAnyMapToJSON(m map[string]any) string {
	if m == nil || len(m) == 0 {
		return "{}"
	}

	if jsonBytes, err := json.Marshal(m); err == nil {
		return string(jsonBytes)
	}

	return "{}"
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

func (g *simpleGenerator) GenerateResolvers() ResolverMap {
	// Resolvers are now embedded in the schema fields
	// This method is kept for interface compatibility but is no longer used
	return make(ResolverMap)
}
