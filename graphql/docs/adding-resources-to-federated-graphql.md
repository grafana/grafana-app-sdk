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
   // In pkg/registry/apps/apps.go - providers are automatically discovered
   providers := []app.Provider{
       playlistAppProvider,           // ✅ Has GraphQL support
       investigationAppProvider,      // ✅ Has GraphQL support
       advisorAppProvider,           // 🔄 Ready for GraphQL support
       alertingNotificationsAppProvider, // 🔄 Ready for GraphQL support
       yourAppProvider,              // 🔄 Add GraphQL support here
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

**No additional registration required** - GraphQL support is automatically discovered through existing app registry.

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

## Three Approaches for Adding GraphQL Support

The modular GraphQL system offers three approaches for adding GraphQL support to your resources:

### Approach 1: App Platform Resources (apps.go Registry)

This is the standard approach for App SDK-backed resources that are already registered in the `apps.go` registry.

### Approach 2: Traditional APIs (apis.go Registry)

For traditional Kubernetes-style APIs registered in the `apis.go` registry (like Dashboards, DataSources, etc.), use the **API Builder Extension** pattern.

### Approach 3: Simple/Custom Handlers

For both app platform and traditional APIs, you can customize the GraphQL implementation.

## Approach 1: App Platform Resources (apps.go Registry)

### Simple Handler (Recommended for Basic Resources)

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

## Approach 2: Traditional APIs (apis.go Registry)

For traditional Kubernetes-style APIs that are registered in the `apis.go` registry (like Dashboards, DataSources, Folders, etc.), use the **API Builder Extension** pattern.

### Overview

Traditional APIs in the `apis.go` registry use the `APIGroupBuilder` interface. To add GraphQL support:

1. **Extend the interface**: Implement `GraphQLCapableBuilder` on your existing API builder
2. **Create storage adapter**: Bridge your existing k8s-style storage to GraphQL storage interface
3. **Auto-discovery**: The system automatically discovers and registers GraphQL-capable builders

### Step 1: Implement GraphQLCapableBuilder Interface

```go
// pkg/registry/apis/myapi/register.go

// Ensure your existing API builder also implements GraphQLCapableBuilder
var (
    _ builder.APIGroupBuilder       = (*MyAPIBuilder)(nil)
    _ builder.GraphQLCapableBuilder = (*MyAPIBuilder)(nil) // Add this line
)

// Add the GraphQL subgraph method to your existing API builder
func (b *MyAPIBuilder) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
    // Create storage adapter that bridges your existing storage to GraphQL
    storageAdapter := NewMyAPIStorageAdapter(
        b.legacyService,    // Your existing service
        b.namespaceMapper,  // Namespace mapping function
    )

    // Create the GraphQL subgraph
    subgraph, err := graphqlsubgraph.New(graphqlsubgraph.SubgraphConfig{
        GroupVersion: b.resourceInfo.GroupVersion(),
        Kinds:        []sdkresource.Kind{MyAPIResourceKind()},
        StorageGetter: func(gvr schema.GroupVersionResource) graphqlsubgraph.Storage {
            return storageAdapter
        },
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create MyAPI GraphQL subgraph: %w", err)
    }

    return subgraph, nil
}
```

### Step 2: Create Storage Adapter

Create a storage adapter that bridges your existing k8s-style storage to the GraphQL storage interface:

```go
// pkg/registry/apis/myapi/graphql_storage.go
package myapi

import (
    "context"
    "fmt"

    graphqlsubgraph "github.com/grafana/grafana-app-sdk/graphql/subgraph"
    "github.com/grafana/grafana-app-sdk/resource"
    myapiv1 "github.com/grafana/grafana/pkg/apis/myapi/v1alpha1"
    "github.com/grafana/grafana/pkg/apimachinery/identity"
    "github.com/grafana/grafana/pkg/services/apiserver/endpoints/request"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    k8srequest "k8s.io/apiserver/pkg/endpoints/request"
)

type myAPIStorageAdapter struct {
    legacyService   MyAPILegacyService  // Your existing service
    namespaceMapper request.NamespaceMapper
}

func NewMyAPIStorageAdapter(
    legacyService MyAPILegacyService,
    namespaceMapper request.NamespaceMapper,
) graphqlsubgraph.Storage {
    return &myAPIStorageAdapter{
        legacyService:   legacyService,
        namespaceMapper: namespaceMapper,
    }
}

// setupContextWithNamespace sets up the proper context with namespace information
func (s *myAPIStorageAdapter) setupContextWithNamespace(ctx context.Context, namespace string) (context.Context, error) {
    user, err := identity.GetRequester(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to get user from context: %w", err)
    }

    orgID := user.GetOrgID()
    if orgID <= 0 {
        return nil, fmt.Errorf("invalid org ID: %d", orgID)
    }

    // Format the namespace using the namespace mapper
    properNamespace := s.namespaceMapper(orgID)
    ctx = k8srequest.WithNamespace(ctx, properNamespace)

    return ctx, nil
}

// Get implements graphqlsubgraph.Storage
func (s *myAPIStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    ctx, err := s.setupContextWithNamespace(ctx, namespace)
    if err != nil {
        return nil, err
    }

    // Get from your legacy service
    item, err := s.legacyService.Get(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("failed to get resource: %w", err)
    }

    // 🚨 CRITICAL: Ensure TypeMeta is set for GraphQL to work
    if item.TypeMeta.APIVersion == "" {
        item.TypeMeta.APIVersion = myapiv1.MyResourceInfo.GroupVersion().String()
    }
    if item.TypeMeta.Kind == "" {
        item.TypeMeta.Kind = "MyResource"
    }

    return resource.NewUnstructuredWrapper(s.toUnstructured(item)), nil
}

// List implements graphqlsubgraph.Storage
func (s *myAPIStorageAdapter) List(ctx context.Context, namespace string, options graphqlsubgraph.ListOptions) (resource.ListObject, error) {
    ctx, err := s.setupContextWithNamespace(ctx, namespace)
    if err != nil {
        return nil, err
    }

    // Get list from your legacy service
    items, err := s.legacyService.List(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to list resources: %w", err)
    }

    // Convert to GraphQL format
    resourceItems := make([]resource.Object, len(items))
    for i, item := range items {
        // 🚨 CRITICAL: Ensure TypeMeta is set for GraphQL to work
        if item.TypeMeta.APIVersion == "" {
            item.TypeMeta.APIVersion = myapiv1.MyResourceInfo.GroupVersion().String()
        }
        if item.TypeMeta.Kind == "" {
            item.TypeMeta.Kind = "MyResource"
        }

        resourceItems[i] = resource.NewUnstructuredWrapper(s.toUnstructured(&item))
    }

    return &resource.UntypedList{
        TypeMeta: metav1.TypeMeta{
            APIVersion: myapiv1.MyResourceInfo.GroupVersion().String(),
            Kind:       "MyResourceList",
        },
        Items: resourceItems,
    }, nil
}

// Helper method to convert to unstructured format
func (s *myAPIStorageAdapter) toUnstructured(item *myapiv1.MyResource) *unstructured.Unstructured {
    obj := &unstructured.Unstructured{}
    obj.SetUnstructuredContent(map[string]interface{}{
        "apiVersion": myapiv1.MyResourceInfo.GroupVersion().String(),
        "kind":       "MyResource",
        "metadata": map[string]interface{}{
            "name":              item.Name,
            "namespace":         item.Namespace,
            "uid":               string(item.UID),
            "resourceVersion":   item.ResourceVersion,
            "generation":        item.Generation,
            "creationTimestamp": item.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z"),
            "labels":            item.Labels,
        },
        "spec": item.Spec, // Your resource's spec
    })
    return obj
}

// Create/Update/Delete implementations (usually not needed for read-only GraphQL)
func (s *myAPIStorageAdapter) Create(ctx context.Context, namespace string, obj resource.Object) (resource.Object, error) {
    return nil, fmt.Errorf("create not implemented via GraphQL")
}

func (s *myAPIStorageAdapter) Update(ctx context.Context, namespace, name string, obj resource.Object) (resource.Object, error) {
    return nil, fmt.Errorf("update not implemented via GraphQL")
}

func (s *myAPIStorageAdapter) Delete(ctx context.Context, namespace, name string) error {
    return fmt.Errorf("delete not implemented via GraphQL")
}
```

### Step 3: Define Resource Kind

```go
// pkg/registry/apis/myapi/resource_kind.go (or add to existing file)
package myapi

import (
    "github.com/grafana/grafana-app-sdk/resource"
)

func MyAPIResourceKind() resource.Kind {
    schema := resource.NewSimpleSchema(
        "myapi.grafana.app",
        "v1alpha1",
        &resource.UntypedObject{},
        &resource.UntypedList{},
        resource.WithKind("MyResource"),
        resource.WithPlural("myresources"),
        resource.WithScope(resource.NamespacedScope),
    )

    return resource.Kind{
        Schema: schema,
        Codecs: map[resource.KindEncoding]resource.Codec{
            resource.KindEncodingJSON: resource.NewJSONCodec(),
        },
    }
}
```

### Step 4: Auto-Discovery (No Additional Registration Needed!)

The GraphQL system automatically discovers your API builder because it's already registered in `apis.go`. The auto-discovery mechanism:

1. Scans all registered API builders in `apis.go`
2. Checks if they implement `GraphQLCapableBuilder` interface
3. Automatically calls `GetGraphQLSubgraph()` and includes them in the federated schema

**No changes needed to `apis.go`** - your existing registration automatically works!

### Special Case: Unified Access Across Multiple Providers

Some APIs (like DataSources) have multiple plugin-specific providers. For these cases, you may want unified GraphQL access across all providers:

```go
// Only enable unified GraphQL on one provider to avoid conflicts
var firstPluginForGraphQL bool = true

for _, plugin := range plugins {
    builder := NewMyAPIBuilder(plugin)

    // Enable unified GraphQL support on the first plugin only
    if firstPluginForGraphQL {
        builder.enableUnifiedGraphQL = true
        builder.unifiedService = service    // Service that can access ALL providers
        builder.unifiedCache = cache
        firstPluginForGraphQL = false
    }

    apiRegistrar.RegisterAPI(builder)
}
```

Then in your `GetGraphQLSubgraph()` method:

```go
func (b *MyAPIBuilder) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
    // Only provide GraphQL if this builder has unified access enabled
    if !b.enableUnifiedGraphQL {
        return nil, fmt.Errorf("GraphQL not enabled on this builder")
    }

    // Use unified storage adapter that can access ALL providers
    storageAdapter := NewUnifiedStorageAdapter(b.unifiedService, b.unifiedCache)
    // ... rest of implementation
}
```

This prevents multiple GraphQL subgraphs for the same logical resource while maintaining separate REST APIs for each provider.

### Real-World Examples

- **Dashboard API**: See `grafana/pkg/registry/apis/dashboard/` for complete implementation
- **DataSource API**: See `grafana/pkg/registry/apis/datasource/` for unified multi-provider access pattern

### Key Differences from App Platform Approach

| App Platform (apps.go)                            | Traditional APIs (apis.go)                     |
| ------------------------------------------------- | ---------------------------------------------- |
| App providers implement `GraphQLSubgraphProvider` | API builders implement `GraphQLCapableBuilder` |
| Uses App SDK storage directly                     | Requires storage adapter to bridge k8s storage |
| Auto-discovered from apps registry                | Auto-discovered from APIs registry             |
| CUE-defined resources                             | Traditional k8s API resources                  |

### Custom Handler (For Complex Resources)

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
curl -X POST http://localhost:3000/apis/graphql \
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
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresource(namespace: \"default\", name: \"test\") { metadata { name } spec } }"
  }'

# Test list query
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresources(namespace: \"default\") { items { metadata { name } } } }"
  }'
```

## Storage Adapter Requirements

### 🚨 #1 Most Common Issue: TypeMeta Not Set

**Symptom**: Fields appear in GraphQL schema but return `null` values
**Cause**: Storage adapter not setting `TypeMeta` on resource objects
**Fix**: Ensure your storage adapter sets proper TypeMeta:

```go
// ensureTypeMetaSet ensures that the TypeMeta is properly set on a resource object
// This is critical for GraphQL resource handlers to be called during conversion
func (a *myStorageAdapter) ensureTypeMetaSet(obj resource.Object) {
    gvk := obj.GroupVersionKind()
    if gvk.Kind == "" || gvk.Version == "" {
        kind := myappv0alpha1.MyResourceKind()
        obj.SetGroupVersionKind(schema.GroupVersionKind{
            Group:   kind.Group(),
            Version: kind.Version(),
            Kind:    kind.Kind(),
        })
    }
}

func (a *myStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // ... get data from service ...

    // ✅ CRITICAL: Ensure TypeMeta is set for GraphQL resource handlers to work
    a.ensureTypeMetaSet(resourceObj)

    return resourceObj, nil
}

func (a *myStorageAdapter) List(ctx context.Context, namespace string, options graphqlsubgraph.ListOptions) (resource.ListObject, error) {
    // ... get list from service ...

    // ✅ CRITICAL: Ensure TypeMeta is set on all items for GraphQL resource handlers to work
    items := listObj.GetItems()
    for _, item := range items {
        a.ensureTypeMetaSet(item)
    }

    return listObj, nil
}
```

**Why This Matters**: Resource handlers are looked up using `staticMetadata.Kind`. Without TypeMeta, this field is empty and handlers aren't called, resulting in null values for custom fields.

### Storage Adapter Checklist

When implementing your storage adapter, ensure you:

- [ ] **Set TypeMeta** with correct `APIVersion` and `Kind` (MOST IMPORTANT)
- [ ] **Set ObjectMeta** with `Name`, `Namespace`, and other metadata
- [ ] **Convert service DTOs** to proper CUE-defined Spec structures
- [ ] **Handle both Get and List** methods consistently
- [ ] **Test with actual data** to verify custom fields appear

## Real-World Example: Playlist Implementation

See the complete working implementation that demonstrates the TypeMeta fix:

- `grafana/pkg/registry/apps/playlist/register.go` - GraphQL subgraph setup
- `grafana/pkg/registry/apps/playlist/graphql_storage.go` - Storage adapter with TypeMeta fix
- `grafana/pkg/registry/apps/playlist/graphql_handler.go` - Custom field handling

**Key Insight**: The TypeMeta fix in the storage adapter was critical for making custom fields work. Before the fix, fields like `name`, `uid`, `interval`, and `items` returned null values even though they appeared in the schema.

## Performance Features

### Query Batching

Automatic batching prevents N+1 query problems:

- Related queries are automatically batched
- Configurable batch sizes and timeouts
- Cross-app relationship queries optimized

### Intelligent Caching

Multi-level caching with automatic invalidation:

- Resource-level caching with TTL
- Query result caching
- Cross-app relationship caching

### Complexity Analysis

Prevents expensive queries:

- Configurable complexity limits
- Query depth analysis
- Field-level complexity scoring

## Troubleshooting

### Common Issues

1. **🚨 Fields in schema but returning null values** (MOST COMMON):

   - **Most likely cause**: Storage adapter isn't setting `TypeMeta`
   - **Debug**: Check if `staticMetadata.Kind` is empty in your resource objects
   - **Fix**: Add proper `TypeMeta` to all resource objects in your storage adapter

2. **"Unknown field" errors**:

   - **For App Platform**: Check that your provider implements `GraphQLSubgraphProvider` and is properly registered
   - **For Traditional APIs**: Check that your API builder implements `GraphQLCapableBuilder`

3. **"No storage available" errors**:

   - **For App Platform**: Verify your `storageGetter` function and `legacyStorageGetter` implementation
   - **For Traditional APIs**: Verify your storage adapter implements all required methods in `graphqlsubgraph.Storage`

4. **Interface conversion errors**: Ensure your storage adapter implements all required methods

5. **Missing fields in schema**:

   - **For App Platform**: Check that your resource kinds are properly defined and returned by `GetKinds()`
   - **For Traditional APIs**: Check that your `MyAPIResourceKind()` function returns correct schema definition

6. **Custom fields not appearing**:

   - **Cause**: Resource handler not being called during conversion
   - **Debug**: Verify `GetHandlerByKindName(kindName)` finds your handler
   - **Fix**: Ensure `TypeMeta.Kind` matches your handler's `GetResourceKind().Kind()`

7. **Multiple subgraphs for same resource** (Traditional APIs):

   - **Cause**: Multiple API builders providing GraphQL for the same logical resource
   - **Fix**: Use unified access pattern - only enable GraphQL on one builder

8. **Namespace/context errors** (Traditional APIs):
   - **Cause**: Missing or incorrect namespace mapping
   - **Debug**: Check that `setupContextWithNamespace()` is called and working
   - **Fix**: Ensure proper `NamespaceMapper` implementation

### Runtime Issues

**9. "GraphQL not enabled on this DataSource builder" or similar provider errors**:

- **Cause**: Auto-discovery logic not implemented in API server
- **Debug**: Check if `globalGraphQLRegistry.RegisterProvider()` is being called during startup
- **Fix**: Add the auto-discovery implementation to your API server service (see "Auto-Discovery Implementation Requirements" below)

**10. `runtime error: invalid memory address or nil pointer dereference` in `createFederatedGateway`**:

- **Cause**: Federation gateway not checking for nil subgraphs from providers
- **Debug**: Check if the error occurs when calling `subgraph.GetGroupVersion()`
- **Fix**: Add nil check in `createFederatedGateway` before calling methods on subgraph:

```go
// Skip providers that don't provide GraphQL support (return nil subgraph)
if subgraph == nil {
    continue
}
```

**11. "Subgraph already registered" duplicate registration errors**:

- **Cause**: Multiple providers trying to register the same GraphQL subgraph
- **Debug**: Check if you have multiple builders for the same resource (e.g., multiple DataSource plugin builders)
- **Fix**: Add duplicate detection to prevent multiple registrations:

```go
// Track registered group versions to avoid duplicates
registeredGVs := make(map[string]bool)

for _, provider := range graphqlProviders {
    if subgraph, err := provider.GetGraphQLSubgraph(); err == nil && subgraph != nil {
        gv := subgraph.GetGroupVersion()
        gvKey := gv.String()

        // Skip if this group version is already registered
        if registeredGVs[gvKey] {
            continue
        }

        globalGraphQLRegistry.RegisterProvider(provider)
        registeredGVs[gvKey] = true
    }
}
```

**12. Wire dependency injection errors for `NamespaceMapper`**:

- **Cause**: Missing provider for `github.com/grafana/grafana/pkg/services/apiserver/endpoints/request.NamespaceMapper`
- **Debug**: Check wire error messages mentioning `NamespaceMapper`
- **Fix**: Change storage adapter to accept `*setting.Cfg` instead and create mapper internally:

```go
// Instead of expecting NamespaceMapper as parameter
func NewMyStorageAdapter(cfg *setting.Cfg, ...) Storage {
    return &myStorageAdapter{
        namespaceMapper: request.GetNamespaceMapper(cfg),
        // ... other fields
    }
}
```

### Implementation Checklist

When adding GraphQL support to Traditional APIs, ensure:

- [ ] **Auto-discovery logic implemented** in API server service
- [ ] **Nil subgraph checks** in `createFederatedGateway` function
- [ ] **Duplicate registration prevention** in global registry
- [ ] **Proper wire dependencies** for storage adapter
- [ ] **TypeMeta set** in storage adapter (CRITICAL!)
- [ ] **Integration tests** for GraphQL queries
- [ ] **Documentation updated** with new fields and usage

### Debug Commands for Runtime Issues

```bash
# 1. Check if GraphQL endpoint is accessible
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { queryType { name } } }"}'

