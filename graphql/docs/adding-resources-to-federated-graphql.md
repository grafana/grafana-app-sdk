# Adding Resources to GraphQL Federation

This guide provides a step-by-step recipe for extending Grafana's GraphQL federation system to include new App SDK-backed resources that are registered in the Grafana apps registry.

## Overview

The GraphQL federation system automatically discovers and integrates App SDK-backed resources by:

1. **Auto-discovery**: Scanning app providers that implement `GraphQLSubgraphProvider`
2. **Dynamic schema generation**: Converting any CUE kinds to GraphQL types and resolvers automatically
3. **Generic storage bridging**: Reusing existing REST storage without data migration
4. **Field prefixing**: Namespacing fields to avoid conflicts (e.g., `playlist_playlist`)

## Key Benefits of the Modular Design

✅ **Zero Resource-Specific Code**: No need to write resource-specific GraphQL types or resolvers
✅ **Automatic Type Generation**: GraphQL types are dynamically created from your CUE kinds
✅ **Generic Conversion**: All resource.Object types are automatically converted to GraphQL format
✅ **Consistent Metadata**: Standard Kubernetes ObjectMeta fields are included for all resources
✅ **Flexible Demo Data**: Configurable demo data generation for any resource type

## Prerequisites

Before adding GraphQL support to your resource, ensure you have:

- [ ] An existing App SDK-backed resource with CUE kinds defined
- [ ] A working REST API endpoint (`/apis/<group>/<version>/<resource>`)
- [ ] An app provider registered in the Grafana apps registry
- [ ] REST storage implementation (typically a `legacyStorage` type)

## Two Approaches for Adding GraphQL Support

The modular GraphQL system offers two approaches for adding GraphQL support to your resources:

### Approach 1: Simple Handler (Recommended for Basic Resources)

For simple resources that don't need complex GraphQL field mapping, use the `SimpleResourceHandler`:

```go
// pkg/registry/apps/myapp/register.go
func (p *MyAppProvider) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
    // Get the group version for your app
    gv := schema.GroupVersion{
        Group:   myappv0alpha1.MyResourceKind().Group(),
        Version: myappv0alpha1.MyResourceKind().Version(),
    }

    // Get the managed kinds
    kinds := []resource.Kind{
        myappv0alpha1.MyResourceKind(),
        // Add additional kinds if your app manages multiple resources
    }

    // Create a storage adapter that bridges GraphQL to existing REST storage
    storageGetter := func(gvr schema.GroupVersionResource) graphqlsubgraph.Storage {
        // Only handle your app's resources
        expectedGVR := schema.GroupVersionResource{
            Group:    gv.Group,
            Version:  gv.Version,
            Resource: myappv0alpha1.MyResourceKind().Plural(),
        }

        if gvr != expectedGVR {
            return nil
        }

        // Return a storage adapter that wraps the legacy storage
        legacyStore := p.legacyStorageGetter(gvr)
        if legacyStore == nil {
            return nil
        }

        return &myAppStorageAdapter{
            legacyStorage: legacyStore,
            namespacer:    request.GetNamespaceMapper(p.cfg),
        }
    }

    // Create a simple handler for basic GraphQL support
    handler := graphqlsubgraph.NewSimpleResourceHandler(myappv0alpha1.MyResourceKind()).
        WithDemoData(func() interface{} {
            return map[string]interface{}{
                "metadata": map[string]interface{}{
                    "name":      "demo-myresource",
                    "namespace": "default",
                },
                "spec": `{"title": "Demo Resource"}`,
            }
        })

    // Create the subgraph with the handler
    return graphqlsubgraph.CreateSubgraphWithHandlers(
        graphqlsubgraph.SubgraphProviderConfig{
            GroupVersion:  gv,
            Kinds:         kinds,
            StorageGetter: storageGetter,
        },
        handler,
    )
}
```

### Approach 2: Custom Handler (For Complex Resources)

For resources with complex GraphQL requirements (like playlists), create a dedicated handler:

```go
// pkg/registry/apps/myapp/graphql_handler.go
package myapp

import (
    "github.com/grafana/grafana-app-sdk/resource"
    graphqlsubgraph "github.com/grafana/grafana-app-sdk/graphql/subgraph"
    "github.com/graphql-go/graphql"
    myappv0alpha1 "github.com/grafana/grafana/apps/myapp/pkg/apis/myapp/v0alpha1"
)

type myAppGraphQLHandler struct{}

func NewMyAppGraphQLHandler() graphqlsubgraph.ResourceGraphQLHandler {
    return &myAppGraphQLHandler{}
}

func (h *myAppGraphQLHandler) GetResourceKind() resource.Kind {
    return myappv0alpha1.MyResourceKind()
}

func (h *myAppGraphQLHandler) GetGraphQLFields() graphql.Fields {
    return graphql.Fields{
        "customField": &graphql.Field{Type: graphql.String},
        "items":       &graphql.Field{Type: graphql.NewList(graphql.String)},
    }
}

func (h *myAppGraphQLHandler) ConvertResourceToGraphQL(obj resource.Object) map[string]interface{} {
    // Custom conversion logic specific to your resource
    return map[string]interface{}{
        "customField": "custom value",
        "items":       []string{"item1", "item2"},
    }
}

func (h *myAppGraphQLHandler) CreateDemoData() interface{} {
    return map[string]interface{}{
        "metadata": map[string]interface{}{
            "name": "demo-myresource",
        },
        "customField": "demo value",
        "items":       []string{"demo item"},
    }
}
```

Then register it in your provider:

