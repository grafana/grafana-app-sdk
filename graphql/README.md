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
    dashboardProvider,
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
  playlist_playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
      namespace
    }
    spec
  }

  # Query dashboards
  dashboard_dashboard(namespace: "default", name: "my-dashboard") {
    metadata {
      name
      namespace
    }
    spec
  }

  # Query your app's resources
  myapp_myresource(namespace: "default", name: "my-resource") {
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

**Important**: Storage adapters must set proper `TypeMeta` on resource objects, or custom GraphQL fields will return null values. No data migration required - GraphQL reuses your existing storage layer.

### Advanced Schema Composition

The federation system uses sophisticated schema composition with multiple strategies:

- **Field Prefixing**: Domain-based prefixing to avoid conflicts

  - `playlist_playlist()` - from playlist app
  - `dashboard_dashboard()` - from dashboard app
  - `myapp_myresource()` - from your app

- **Cross-app Relationships**: Resources can reference each other across apps
- **Type Sharing**: Common types are unified across the federated schema
- **Query Optimization**: Intelligent query planning and batching

## Examples

### Manual Registration

If you prefer manual control:

```go
registry := gateway.NewAppProviderRegistry()

// Register each provider manually
registry.RegisterProvider("playlist", playlistProvider)
registry.RegisterProvider("dashboard", dashboardProvider)

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

### Common Issues

- **Custom fields return null**: Storage adapter not setting `TypeMeta` on resource objects
- **"Unknown field" errors**: Provider not implementing `GraphQLSubgraphProvider` correctly
- **Schema missing fields**: Resource handlers not registered properly

See [Adding Resources to GraphQL Federation](docs/adding-resources-to-federated-graphql.md) for detailed troubleshooting.

## Current Status (Phase 3 - In Progress)

### ✅ Completed Features

- **App Platform Integration**: Full integration with Grafana App Platform
- **Auto-discovery**: Automatic detection of GraphQL-capable apps
- **Storage Bridge**: Seamless integration with existing REST storage
- **Enhanced Type Mapping**: Improved CUE to GraphQL type conversion
- **Cross-app Relationships**: Support for `@relation` attributes between apps
- **Schema Composition**: Advanced federated schema composition

### 🚧 In Development

- **Performance Optimization**: Query batching and caching improvements
- **Security Features**: Enhanced authentication and authorization
- **Production Hardening**: Monitoring, error handling, and reliability

### 📋 Upcoming

- **Mesh Compose Integration**: Full Hive Gateway integration
- **Advanced Relationships**: Complex multi-app data relationships
- **Developer Tooling**: Enhanced debugging and introspection tools

## Roadmap

### Phase 2 ✅ (Completed)

- [x] App Platform integration
- [x] Auto-discovery of GraphQL apps
- [x] Storage bridge to existing REST APIs

### Phase 3 🚧 (In Progress)

- [x] Enhanced CUE type mapping
- [x] Cross-app relationships with `@relation` attributes
- [x] Advanced schema composition
- [ ] Mesh Compose + Hive Gateway integration
- [ ] Production performance optimization
- [ ] Security and monitoring features

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