# 2. Check for your resource fields in schema
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { queryType { fields { name } } } }"}'

# 3. Look for error patterns in logs
grep -E "(GraphQL not enabled|nil pointer dereference|already registered)" /path/to/grafana.log

# 4. Test specific resource queries
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ datasourceconnections(namespace: \"default\") { items { metadata { name } } } }"}'
```

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

#### Additional Debugging Commands

```bash
# 1. Check schema has your fields
curl -X POST http://localhost:3000/apis/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __type(name: \"MyResource\") { fields { name type { name } } } }"}'

# 2. Test actual data query
curl -X POST http://localhost:3000/apis/graphql \
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
    gvk := resource.GroupVersionKind()
    fmt.Printf("🔍 Resource TypeMeta: Group=%s, Version=%s, Kind=%s\n",
               gvk.Group, gvk.Version, gvk.Kind)

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

#### For App Platform Resources (apps.go):

1. **Check REST API first**: Ensure `/apis/<group>/<version>/<resource>` works
2. **Verify app provider registration**: Check that your provider appears in the apps registry
3. **Test subgraph creation**: Verify `GetGraphQLSubgraph()` returns without errors
4. **Inspect generated schema**: Use GraphQL introspection to see your fields
5. **Test with real data**: Query actual resources, not just demo data
6. **Verify TypeMeta**: Ensure all resource objects have proper Kind set

