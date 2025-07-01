package router

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/grafana/grafana-app-sdk/resource"
)

// AppGraphQLProvider represents a GraphQL provider for an individual app
// This replaces the previous GraphQLRegistry which was moved to the main Grafana server
type AppGraphQLProvider struct {
	appName             string
	resourceCollections map[string]resource.KindCollection
	stores              map[string]Store
	schema              graphql.Schema
	schemaBuilder       *SchemaBuilder
}

// NewAppGraphQLProvider creates a new GraphQL provider for a single app
func NewAppGraphQLProvider(appName string, resourceCollection resource.KindCollection, store Store) (*AppGraphQLProvider, error) {
	provider := &AppGraphQLProvider{
		appName:             appName,
		resourceCollections: make(map[string]resource.KindCollection),
		stores:              make(map[string]Store),
		schemaBuilder:       NewSchemaBuilder(),
	}
	
	// Add the provided resource collection
	err := provider.AddResourceCollection(appName, resourceCollection, store)
	if err != nil {
		return nil, err
	}
	
	return provider, nil
}

// NewAppGraphQLProviderFromApp creates a GraphQL provider from an existing app instance
// This ensures the GraphQL provider uses the same storage backend as the REST API
func NewAppGraphQLProviderFromApp(appName string, app interface{}) (*AppGraphQLProvider, error) {
	// Try to extract client generator and kind collection from the app
	var clientGenerator resource.ClientGenerator
	var kindCollection resource.KindCollection

	// Check if the app has the methods we need (duck typing)
	if appWithClientGen, ok := app.(interface{ GetClientGenerator() resource.ClientGenerator }); ok {
		clientGenerator = appWithClientGen.GetClientGenerator()
	} else {
		return nil, fmt.Errorf("app does not provide GetClientGenerator() method")
	}

	if appWithKindCollection, ok := app.(interface{ GetKindCollection() resource.KindCollection }); ok {
		kindCollection = appWithKindCollection.GetKindCollection()
	} else {
		return nil, fmt.Errorf("app does not provide GetKindCollection() method")
	}

	// Create a store using the same client generator as the app
	store := resource.NewStore(clientGenerator, kindCollection)

	return NewAppGraphQLProvider(appName, kindCollection, store)
}

// AddResourceCollection adds a resource collection to this app's GraphQL schema
func (p *AppGraphQLProvider) AddResourceCollection(groupName string, collection resource.KindCollection, store Store) error {
	p.resourceCollections[groupName] = collection
	p.stores[groupName] = store
	
	// Rebuild schema when collections are added
	return p.buildSchema()
}

// GetGraphQLSchema returns the GraphQL schema for this app
func (p *AppGraphQLProvider) GetGraphQLSchema() (graphql.Schema, error) {
	if p.schema.QueryType() == nil {
		if err := p.buildSchema(); err != nil {
			return graphql.Schema{}, err
		}
	}
	return p.schema, nil
}

// GetAppName returns the name of this app
func (p *AppGraphQLProvider) GetAppName() string {
	return p.appName
}

// GetResourceCollections returns the resource collections for this app
func (p *AppGraphQLProvider) GetResourceCollections() map[string]interface{} {
	result := make(map[string]interface{})
	for name, collection := range p.resourceCollections {
		result[name] = collection
	}
	return result
}

// buildSchema builds the GraphQL schema from the app's resource collections
func (p *AppGraphQLProvider) buildSchema() error {
	schema, err := p.schemaBuilder.BuildSchemaFromKinds(p.resourceCollections)
	if err != nil {
		return err
	}

	// Set resolvers for the schema
	p.setResolvers(schema)
	
	p.schema = schema
	return nil
}

// setResolvers sets up resolvers for the GraphQL schema
func (p *AppGraphQLProvider) setResolvers(schema graphql.Schema) {
	queryType := schema.QueryType()
	mutationType := schema.MutationType()

	for groupName, collection := range p.resourceCollections {
		store := p.stores[groupName]
		
		for _, kind := range collection.Kinds() {
			kindName := kind.Kind()
			
			// Get field resolver
			if getField := queryType.Fields()[p.schemaBuilder.camelCase(kindName)]; getField != nil {
				getField.Resolve = p.createGetResolver(kind, store)
			}

			// List field resolver
			if listField := queryType.Fields()[p.schemaBuilder.camelCase(kind.Plural())]; listField != nil {
				listField.Resolve = p.createListResolver(kind, store)
			}

			// Mutation field resolvers
			if mutationType != nil {
				if createField := mutationType.Fields()["create"+kindName]; createField != nil {
					createField.Resolve = p.createCreateResolver(kind, store)
				}

				if updateField := mutationType.Fields()["update"+kindName]; updateField != nil {
					updateField.Resolve = p.createUpdateResolver(kind, store)
				}

				if deleteField := mutationType.Fields()["delete"+kindName]; deleteField != nil {
					deleteField.Resolve = p.createDeleteResolver(kind, store)
				}
			}
		}
	}
}

// Resolver creators (simplified versions from the original registry)

func (p *AppGraphQLProvider) createGetResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		name := params.Args["name"].(string)
		namespace := ""
		if ns, ok := params.Args["namespace"]; ok && ns != nil {
			namespace = ns.(string)
		}

		identifier := resource.Identifier{
			Name:      name,
			Namespace: namespace,
		}

		return store.Get(params.Context, kind.Kind(), identifier)
	}
}

func (p *AppGraphQLProvider) createListResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		options := resource.StoreListOptions{}

		if namespace, ok := params.Args["namespace"]; ok && namespace != nil {
			options.Namespace = namespace.(string)
		}

		listResult, err := store.List(params.Context, kind.Kind(), options)
		if err != nil {
			return nil, err
		}

		return listResult.GetItems(), nil
	}
}

func (p *AppGraphQLProvider) createCreateResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		input := params.Args["input"].(map[string]interface{})

		obj := kind.ZeroValue()

		if metadata, ok := input["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				obj.SetName(name)
			}
			if namespace, ok := metadata["namespace"].(string); ok {
				obj.SetNamespace(namespace)
			}
		}

		if spec, ok := input["spec"]; ok {
			if err := obj.SetSpec(spec); err != nil {
				return nil, err
			}
		}

		obj.SetStaticMetadata(resource.StaticMetadata{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Group:     kind.Group(),
			Version:   kind.Version(),
			Kind:      kind.Kind(),
		})

		return store.Add(params.Context, obj)
	}
}

func (p *AppGraphQLProvider) createUpdateResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		input := params.Args["input"].(map[string]interface{})

		obj := kind.ZeroValue()

		if metadata, ok := input["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				obj.SetName(name)
			}
			if namespace, ok := metadata["namespace"].(string); ok {
				obj.SetNamespace(namespace)
			}
		}

		if spec, ok := input["spec"]; ok {
			if err := obj.SetSpec(spec); err != nil {
				return nil, err
			}
		}

		obj.SetStaticMetadata(resource.StaticMetadata{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Group:     kind.Group(),
			Version:   kind.Version(),
			Kind:      kind.Kind(),
		})

		return store.Update(params.Context, obj)
	}
}

func (p *AppGraphQLProvider) createDeleteResolver(kind resource.Schema, store Store) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (interface{}, error) {
		name := params.Args["name"].(string)
		namespace := ""
		if ns, ok := params.Args["namespace"]; ok && ns != nil {
			namespace = ns.(string)
		}

		identifier := resource.Identifier{
			Name:      name,
			Namespace: namespace,
		}

		err := store.Delete(params.Context, kind.Kind(), identifier)
		if err != nil {
			return false, err
		}
		return true, nil
	}
} 