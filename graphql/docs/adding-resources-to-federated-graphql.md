# Adding Resources to GraphQL Federation

This guide provides a step-by-step recipe for extending Grafana's GraphQL federation system to include new App SDK-backed resources that are registered in the Grafana apps registry.

## Overview

The GraphQL federation system automatically discovers and integrates App SDK-backed resources by:

1. **Auto-discovery**: Scanning app providers that implement `GraphQLSubgraphProvider`
2. **Dynamic schema generation**: Converting any CUE kinds to GraphQL types and resolvers automatically
3. **Generic storage bridging**: Reusing existing REST storage without data migration
4. **Field prefixing**: Namespacing fields to avoid conflicts (e.g., `playlist_playlist`)

## Auto-Discovery from apps.go Registry

The GraphQL federation system automatically discovers resources from `pkg/registry/apps/apps.go`. Any provider that implements `GraphQLSubgraphProvider` is automatically included in the federated GraphQL schema.

### How Auto-Discovery Works

1. **Provider Registration**: Resources are registered in `apps.go`:

   ```go
   // In pkg/registry/apps/apps.go
   providers := []app.Provider{
       playlistAppProvider,           // ✅ Has GraphQL support
       investigationAppProvider,      // ✅ Has GraphQL support
       advisorAppProvider,           // 🔄 Ready for GraphQL support
       alertingNotificationsAppProvider, // 🔄 Ready for GraphQL support
   }
   ```

2. **Automatic GraphQL Discovery**: The service automatically finds GraphQL-capable providers:

   ```go
   // The Service.GetGraphQLProviders() method finds all GraphQL-capable providers
   func (s *Service) GetGraphQLProviders() []graphqlsubgraph.GraphQLSubgraphProvider {
       var graphqlProviders []graphqlsubgraph.GraphQLSubgraphProvider
       for _, provider := range s.providers {
           if graphqlProvider, ok := provider.(graphqlsubgraph.GraphQLSubgraphProvider); ok {
               graphqlProviders = append(graphqlProviders, graphqlProvider)
           }
       }
       return graphqlProviders
   }
   ```

3. **Schema Integration**: Each discovered provider's subgraph is automatically merged into the federated schema with proper field prefixing.

### Adding GraphQL Support to Any apps.go Resource

To add GraphQL support to **any** resource already registered in `apps.go`:

1. **Implement the Interface**: Add `GraphQLSubgraphProvider` to your existing provider:

   ```go
   // Ensure your provider implements GraphQLSubgraphProvider
   var _ graphqlsubgraph.GraphQLSubgraphProvider = (*YourAppProvider)(nil)

   func (p *YourAppProvider) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
       // Implementation here
   }
   ```

2. **No Additional Registration Required**: The GraphQL system automatically discovers your implementation through the existing `apps.go` registration.

3. **Automatic Schema Inclusion**: Your resource's GraphQL fields are automatically included in the federated schema.

### Feature Flag Considerations

Some resources in `apps.go` are feature-flagged. The GraphQL discovery respects these flags:

```go
// Resources with feature flags are conditionally included
if features.IsEnabledGlobally(featuremgmt.FlagInvestigationsBackend) {
    providers = append(providers, investigationAppProvider)
}
```

GraphQL support will automatically follow the same feature flag logic - if a resource is conditionally included in `apps.go`, its GraphQL support will be conditionally included as well.

## Key Benefits of the Modular Design

✅ **Zero Resource-Specific Code**: No need to write resource-specific GraphQL types or resolvers
✅ **Automatic Type Generation**: GraphQL types are dynamically created from your CUE kinds
✅ **Generic Conversion**: All resource.Object types are automatically converted to GraphQL format
✅ **Consistent Metadata**: Standard Kubernetes ObjectMeta fields are included for all resources

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
    handler := graphqlsubgraph.NewSimpleResourceHandler(myappv0alpha1.MyResourceKind())

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

## Storage Adapter Requirements

### ⚠️ CRITICAL: TypeMeta Must Be Set

**Most Common Issue**: Your storage adapter **MUST** set proper `TypeMeta` on resource objects, or resource handlers won't be called during data conversion.

```go
// ❌ WRONG - Missing TypeMeta causes null values in GraphQL responses
func (a *myStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // ... get data from service ...

    return &myappv0alpha1.MyResource{
        ObjectMeta: metav1.ObjectMeta{
            Name:      dto.ID,
            Namespace: namespace,
        },
        Spec: myappv0alpha1.MyResourceSpec{
            Title: dto.Title,
        },
    }, nil
}

// ✅ CORRECT - With TypeMeta, handlers work properly
func (a *myStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // ... get data from service ...

    return &myappv0alpha1.MyResource{
        TypeMeta: metav1.TypeMeta{
            APIVersion: myappv0alpha1.GroupVersion.String(), // e.g., "myapp.grafana.app/v0alpha1"
            Kind:       "MyResource",                         // Must match your Kind name exactly
        },
        ObjectMeta: metav1.ObjectMeta{
            Name:      dto.ID,
            Namespace: namespace,
        },
        Spec: myappv0alpha1.MyResourceSpec{
            Title: dto.Title,
        },
    }, nil
}
```