#### For Traditional APIs (apis.go)

1. **Check REST API first**: Ensure `/apis/<group>/<version>/<resource>` works
2. **Verify API builder registration**: Check that your builder appears in the APIs registry
3. **Verify GraphQLCapableBuilder**: Ensure your builder implements the interface correctly
4. **Test storage adapter**: Verify your storage adapter bridges k8s storage to GraphQL storage
5. **Check namespace mapping**: Ensure proper context setup with namespace information
6. **Test subgraph creation**: Verify `GetGraphQLSubgraph()` returns without errors
7. **Inspect generated schema**: Use GraphQL introspection to see your fields
8. **Test with real data**: Query actual resources, not just demo data
9. **Verify TypeMeta**: Ensure all resource objects have proper Kind set (CRITICAL!)

## Example: Complete Playlist Implementation

For reference, see the complete playlist implementation:

- `grafana/pkg/registry/apps/playlist/register.go` - App provider and GraphQL subgraph
- `grafana/pkg/registry/apps/playlist/graphql_storage.go` - Storage adapter
- `grafana/pkg/registry/apps/apps.go` - Registry integration
- `grafana/pkg/registry/apps/wireset.go` - Wire dependency injection

## Auto-Discovery Implementation Requirements

### For Traditional APIs (apis.go Registry)