```go
// pkg/registry/apps/myapp/register.go
func (p *MyAppProvider) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
    // ... storage setup ...

    // Create resource handler registry and register your custom handler
    resourceHandlers := graphqlsubgraph.NewResourceHandlerRegistry()
    resourceHandlers.RegisterHandler(NewMyAppGraphQLHandler())

    return graphqlsubgraph.CreateSubgraphFromConfig(graphqlsubgraph.SubgraphProviderConfig{
        GroupVersion:     gv,
        Kinds:            kinds,
        StorageGetter:    storageGetter,
        ResourceHandlers: resourceHandlers,
    })
}
```

## Generated GraphQL Schema

The modular system automatically generates a complete GraphQL schema for your resource with:

### Query Fields (Auto-Generated)

- `<app>_<resource>(namespace: String!, name: String!)` - Get single resource
- `<app>_<resources>(namespace: String!)` - List resources
- `demo_<resource>()` - Demo data for testing (configurable)

### Types (Auto-Generated)

- `<ResourceKind>` - Individual resource type with standardized structure:
  - `metadata: ObjectMeta` - Complete Kubernetes metadata
  - `spec: String` - JSON scalar (enhanced structured types in future phases)
- `<ResourceKind>List` - List wrapper with `items` field
- `ObjectMeta` - Standard Kubernetes metadata with full field support:
  - `name`, `namespace`, `uid`, `resourceVersion`, `generation`
  - `creationTimestamp`, `labels`, `annotations`

### Features

✅ **Consistent Structure**: All resources follow the same GraphQL pattern
✅ **Complete Metadata**: Full Kubernetes ObjectMeta fields included
✅ **Error Handling**: Proper argument validation and error messages
✅ **Demo Support**: Optional demo data for easy testing

## Testing Your Implementation

### 1. Verify Registration

Check that your subgraph is registered:

```bash
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { queryType { fields { name } } } }"}'
```

Look for your prefixed fields like `myapp_myresource` and `myapp_myresources`.

### 2. Test Basic Queries

```graphql
# Get a single resource
{
  myapp_myresource(namespace: "default", name: "test-resource") {
    metadata {
      name
      namespace
      creationTimestamp
    }
    spec
  }
}

# List resources
{
  myapp_myresources(namespace: "default") {
    items {
      metadata {
        name
      }
      spec
    }
  }
}
```

### 3. Test with curl

```bash
# Test single resource query
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresource(namespace: \"default\", name: \"test\") { metadata { name } spec } }"
  }'

# Test list query
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresources(namespace: \"default\") { items { metadata { name } } } }"
  }'
```

## Troubleshooting

### Common Issues

1. **"Unknown field" errors**: Check that your provider implements `GraphQLSubgraphProvider` and is properly registered
2. **"No storage available" errors**: Verify your `storageGetter` function and `legacyStorageGetter` implementation
3. **Interface conversion errors**: Ensure your storage adapter implements all required methods
4. **Missing fields in schema**: Check that your resource kinds are properly defined and returned by `GetKinds()`

### Debug Logging

The GraphQL system includes extensive debug logging. Look for log messages prefixed with 🔍 to trace:

- Subgraph creation and registration
- Schema generation process
- Storage adapter operations
- Field resolution

### Verification Steps

1. **Check REST API first**: Ensure `/apis/<group>/<version>/<resource>` works
2. **Verify app provider registration**: Check that your provider appears in the apps registry
3. **Test subgraph creation**: Verify `GetGraphQLSubgraph()` returns without errors
4. **Inspect generated schema**: Use GraphQL introspection to see your fields

## Example: Complete Playlist Implementation

For reference, see the complete playlist implementation:

- `grafana/pkg/registry/apps/playlist/register.go` - App provider and GraphQL subgraph
- `grafana/pkg/registry/apps/playlist/graphql_storage.go` - Storage adapter
- `grafana/pkg/registry/apps/apps.go` - Registry integration
- `grafana/pkg/registry/apps/wireset.go` - Wire dependency injection

## Field Naming Convention

Resources are exposed with prefixed field names to avoid conflicts:

- Format: `<app-name>_<resource>` and `<app-name>_<resources>`
- Examples: `playlist_playlist`, `dashboard_dashboard`, `myapp_myresource`
- The prefix is derived from the first part of your app's group name

## Future Enhancements

- **Enhanced Type Mapping**: Phase 3 will include proper GraphQL type generation from CUE specs
- **Cross-app Relationships**: Automatic relationship resolution between resources
- **Advanced Filtering**: Support for complex filtering and search operations
- **Real-time Subscriptions**: GraphQL subscriptions for real-time updates

---

## Modular Design Benefits

The refactored GraphQL federation system provides significant improvements for developers:

### Before (Resource-Specific Implementation)

- ❌ Hard-coded playlist-specific GraphQL types
- ❌ Manual type definitions for each resource
- ❌ Resource-specific conversion functions
- ❌ Duplicate resolver logic for similar resources

### After (Generic Modular System)

- ✅ **Zero Boilerplate**: No resource-specific GraphQL code needed
- ✅ **Automatic Generation**: Types and resolvers generated from CUE kinds
- ✅ **Consistent API**: All resources follow the same GraphQL patterns
- ✅ **Future-Proof**: Enhanced type mapping applies to all resources

### What This Means for You

1. **Faster Development**: Adding a new resource requires only the storage adapter
2. **Consistency**: All GraphQL APIs follow the same structure and behavior
3. **Maintainability**: Changes to the core system benefit all resources
4. **Flexibility**: Easy to add new metadata fields or query capabilities

This pattern provides a consistent, scalable way to extend Grafana's GraphQL federation system while reusing existing storage infrastructure and maintaining backward compatibility with REST APIs.
