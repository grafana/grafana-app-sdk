package codegen

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/resource"
)

// GraphQLGenerator generates GraphQL schemas and resolvers from CUE kinds
type GraphQLGenerator struct {
	kinds              []resource.Kind
	groupVersion       schema.GroupVersion
	storageGetter      func(gvr schema.GroupVersionResource) subgraph.Storage
	relationshipParser *RelationshipParser
	subgraphRegistry   SubgraphRegistry
}

// NewGraphQLGenerator creates a new GraphQL generator
func NewGraphQLGenerator(kinds []resource.Kind, gv schema.GroupVersion, storageGetter func(gvr schema.GroupVersionResource) subgraph.Storage) *GraphQLGenerator {
	return &GraphQLGenerator{
		kinds:         kinds,
		groupVersion:  gv,
		storageGetter: storageGetter,
	}
}

// WithRelationships adds relationship support to the generator
func (g *GraphQLGenerator) WithRelationships(parser *RelationshipParser, registry SubgraphRegistry) *GraphQLGenerator {
	g.relationshipParser = parser
	g.subgraphRegistry = registry
	return g
}

// GenerateSchema generates a complete GraphQL schema from the configured kinds
func (g *GraphQLGenerator) GenerateSchema() (*graphql.Schema, error) {
	// Create common scalar types
	jsonScalar := g.createJSONScalar()
	labelsScalar := g.createLabelsScalar()
	annotationsScalar := g.createAnnotationsScalar()

	// Create metadata type (common to all Kubernetes resources)
	metadataType := g.createMetadataType(labelsScalar, annotationsScalar)

	// Generate types for each kind
	objectTypes := make(map[string]*graphql.Object)
	inputTypes := make(map[string]*graphql.InputObject)

	for _, kind := range g.kinds {
		objectType, inputType, err := g.generateTypesForKind(kind, metadataType, jsonScalar)
		if err != nil {
			return nil, fmt.Errorf("failed to generate types for kind %s: %w", kind.Kind(), err)
		}
		objectTypes[kind.Kind()] = objectType
		inputTypes[kind.Kind()] = inputType
	}

	// Generate query fields
	queryFields := g.generateQueryFields(objectTypes)

	// Generate mutation fields
	mutationFields := g.generateMutationFields(objectTypes, inputTypes)

	// Create root query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: queryFields,
	})

	// Create root mutation type
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Mutation",
		Fields: mutationFields,
	})

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})

	return &schema, err
}

// GenerateResolvers generates resolver functions for the schema
func (g *GraphQLGenerator) GenerateResolvers() subgraph.ResolverMap {
	resolvers := make(subgraph.ResolverMap)

	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)
		pluralKind := strings.ToLower(kindName) + "s" // Simple pluralization

		// Get resolver for single resource
		resolvers[lowercaseKind] = g.createGetResolver(kind)

		// List resolver for multiple resources
		resolvers[pluralKind] = g.createListResolver(kind)

		// Create resolver
		resolvers["create"+kindName] = g.createCreateResolver(kind)

		// Update resolver
		resolvers["update"+kindName] = g.createUpdateResolver(kind)

		// Delete resolver
		resolvers["delete"+kindName] = g.createDeleteResolver(kind)
	}

	return resolvers
}

// createJSONScalar creates a JSON scalar type for arbitrary JSON data
func (g *GraphQLGenerator) createJSONScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name:        "JSON",
		Description: "Arbitrary JSON data",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) interface{} {
			// For now, return nil - this would need proper AST parsing
			return nil
		},
	})
}

// createLabelsScalar creates a scalar for labels (key-value pairs)
func (g *GraphQLGenerator) createLabelsScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name:        "Labels",
		Description: "Key-value pairs for labels",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) interface{} {
			return nil
		},
	})
}

// createAnnotationsScalar creates a scalar for annotations (key-value pairs)
func (g *GraphQLGenerator) createAnnotationsScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name:        "Annotations",
		Description: "Key-value pairs for annotations",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) interface{} {
			return nil
		},
	})
}

// createMetadataType creates the standard Kubernetes metadata type
func (g *GraphQLGenerator) createMetadataType(labelsScalar, annotationsScalar *graphql.Scalar) *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "ObjectMeta",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type: graphql.String,
			},
			"namespace": &graphql.Field{
				Type: graphql.String,
			},
			"uid": &graphql.Field{
				Type: graphql.String,
			},
			"resourceVersion": &graphql.Field{
				Type: graphql.String,
			},
			"generation": &graphql.Field{
				Type: graphql.Int,
			},
			"creationTimestamp": &graphql.Field{
				Type: graphql.DateTime,
			},
			"labels": &graphql.Field{
				Type: labelsScalar,
			},
			"annotations": &graphql.Field{
				Type: annotationsScalar,
			},
		},
	})
}

