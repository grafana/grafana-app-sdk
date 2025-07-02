# GraphQL Federation for App Platform

This package provides advanced federated GraphQL support for Grafana App Platform apps, enabling multiple apps to automatically contribute their schemas to a unified GraphQL API with cross-app relationships, enhanced type mapping, and performance optimizations.

## Quick Start

### 1. Add GraphQL Support to Your App

Make your app provider implement the `GraphQLSubgraphProvider` interface:

```go
import (
    "github.com/grafana/grafana-app-sdk/graphql/subgraph"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// Your existing app provider
type MyAppProvider struct {
    app.Provider
    // ... your existing fields
}

// Add GraphQL support
func (p *MyAppProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    return subgraph.CreateSubgraphFromConfig(subgraph.SubgraphProviderConfig{
        GroupVersion: schema.GroupVersion{
            Group:   "myapp.grafana.app",
            Version: "v1alpha1",
        },
        Kinds: p.getManagedKinds(), // Your existing kinds
        StorageGetter: func(gvr schema.GroupVersionResource) subgraph.Storage {
            // Bridge to your existing storage
            return &myAppStorageAdapter{
                legacyStorage: p.legacyStorageGetter(gvr),
            }
        },
    })
}
```

### 2. Set Up Federation

Use auto-discovery to register all GraphQL-capable apps:

```go
import "github.com/grafana/grafana-app-sdk/graphql/gateway"

// Auto-discover GraphQL subgraphs from app providers
registry, err := gateway.AutoDiscovery(
    playlistProvider,
    investigationsProvider,
    myAppProvider,
)
if err != nil {
    return err
}

// Get the federated gateway
federatedGateway := registry.GetFederatedGateway()

// Set up GraphQL HTTP endpoint
http.HandleFunc("/graphql", federatedGateway.HandleGraphQL)
```

### 3. Query Across Apps

```graphql
{
  # Query playlists
  playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
      namespace
    }
    spec
  }

  # Query your app's resources
  myresource(namespace: "default", name: "my-resource") {
    metadata {
      name
      namespace
    }
    spec
  }
}
```

## Architecture

### Auto-Generated Schemas

Each app's CUE kinds are automatically converted to GraphQL with enhanced type mapping:

- **Types**: CUE structs → GraphQL objects with proper field types
- **Queries**: `get`, `list` operations for each kind with argument validation
- **Mutations**: `create`, `update`, `delete` operations with type safety
- **Metadata**: Standard Kubernetes ObjectMeta fields
- **Specs**: Structured GraphQL types with proper field mapping
- **Relationships**: Cross-app relationships via `@relation` attributes

### Storage Integration

GraphQL operations delegate to your existing REST storage:

```go
type MyAppStorageAdapter struct {
    legacyStorage rest.Storage
}

func (a *MyAppStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // Get data from existing storage
    obj, err := a.legacyStorage.Get(ctx, name, &metav1.GetOptions{})
    if err != nil {
        return nil, err
    }

    // ⚠️ CRITICAL: Ensure TypeMeta is set for resource handlers to work
    if typedObj, ok := obj.(*myappv0alpha1.MyResource); ok {
        typedObj.TypeMeta = metav1.TypeMeta{
            APIVersion: myappv0alpha1.GroupVersion.String(),
            Kind:       "MyResource",
        }
        return typedObj, nil
    }

    return obj, nil
}
```

### ⚠️ Critical: TypeMeta Must Be Set

**Most Important**: Your storage adapter **MUST** set proper `TypeMeta` on resource objects, or resource handlers won't be called during data conversion, resulting in null values.

```go
// ❌ WRONG - Missing TypeMeta causes null values in GraphQL responses
return &myappv0alpha1.MyResource{
    ObjectMeta: metav1.ObjectMeta{Name: "test"},
    Spec: myappv0alpha1.MyResourceSpec{Title: "Test"},
}

// ✅ CORRECT - With TypeMeta, handlers work properly
return &myappv0alpha1.MyResource{
    TypeMeta: metav1.TypeMeta{
        APIVersion: myappv0alpha1.GroupVersion.String(),
        Kind:       "MyResource",
    },
    ObjectMeta: metav1.ObjectMeta{Name: "test"},
    Spec: myappv0alpha1.MyResourceSpec{Title: "Test"},
}
```

**Why This Matters**: Resource handlers are looked up using `staticMetadata.Kind`. Without TypeMeta, this field is empty and handlers aren't called, resulting in null values for custom fields.

**Important**: No data migration required - GraphQL reuses your existing storage layer.

### Advanced Schema Composition

The federation system uses sophisticated schema composition with multiple strategies:

- **Cross-app Relationships**: Resources can reference each other across apps
- **Type Sharing**: Common types are unified across the federated schema
- **Query Optimization**: Intelligent query planning and batching

