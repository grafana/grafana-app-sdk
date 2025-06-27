# GraphQL Federation for App Platform

This package provides federated GraphQL support for Grafana App Platform apps, allowing multiple apps to automatically contribute their schemas to a unified GraphQL API.

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

Each app's CUE kinds are automatically converted to GraphQL:

- **Types**: CUE structs → GraphQL objects
- **Queries**: `get`, `list` operations for each kind
- **Mutations**: `create`, `update`, `delete` operations
- **Metadata**: Standard Kubernetes ObjectMeta fields
- **Specs**: JSON scalars (enhanced type mapping coming in Phase 3)

### Storage Integration

GraphQL operations delegate to your existing REST storage:

```go
type MyAppStorageAdapter struct {
    legacyStorage rest.Storage
}

func (a *MyAppStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    // Delegate to existing REST storage
    getter := a.legacyStorage.(rest.Getter)
    return getter.Get(ctx, name, &metav1.GetOptions{})
}
```

No data migration required - GraphQL reuses your existing storage layer.

### Field Prefixing

To avoid naming conflicts, fields are prefixed by app domain:

- `playlist_playlist()` - from playlist app
- `dashboard_dashboard()` - from dashboard app
- `myapp_myresource()` - from your app

This will be enhanced with proper federation in Phase 3.

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
2. Create storage adapter to bridge to existing storage
3. Register with federation gateway
4. GraphQL schema and resolvers auto-generated

## Current Limitations (Phase 2)

- **Type Mapping**: Complex CUE types mapped as JSON scalars
- **Relationships**: No cross-app relationships yet
- **Field Prefixing**: Simple prefixing instead of proper federation
- **Performance**: Not yet optimized for production

These will be addressed in Phase 3.

## Roadmap

### Phase 2 ✅ (Current)

- [x] App Platform integration
- [x] Auto-discovery of GraphQL apps
- [x] Storage bridge to existing REST APIs

### Phase 3 (Next)

- [ ] Enhanced CUE type mapping
- [ ] Cross-app relationships with `@relation` attributes
- [ ] Mesh Compose + Hive Gateway integration
- [ ] Production performance optimization

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