The GraphQL system includes auto-discovery for traditional APIs, but you must ensure the discovery logic is properly implemented in your API server. Here's the required implementation:

#### Required Service Integration

Add this code to your API server's service initialization (typically in `pkg/services/apiserver/service.go`):

```go
// During API server initialization, add GraphQL auto-discovery
func (s *service) initializeBuilders(builders []builder.APIGroupBuilder) error {
    // ... existing builder initialization code ...

    // Discover and register GraphQL-capable builders with the global registry
    // This enables GraphQL federation auto-discovery
    discovery := builder.NewGraphQLDiscovery()
    graphqlProviders := discovery.DiscoverFromBuilders(builders)

    // Track registered group versions to avoid duplicates during discovery
    registeredGVs := make(map[string]bool)

    // Only register providers that actually provide a subgraph (non-nil return)
    for _, provider := range graphqlProviders {
        // Test if the provider actually provides a GraphQL subgraph
        // This filters out builders that return nil (like non-unified DataSource builders)
        if subgraph, err := provider.GetGraphQLSubgraph(); err == nil && subgraph != nil {
            gv := subgraph.GetGroupVersion()
            gvKey := gv.String()

            // Skip if this group version is already registered
            if registeredGVs[gvKey] {
                continue
            }

            globalGraphQLRegistry.RegisterProvider(provider)
            registeredGVs[gvKey] = true
        }
        // If error or nil subgraph, skip registration (no GraphQL support)
    }

    return nil
}
```