// generateTypesForKind generates GraphQL object and input types for a single kind
func (g *GraphQLGenerator) generateTypesForKind(kind resource.Kind, metadataType *graphql.Object, jsonScalar *graphql.Scalar) (*graphql.Object, *graphql.InputObject, error) {
	kindName := kind.Kind()

	// Start with base fields
	fields := graphql.Fields{
		"apiVersion": &graphql.Field{
			Type: graphql.String,
		},
		"kind": &graphql.Field{
			Type: graphql.String,
		},
		"metadata": &graphql.Field{
			Type: metadataType,
		},
		"spec": &graphql.Field{
			Type:        jsonScalar,
			Description: fmt.Sprintf("Specification for %s", kindName),
		},
		"status": &graphql.Field{
			Type:        jsonScalar,
			Description: fmt.Sprintf("Status of %s", kindName),
		},
	}

	// Add relationship fields if relationship support is enabled
	if g.relationshipParser != nil && g.subgraphRegistry != nil {
		relationshipFields, err := g.generateRelationshipFields(kind)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate relationship fields for %s: %w", kindName, err)
		}

		// Add relationship fields to the base fields
		for fieldName, field := range relationshipFields {
			fields[fieldName] = field
		}
	}

	// Create the main object type
	objectType := graphql.NewObject(graphql.ObjectConfig{
		Name:   kindName,
		Fields: fields,
	})

	// Create input type for mutations
	inputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: kindName + "Input",
		Fields: graphql.InputObjectConfigFieldMap{
			"metadata": &graphql.InputObjectFieldConfig{
				Type: g.createMetadataInputType(),
			},
			"spec": &graphql.InputObjectFieldConfig{
				Type: jsonScalar,
			},
		},
	})

	return objectType, inputType, nil
}

// generateRelationshipFields generates GraphQL fields for relationships
func (g *GraphQLGenerator) generateRelationshipFields(kind resource.Kind) (graphql.Fields, error) {
	fields := make(graphql.Fields)

	// Parse relationships for this kind
	relationships, err := g.relationshipParser.ParseRelationships(kind)
	if err != nil {
		return nil, fmt.Errorf("failed to parse relationships: %w", err)
	}

	// Generate GraphQL field for each relationship
	for fieldName, config := range relationships {
		resolver := NewRelationshipResolver(config, g.subgraphRegistry)
		field := resolver.CreateGraphQLField()
		fields[fieldName] = field
	}

	return fields, nil
}

// createMetadataInputType creates an input type for metadata
func (g *GraphQLGenerator) createMetadataInputType() *graphql.InputObject {
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ObjectMetaInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name": &graphql.InputObjectFieldConfig{
				Type: graphql.String,
			},
			"namespace": &graphql.InputObjectFieldConfig{
				Type: graphql.String,
			},
			"labels": &graphql.InputObjectFieldConfig{
				Type: g.createJSONScalar(),
			},
			"annotations": &graphql.InputObjectFieldConfig{
				Type: g.createJSONScalar(),
			},
		},
	})
}

// generateQueryFields generates query fields for all kinds
func (g *GraphQLGenerator) generateQueryFields(objectTypes map[string]*graphql.Object) graphql.Fields {
	fields := make(graphql.Fields)

	for _, kind := range g.kinds {
		kindName := kind.Kind()
		lowercaseKind := strings.ToLower(kindName)
		pluralKind := strings.ToLower(kindName) + "s"

		objectType := objectTypes[kindName]

		// Single resource query
		fields[lowercaseKind] = &graphql.Field{
			Type: objectType,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"name": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
			},
			Resolve: g.createGetResolver(kind),
		}

		// List resources query
		fields[pluralKind] = &graphql.Field{
			Type: graphql.NewList(objectType),
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"labelSelector": &graphql.ArgumentConfig{
					Type: graphql.String,
				},
				"fieldSelector": &graphql.ArgumentConfig{
					Type: graphql.String,
				},
				"limit": &graphql.ArgumentConfig{
					Type: graphql.Int,
				},
			},
			Resolve: g.createListResolver(kind),
		}
	}

	return fields
}