**Why This Matters**: Resource handlers are looked up using `staticMetadata.Kind`. Without TypeMeta, this field is empty and handlers aren't called, resulting in null values for custom fields.

### Storage Adapter Checklist

When implementing your storage adapter, ensure you:

- [ ] **Set TypeMeta** with correct `APIVersion` and `Kind`
- [ ] **Set ObjectMeta** with `Name`, `Namespace`, and other metadata
- [ ] **Convert service DTOs** to proper CUE-defined Spec structures
- [ ] **Handle both Get and List** methods consistently
- [ ] **Test with actual data** to verify custom fields appear

## Troubleshooting

### Common Issues

1. **"Unknown field" errors**: Check that your provider implements `GraphQLSubgraphProvider` and is properly registered

2. **"No storage available" errors**: Verify your `storageGetter` function and `legacyStorageGetter` implementation

3. **Interface conversion errors**: Ensure your storage adapter implements all required methods

4. **Missing fields in schema**: Check that your resource kinds are properly defined and returned by `GetKinds()`

5. **🚨 Fields in schema but returning null values**:

   - **Most likely cause**: Storage adapter isn't setting `TypeMeta`
   - **Debug**: Check if `staticMetadata.Kind` is empty in your resource objects
   - **Fix**: Add proper `TypeMeta` to all resource objects in your storage adapter

6. **Custom fields not appearing**:
   - **Cause**: Resource handler not being called during conversion
   - **Debug**: Verify `GetHandlerByKindName(kindName)` finds your handler
   - **Fix**: Ensure `TypeMeta.Kind` matches your handler's `GetResourceKind().Kind()`

### Debug Approaches

#### Two-Phase Debugging

GraphQL issues typically fall into two categories:

1. **Schema Generation Issues** (fields missing from schema):

   - Use introspection queries to check available fields
   - Verify resource handlers are registered
   - Check `GetGraphQLFields()` method

2. **Data Conversion Issues** (fields in schema but returning null):
   - Verify TypeMeta is set on resource objects
   - Check if resource handlers are being called
   - Validate storage adapter implementation

#### Debugging Commands

```bash
# 1. Check schema has your fields
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __type(name: \"MyResource\") { fields { name type { name } } } }"}'

# 2. Test actual data query
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ myapp_myresources(namespace: \"default\") { items { customField } } }"}'

# 3. Compare with REST API
curl http://localhost:3000/apis/myapp.grafana.app/v0alpha1/namespaces/default/myresources
```

#### Quick TypeMeta Test

Add this to your storage adapter temporarily to verify TypeMeta:

```go
func (a *myStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // ... your existing code ...

    // Debug: Log the TypeMeta
    fmt.Printf("🔍 Resource TypeMeta: APIVersion=%s, Kind=%s\n",
               resource.APIVersion, resource.Kind)

    return resource, nil
}
```

### Debug Logging

The GraphQL system includes extensive debug logging. Look for log messages prefixed with 🔍 to trace:

- Subgraph creation and registration
- Schema generation process
- Storage adapter operations
- Field resolution
- Resource handler lookup and conversion

### Verification Steps

1. **Check REST API first**: Ensure `/apis/<group>/<version>/<resource>` works
2. **Verify app provider registration**: Check that your provider appears in the apps registry
3. **Test subgraph creation**: Verify `GetGraphQLSubgraph()` returns without errors
4. **Inspect generated schema**: Use GraphQL introspection to see your fields
5. **Test with real data**: Query actual resources, not just demo data
6. **Verify TypeMeta**: Ensure all resource objects have proper Kind set

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

1. **Faster Development**: Adding a new resource requires only the storage adapter (with proper TypeMeta!)
2. **Consistency**: All GraphQL APIs follow the same structure and behavior
3. **Maintainability**: Changes to the core system benefit all resources
4. **Flexibility**: Easy to add new metadata fields or query capabilities

This pattern provides a consistent, scalable way to extend Grafana's GraphQL federation system while reusing existing storage infrastructure and maintaining backward compatibility with REST APIs.

---

## Important Notes for Maintainers

### GraphQL Provider Consistency

If you have multiple GraphQL providers for the same resource (e.g., one in `pkg/api/graphql.go` and one in `pkg/registry/apps/`), ensure **both** use resource handlers. Missing resource handlers in any provider will cause regressions where fields are available in the schema but return null values.

### TypeMeta is Non-Negotiable

The most critical requirement: **All resource objects MUST have TypeMeta set**. This is not optional for GraphQL to work correctly. The system uses `staticMetadata.Kind` to look up resource handlers, and this field comes from TypeMeta.

### Testing Checklist

When adding GraphQL support:

- [ ] Test that fields appear in schema (introspection)
- [ ] Test that fields return actual data (not null)
- [ ] Test both single resource and list queries
- [ ] Verify TypeMeta is set in storage adapter
- [ ] Test with real data, not just demo data
