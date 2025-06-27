package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/resource"
)

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   interface{}            `json:"data"`
	Errors []interface{}          `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// GraphQLRouter is a router that provides GraphQL endpoints for resource collections
type GraphQLRouter struct {
	*JSONRouter
	schema            graphql.Schema
	resourceCollections map[string]resource.KindCollection
	stores            map[string]Store
	dataloaders       map[string]*DataLoader
}

// DataLoader helps batch and cache resource loading to avoid N+1 problems
type DataLoader struct {
	BatchFunc func(ctx context.Context, keys []string) ([]resource.Object, error)
	cache     map[string]resource.Object
}

// NewGraphQLRouter creates a new GraphQL router that aggregates multiple resource groups
func NewGraphQLRouter() (*GraphQLRouter, error) {
	router := &GraphQLRouter{
		JSONRouter:          NewJSONRouter(),
		resourceCollections: make(map[string]resource.KindCollection),
		stores:              make(map[string]Store),
		dataloaders:         make(map[string]*DataLoader),
	}

	// Initially create an empty schema - it will be rebuilt when resource groups are added
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: graphql.Fields{
				"ping": &graphql.Field{
					Type: graphql.String,
					Resolve: func(p graphql.ResolveParams) (interface{}, error) {
						return "pong", nil
					},
				},
			},
		}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: graphql.Fields{},
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create initial GraphQL schema: %w", err)
	}

	router.schema = schema
	router.setupRoutes()
	return router, nil
}

// AddResourceGroup adds a resource group to the GraphQL schema
func (g *GraphQLRouter) AddResourceGroup(groupName string, resourceGroup resource.KindCollection, store Store) error {
	g.resourceCollections[groupName] = resourceGroup
	g.stores[groupName] = store

	// Create dataloader for this resource group
	g.dataloaders[groupName] = &DataLoader{
		BatchFunc: func(ctx context.Context, keys []string) ([]resource.Object, error) {
			return g.batchLoadResources(ctx, groupName, keys)
		},
		cache: make(map[string]resource.Object),
	}

	// Rebuild the schema with all registered resource groups
	return g.rebuildSchema()
}

// setupRoutes sets up the GraphQL endpoint
func (g *GraphQLRouter) setupRoutes() {
	g.Handle("/graphql", g.handleGraphQL, http.MethodPost, http.MethodGet)
}

// handleGraphQL handles GraphQL requests
func (g *GraphQLRouter) handleGraphQL(ctx context.Context, request JSONRequest) (JSONResponse, error) {
	var gqlRequest GraphQLRequest

	if request.Method == http.MethodGet {
		// Handle GET requests with query parameters
		query := request.URL.Query().Get("query")
		variables := request.URL.Query().Get("variables")
		operationName := request.URL.Query().Get("operationName")

		gqlRequest.Query = query
		gqlRequest.OperationName = operationName

		if variables != "" {
			if err := json.Unmarshal([]byte(variables), &gqlRequest.Variables); err != nil {
				return nil, plugin.NewError(http.StatusBadRequest, fmt.Sprintf("invalid variables: %v", err))
			}
		}
	} else {
		// Handle POST requests with JSON body
		if err := json.NewDecoder(request.Body).Decode(&gqlRequest); err != nil {
			return nil, plugin.NewError(http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		}
	}

	// Execute GraphQL query
	result := graphql.Do(graphql.Params{
		Schema:         g.schema,
		RequestString:  gqlRequest.Query,
		VariableValues: gqlRequest.Variables,
		OperationName:  gqlRequest.OperationName,
		Context:        ctx,
	})

	response := GraphQLResponse{
		Data: result.Data,
	}

	if len(result.Errors) > 0 {
		errors := make([]interface{}, len(result.Errors))
		for i, err := range result.Errors {
			errors[i] = map[string]interface{}{
				"message": err.Error(),
			}
		}
		response.Errors = errors
	}

	if len(result.Extensions) > 0 {
		response.Extensions = result.Extensions
	}

	return response, nil
}

// rebuildSchema rebuilds the GraphQL schema with all registered resource groups
func (g *GraphQLRouter) rebuildSchema() error {
	// Use the schema builder for better type generation
	schemaBuilder := NewSchemaBuilder()
	
	// Build schema from all resource collections
	schema, err := schemaBuilder.BuildSchemaFromKinds(g.resourceCollections)
	if err != nil {
		return fmt.Errorf("failed to build GraphQL schema: %w", err)
	}

	// Override resolvers with our implementations
	g.setResolvers(schema, schemaBuilder)

	g.schema = schema
	return nil
}

// setResolvers sets the actual resolvers for the GraphQL fields
func (g *GraphQLRouter) setResolvers(schema graphql.Schema, builder *SchemaBuilder) {
	queryType := schema.QueryType()
	mutationType := schema.MutationType()

	// Set resolvers for query fields
	for groupName, collection := range g.resourceCollections {
		store := g.stores[groupName]

		for _, kind := range collection.Kinds() {
			kindName := kind.Kind()
			
			// Get field resolver
			if getField := queryType.Fields()[builder.camelCase(kindName)]; getField != nil {
				getField.Resolve = g.createGetResolver(kind, store)
			}

			// List field resolver
			if listField := queryType.Fields()[builder.camelCase(kind.Plural())]; listField != nil {
				listField.Resolve = g.createListResolver(kind, store)
			}

			// Relationship field resolvers
			if kind.Kind() == "Investigation" {
				if investigationsByUserField := queryType.Fields()["investigationsByUser"]; investigationsByUserField != nil {
					investigationsByUserField.Resolve = g.createInvestigationsByUserResolver(store)
				}
			}

			// Mutation field resolvers
			if createField := mutationType.Fields()["create"+kindName]; createField != nil {
				createField.Resolve = g.createCreateResolver(kind, store)
			}

			if updateField := mutationType.Fields()["update"+kindName]; updateField != nil {
				updateField.Resolve = g.createUpdateResolver(kind, store)
			}

			if deleteField := mutationType.Fields()["delete"+kindName]; deleteField != nil {
				deleteField.Resolve = g.createDeleteResolver(kind, store)
			}
		}
	}
}

// createGetResolver creates a resolver function for getting a single resource
func (g *GraphQLRouter) createGetResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		name := p.Args["name"].(string)
		namespace := ""
		if ns, ok := p.Args["namespace"]; ok && ns != nil {
			namespace = ns.(string)
		}

		identifier := resource.Identifier{
			Name:      name,
			Namespace: namespace,
		}

		return store.Get(p.Context, kind.Kind(), identifier)
	}
}

// createListResolver creates a resolver function for listing resources
func (g *GraphQLRouter) createListResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		options := resource.StoreListOptions{}

		if namespace, ok := p.Args["namespace"]; ok && namespace != nil {
			options.Namespace = namespace.(string)
		}

		if labelSelector, ok := p.Args["labelSelector"]; ok && labelSelector != nil {
			options.Filters = append(options.Filters, labelSelector.(string))
		}

		if limit, ok := p.Args["limit"]; ok && limit != nil {
			options.PerPage = limit.(int)
		}

		listResult, err := store.List(p.Context, kind.Kind(), options)
		if err != nil {
			return nil, err
		}

		return listResult.GetItems(), nil
	}
}

// createCreateResolver creates a resolver function for creating resources
func (g *GraphQLRouter) createCreateResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		input := p.Args["input"].(map[string]interface{})

		// Create a new object from the kind
		obj := kind.ZeroValue()

		// Set basic metadata
		if metadata, ok := input["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				obj.SetName(name)
			}
			if namespace, ok := metadata["namespace"].(string); ok {
				obj.SetNamespace(namespace)
			}
		}

		// Set spec
		if spec, ok := input["spec"]; ok {
			if err := obj.SetSpec(spec); err != nil {
				return nil, fmt.Errorf("failed to set spec: %w", err)
			}
		}

		// Set static metadata
		obj.SetStaticMetadata(resource.StaticMetadata{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Group:     kind.Group(),
			Version:   kind.Version(),
			Kind:      kind.Kind(),
		})

		return store.Add(p.Context, obj)
	}
}

// createUpdateResolver creates a resolver function for updating resources
func (g *GraphQLRouter) createUpdateResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		input := p.Args["input"].(map[string]interface{})

		// Create object from input
		obj := kind.ZeroValue()

		// Set metadata
		if metadata, ok := input["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				obj.SetName(name)
			}
			if namespace, ok := metadata["namespace"].(string); ok {
				obj.SetNamespace(namespace)
			}
		}

		// Set spec
		if spec, ok := input["spec"]; ok {
			if err := obj.SetSpec(spec); err != nil {
				return nil, fmt.Errorf("failed to set spec: %w", err)
			}
		}

		// Set static metadata
		obj.SetStaticMetadata(resource.StaticMetadata{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Group:     kind.Group(),
			Version:   kind.Version(),
			Kind:      kind.Kind(),
		})

		return store.Update(p.Context, obj)
	}
}

// createDeleteResolver creates a resolver function for deleting resources
func (g *GraphQLRouter) createDeleteResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		name := p.Args["name"].(string)
		namespace := ""
		if ns, ok := p.Args["namespace"]; ok && ns != nil {
			namespace = ns.(string)
		}

		identifier := resource.Identifier{
			Name:      name,
			Namespace: namespace,
		}

		err := store.Delete(p.Context, kind.Kind(), identifier)
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

// createInvestigationsByUserResolver creates a resolver for getting investigations by user
func (g *GraphQLRouter) createInvestigationsByUserResolver(store Store) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		userId := p.Args["userId"].(string)
		limit := 10 // default limit
		if l, ok := p.Args["limit"]; ok && l != nil {
			limit = l.(int)
		}

		// Use Filters and PerPage instead of LabelSelector and Limit
		options := resource.StoreListOptions{
			Filters: []string{fmt.Sprintf("createdBy=%s", userId)},
			PerPage: limit,
		}

		listResult, err := store.List(p.Context, "Investigation", options)
		if err != nil {
			return nil, err
		}

		return listResult.GetItems(), nil
	}
}

// batchLoadResources implements batch loading for the dataloader
func (g *GraphQLRouter) batchLoadResources(ctx context.Context, groupName string, keys []string) ([]resource.Object, error) {
	store := g.stores[groupName]
	results := make([]resource.Object, len(keys))

	// This is a simplified implementation - in practice, you'd want to batch these calls
	for i, key := range keys {
		// Parse the key (could be "kind:namespace:name" format)
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}

		kindName := parts[0]
		identifier := resource.Identifier{Name: parts[len(parts)-1]}
		if len(parts) > 2 {
			identifier.Namespace = parts[1]
		}

		obj, err := store.Get(ctx, kindName, identifier)
		if err != nil {
			logging.FromContext(ctx).Error("failed to load resource", "key", key, "error", err)
			continue
		}
		results[i] = obj
	}

	return results, nil
} 