// generateMutationFields generates mutation fields for all kinds
func (g *GraphQLGenerator) generateMutationFields(objectTypes map[string]*graphql.Object, inputTypes map[string]*graphql.InputObject) graphql.Fields {
	fields := make(graphql.Fields)

	for _, kind := range g.kinds {
		kindName := kind.Kind()
		objectType := objectTypes[kindName]
		inputType := inputTypes[kindName]

		// Create mutation
		fields["create"+kindName] = &graphql.Field{
			Type: objectType,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"input": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(inputType),
				},
			},
			Resolve: g.createCreateResolver(kind),
		}

		// Update mutation
		fields["update"+kindName] = &graphql.Field{
			Type: objectType,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"name": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"input": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(inputType),
				},
			},
			Resolve: g.createUpdateResolver(kind),
		}

		// Delete mutation
		fields["delete"+kindName] = &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"namespace": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"name": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
			},
			Resolve: g.createDeleteResolver(kind),
		}
	}

	return fields
}

// createGetResolver creates a resolver for getting a single resource
func (g *GraphQLGenerator) createGetResolver(kind resource.Kind) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		namespace := p.Args["namespace"].(string)
		name := p.Args["name"].(string)

		gvr := schema.GroupVersionResource{
			Group:    g.groupVersion.Group,
			Version:  g.groupVersion.Version,
			Resource: strings.ToLower(kind.Kind()) + "s", // Simple pluralization
		}

		storage := g.storageGetter(gvr)
		if storage == nil {
			return nil, fmt.Errorf("no storage found for %s", gvr)
		}

		return storage.Get(p.Context, namespace, name)
	}
}

// createListResolver creates a resolver for listing resources
func (g *GraphQLGenerator) createListResolver(kind resource.Kind) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		namespace := p.Args["namespace"].(string)

		options := subgraph.ListOptions{}
		if labelSelector, ok := p.Args["labelSelector"].(string); ok {
			options.LabelSelector = labelSelector
		}
		if fieldSelector, ok := p.Args["fieldSelector"].(string); ok {
			options.FieldSelector = fieldSelector
		}
		if limit, ok := p.Args["limit"].(int); ok {
			options.Limit = int64(limit)
		}

		gvr := schema.GroupVersionResource{
			Group:    g.groupVersion.Group,
			Version:  g.groupVersion.Version,
			Resource: strings.ToLower(kind.Kind()) + "s", // Simple pluralization
		}

		storage := g.storageGetter(gvr)
		if storage == nil {
			return nil, fmt.Errorf("no storage found for %s", gvr)
		}

		listResult, err := storage.List(p.Context, namespace, options)
		if err != nil {
			return nil, err
		}

		// Return the items from the list
		return listResult.GetItems(), nil
	}
}

// createCreateResolver creates a resolver for creating resources
func (g *GraphQLGenerator) createCreateResolver(kind resource.Kind) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// namespace := p.Args["namespace"].(string)
		// input := p.Args["input"] // TODO: Convert GraphQL input to resource.Object

		gvr := schema.GroupVersionResource{
			Group:    g.groupVersion.Group,
			Version:  g.groupVersion.Version,
			Resource: strings.ToLower(kind.Kind()) + "s",
		}

		storage := g.storageGetter(gvr)
		if storage == nil {
			return nil, fmt.Errorf("no storage found for %s", gvr)
		}

		// TODO: Convert GraphQL input to resource.Object and implement create
		// For now, return error indicating not implemented
		return nil, fmt.Errorf("create resolver not fully implemented yet")
	}
}

// createUpdateResolver creates a resolver for updating resources
func (g *GraphQLGenerator) createUpdateResolver(kind resource.Kind) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		namespace := p.Args["namespace"].(string)
		name := p.Args["name"].(string)

		gvr := schema.GroupVersionResource{
			Group:    g.groupVersion.Group,
			Version:  g.groupVersion.Version,
			Resource: strings.ToLower(kind.Kind()) + "s",
		}

		storage := g.storageGetter(gvr)
		if storage == nil {
			return nil, fmt.Errorf("no storage found for %s", gvr)
		}

		// TODO: Convert GraphQL input to resource.Object and implement update
		return nil, fmt.Errorf("update resolver not fully implemented yet for %s/%s", namespace, name)
	}
}

// createDeleteResolver creates a resolver for deleting resources
func (g *GraphQLGenerator) createDeleteResolver(kind resource.Kind) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		namespace := p.Args["namespace"].(string)
		name := p.Args["name"].(string)

		gvr := schema.GroupVersionResource{
			Group:    g.groupVersion.Group,
			Version:  g.groupVersion.Version,
			Resource: strings.ToLower(kind.Kind()) + "s",
		}

		storage := g.storageGetter(gvr)
		if storage == nil {
			return nil, fmt.Errorf("no storage found for %s", gvr)
		}

		err := storage.Delete(p.Context, namespace, name)
		if err != nil {
			return false, err
		}

		return true, nil
	}
}