### Rate Limiting

Configure rate limiting for production deployments:

```go
gateway := registry.GetFederatedGateway()

// Configure rate limiting
rateLimitedGateway := gateway.WithRateLimit(gateway.RateLimitConfig{
    RequestsPerMinute: 100,
    BurstSize:        20,
    KeyExtractor:     gateway.IPBasedKeyExtractor, // or UserBasedKeyExtractor
})

// Use rate-limited gateway
http.HandleFunc("/graphql", rateLimitedGateway.HandleGraphQL)
```

Available rate limiting strategies:

- **IP-based**: Limit requests per IP address
- **User-based**: Limit requests per authenticated user
- **Custom**: Define your own key extraction logic

## Examples

### Manual Registration

If you prefer manual control:

```go
registry := gateway.NewAppProviderRegistry()

// Register each provider manually
registry.RegisterProvider("playlist", playlistProvider)
registry.RegisterProvider("investigations", investigationsProvider)

federatedGateway := registry.GetFederatedGateway()
```

### Introspection

Check what subgraphs are registered:

```go
subgraphs := registry.GetRegisteredSubgraphs()
for _, info := range subgraphs {
    fmt.Printf("App: %s/%s, Kinds: %v\n",
        info.GroupVersion.Group,
        info.GroupVersion.Version,
        info.Kinds)
}
```

## Migration Path

### Existing Apps

1. Apps without GraphQL support continue working unchanged
2. No breaking changes to existing App Platform APIs
3. GraphQL is purely additive functionality

### Adding GraphQL to Your App

1. Implement `GraphQLSubgraphProvider` interface (one method)
2. Create storage adapter to bridge to existing storage ⚠️ **Must set TypeMeta**
3. Register with federation gateway
4. GraphQL schema and resolvers auto-generated

### Migration from `/api/graphql` to `/apis/graphql`

The new federated system provides:

- ✅ **Same Queries**: Existing GraphQL queries work unchanged
- ✅ **Better Performance**: Automatic batching and caching
- ✅ **More Features**: Cross-app relationships and enhanced types
- ✅ **Auto-Discovery**: No manual registration required

Simply ensure your storage adapters set TypeMeta correctly.

### Common Issues

- **Custom fields return null**: Storage adapter not setting `TypeMeta` on resource objects
- **"Unknown field" errors**: Provider not implementing `GraphQLSubgraphProvider` correctly
- **Schema missing fields**: Resource handlers not registered properly

See [Adding Resources to GraphQL Federation](docs/adding-resources-to-federated-graphql.md) for detailed troubleshooting.

## Current Status (Phase 3 - Completed)

### ✅ Completed Features

- **App Platform Integration**: Full integration with Grafana App Platform
- **Auto-discovery**: Automatic detection of GraphQL-capable apps
- **Storage Bridge**: Seamless integration with existing REST storage
- **Enhanced Type Mapping**: Rich GraphQL types generated from CUE schemas
- **Cross-app Relationships**: Support for `@relation` attributes between apps
- **Schema Composition**: Advanced federated schema composition
- **Performance Optimization**: Query batching, caching, and complexity analysis
- **Rate Limiting**: Configurable rate limiting with multiple strategies
- **Production Integration**: Full integration with `/apis/graphql` endpoint

### 🚧 In Development

- **Security Features**: Enhanced authentication and authorization
- **Production Hardening**: Monitoring, error handling, and reliability

### 📋 Upcoming

- **Mesh Compose Integration**: Full Hive Gateway integration
- **Advanced Relationships**: Complex multi-app data relationships
- **Developer Tooling**: Enhanced debugging and introspection tools

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

## Roadmap

### Phase 2 ✅ (Completed)

- [x] App Platform integration
- [x] Auto-discovery of GraphQL apps
- [x] Storage bridge to existing REST APIs

### Phase 3 ✅ (Completed)

- [x] Enhanced CUE type mapping
- [x] Cross-app relationships with `@relation` attributes
- [x] Advanced schema composition
- [x] Performance optimization (batching, caching, complexity analysis)
- [x] Rate limiting and security features
- [x] Production integration with `/apis/graphql`

### Phase 4 🚧 (In Progress)

- [ ] Advanced security and monitoring features
- [ ] Enhanced developer tooling

## See Also

- [GraphQL Federation Design](../docs/graphql-federation-design.md) - Complete architecture
- [Implementation Plan](../docs/graphql-federation-implementation-plan.md) - Technical details
- [Status](../docs/graphql-federation-status.md) - Current progress

## Contributing

The federated GraphQL system is under active development. Contributions welcome for:

1. Enhanced CUE type mapping
2. Cross-app relationship patterns
3. Performance optimizations
4. Integration with more apps

See the implementation plan for detailed next steps.