#### Required Federation Gateway Updates

Ensure your `createFederatedGateway` function properly handles nil subgraphs:

```go
func (s *service) createFederatedGateway(ctx context.Context) (*gateway.Gateway, error) {
    // Get all registered GraphQL providers
    graphqlProviders := globalGraphQLRegistry.GetProviders()

    // Register each GraphQL provider's subgraph with the gateway
    for _, provider := range graphqlProviders {
        subgraph, err := provider.GetGraphQLSubgraph()
        if err != nil {
            return nil, fmt.Errorf("failed to get GraphQL subgraph from provider: %w", err)
        }

        // 🚨 CRITICAL: Skip providers that don't provide GraphQL support (return nil subgraph)
        if subgraph == nil {
            continue
        }

        // Get the group version from the subgraph
        gv := subgraph.GetGroupVersion()

        // Register the subgraph with the gateway
        if err := gatewayBuilder.RegisterSubgraph(gv.String(), subgraph); err != nil {
            return nil, fmt.Errorf("failed to register subgraph for group %s version %s: %w",
                                 gv.Group, gv.Version, err)
        }
    }

    return gatewayBuilder.Build()
}
```

#### Global Registry Implementation

You'll need a global registry to track GraphQL providers:

```go
// GraphQLProviderRegistry manages GraphQL subgraph providers
type GraphQLProviderRegistry struct {
    providers     []graphqlsubgraph.GraphQLSubgraphProvider
    registeredGVs map[string]bool // Track registered group versions
}

// RegisterProvider adds a GraphQL subgraph provider to the registry
// Includes duplicate detection to prevent registration conflicts
func (r *GraphQLProviderRegistry) RegisterProvider(provider graphqlsubgraph.GraphQLSubgraphProvider) {
    // Initialize the tracking map if needed
    if r.registeredGVs == nil {
        r.registeredGVs = make(map[string]bool)
    }

    // Test if provider actually provides a subgraph
    if subgraph, err := provider.GetGraphQLSubgraph(); err == nil && subgraph != nil {
        gv := subgraph.GetGroupVersion()
        gvKey := gv.String()

        // Skip duplicate registrations
        if r.registeredGVs[gvKey] {
            return
        }

        r.providers = append(r.providers, provider)
        r.registeredGVs[gvKey] = true
    }
}

// GetProviders returns all registered GraphQL providers
func (r *GraphQLProviderRegistry) GetProviders() []graphqlsubgraph.GraphQLSubgraphProvider {
    return r.providers
}

// Global registry instance
var globalGraphQLRegistry = &GraphQLProviderRegistry{}
```

**Why This Is Required**: Without proper auto-discovery implementation, your GraphQL-capable builders won't be registered with the federation system, leading to "GraphQL not enabled" errors.

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
