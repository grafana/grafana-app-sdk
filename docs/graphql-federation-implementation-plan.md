# GraphQL Federation Implementation Plan

## Implementation Phases

### Phase 1: Foundation (Week 1-2)

#### 1.1 App SDK GraphQL Package Structure

```tree
grafana-app-sdk/
├── graphql/
│   ├── gateway/          # Federated gateway implementation
│   │   ├── gateway.go
│   │   ├── composer.go
│   │   └── mesh.go       # Mesh Compose + Hive Gateway integration
│   ├── codegen/          # Schema generation from CUE
│   │   ├── generator.go
│   │   ├── types.go
│   │   └── resolvers.go
│   ├── subgraph/         # Subgraph interface and utilities
│   │   ├── subgraph.go
│   │   └── resolver.go
│   └── examples/         # Example implementations
└── cmd/grafana-app-sdk/
    └── graphql.go        # CLI command for GraphQL generation
```

#### 1.2 Core Interfaces

```go
// grafana-app-sdk/graphql/subgraph/subgraph.go
package subgraph

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "github.com/graphql-go/graphql"
)

type GraphQLSubgraph interface {
    GetSchema() *graphql.Schema
    GetResolvers() ResolverMap
    GetGroupVersion() schema.GroupVersion
}

type ResolverMap map[string]interface{}

type SubgraphConfig struct {
    GroupVersion schema.GroupVersion
    Kinds        []resource.Kind
    StorageGetter func(gvr schema.GroupVersionResource) Storage
}
```

#### 1.3 Schema Generator

```go
// grafana-app-sdk/graphql/codegen/generator.go
package codegen

type GraphQLGenerator struct {
    kinds []resource.Kind
    gv    schema.GroupVersion
}

func (g *GraphQLGenerator) GenerateSchema() (*graphql.Schema, error) {
    // Convert CUE kinds to GraphQL types
    // Generate standard CRUD operations
    // Create resolvers that delegate to storage layer
}

func (g *GraphQLGenerator) GenerateResolvers(storage StorageProvider) ResolverMap {
    // Generate standard resolvers for each kind
    // Handle get, list, create, update, delete operations
}
```

### Phase 2: Basic Gateway (Week 3-4)

#### 2.1 Federated Gateway Implementation

```go
// grafana-app-sdk/graphql/gateway/gateway.go
package gateway

import (
    "github.com/graphql-go/graphql"
    "github.com/grafana/grafana-app-sdk/graphql/subgraph"
)

type FederatedGateway struct {
    subgraphs   map[string]subgraph.GraphQLSubgraph
    meshClient  *MeshComposeClient
    hiveClient  *HiveGatewayClient
    schema      *graphql.Schema
}

func NewFederatedGateway() *FederatedGateway

func (g *FederatedGateway) RegisterSubgraph(gv schema.GroupVersion, sg subgraph.GraphQLSubgraph) error

func (g *FederatedGateway) ComposeSchema() (*graphql.Schema, error) {
    // Use Mesh Compose to merge schemas
    // Use Hive Gateway for query planning
}

func (g *FederatedGateway) HandleGraphQL(w http.ResponseWriter, r *http.Request)
```

#### 2.2 Mesh Compose + Hive Gateway Integration

```go
// grafana-app-sdk/graphql/gateway/mesh.go
package gateway

// Integration with external tools for schema composition
type MeshComposeClient struct {
    // Configuration for Mesh Compose
}

type HiveGatewayClient struct {
    // Configuration for Hive Gateway
}

func (m *MeshComposeClient) ComposeSchemas(subgraphs []SubgraphSchema) (*ComposedSchema, error)
func (h *HiveGatewayClient) ExecuteQuery(schema *ComposedSchema, query string) (*QueryResult, error)
```

### Phase 3: Integration with App Platform (Week 5)

#### 3.1 Extend App Provider Interface

```go
// In grafana-app-sdk
type GraphQLProvider interface {
    GetGraphQLSubgraph() subgraph.GraphQLSubgraph
}

// Apps can implement this interface to provide GraphQL subgraphs
type AppProvider interface {
    // Existing methods...
    app.Provider

    // New optional method for GraphQL
    GetGraphQLSubgraph() subgraph.GraphQLSubgraph
}
```

#### 3.2 Auto-Generation Integration

```go
// In app provider implementations
type PlaylistAppProvider struct {
    // existing fields...
    graphqlSubgraph subgraph.GraphQLSubgraph
}

func (p *PlaylistAppProvider) GetGraphQLSubgraph() subgraph.GraphQLSubgraph {
    if p.graphqlSubgraph == nil {
        // Auto-generate from CUE kinds
        generator := codegen.NewGraphQLGenerator(p.GetKinds(), p.GetGroupVersion())
        schema, _ := generator.GenerateSchema()
        resolvers := generator.GenerateResolvers(p.storageProvider)

        p.graphqlSubgraph = subgraph.New(subgraph.Config{
            Schema:       schema,
            Resolvers:    resolvers,
            GroupVersion: p.GetGroupVersion(),
        })
    }
    return p.graphqlSubgraph
}
```

#### 3.3 Update Grafana Core Integration

```go
// In grafana/pkg/registry/apps/apps.go
func ProvideRegistryServiceSink(...) (*Service, error) {
    // Existing setup...
    providers := []app.Provider{playlistAppProvider, dashboardAppProvider}

    // GraphQL federation setup
    gateway := gateway.NewFederatedGateway()

    for _, provider := range providers {
        if gqlProvider, ok := provider.(GraphQLProvider); ok {
            subgraph := gqlProvider.GetGraphQLSubgraph()
            err := gateway.RegisterSubgraph(provider.GroupVersion(), subgraph)
            if err != nil {
                return nil, err
            }
        }
    }

    // Compose unified schema
    composedSchema, err := gateway.ComposeSchema()
    if err != nil {
        return nil, err
    }

    // Register GraphQL endpoint
    rr.Post("/graphql", gateway.HandleGraphQL)

    return &Service{runner: apiGroupRunner, gateway: gateway, log: logger}, nil
}
```

### ✅ Phase 4: Real App Integration (Completed)

#### 4.1 Playlist App Implementation

- ✅ Playlist app implements `GraphQLSubgraphProvider` interface
- ✅ GraphQL schema auto-generated from CUE kinds
- ✅ Storage adapter bridges GraphQL to existing REST storage
- ✅ Zero breaking changes to existing functionality

#### 4.2 Auto-Discovery System

- ✅ `AppProviderRegistry` automatically finds GraphQL-capable providers
- ✅ Gateway composes schemas from discovered subgraphs
- ✅ HTTP GraphQL endpoint handles unified queries
- ✅ Field prefixing prevents conflicts between apps

#### 4.3 Working Examples

```graphql
# Query playlists from unified GraphQL endpoint
query {
  playlist_playlists(namespace: "default") {
    metadata {
      name
      namespace
    }
    spec {
      title
      description
    }
  }
}

# Query specific playlist
query {
  playlist_playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
      creationTimestamp
    }
    spec {
      title
      items
    }
  }
}
```

## Technical Specifications

### Dependencies

#### App SDK Dependencies

```go
// Add to grafana-app-sdk/go.mod
require (
    github.com/graphql-go/graphql v0.8.0
    github.com/graphql-go/handler v0.2.3
    // No external federation tools - native Go implementation
)
```

#### Grafana Core Dependencies

```go
// No new dependencies needed - reuses existing App Platform infrastructure
// GraphQL endpoints integrate with existing HTTP routing
```

### CLI Command Integration

```bash
# Generate GraphQL schema from CUE
grafana-app-sdk generate graphql --input ./kinds --output ./graphql

# Validate GraphQL schema
grafana-app-sdk validate graphql --schema ./graphql/schema.graphql

# Preview federated schema
grafana-app-sdk compose graphql --subgraphs ./apps/*/graphql
```

### Configuration

#### App-level Configuration

```yaml
# In app manifest
apiVersion: apps.grafana.com/v1alpha1
kind: AppManifest
spec:
  graphql:
    enabled: true
    customResolvers: []
    relationships:
      - kind: "Dashboard"
        field: "spec.folderUID"
        target: "Folder"
        targetField: "metadata.uid"
```

#### Gateway Configuration

```yaml
# In Grafana configuration
[app_platform]
graphql_federation_enabled = true
graphql_endpoint = "/graphql"
# Auto-discovery enabled by default
# No external configuration files needed
```

## Testing Strategy

### Unit Tests

- Schema generation from CUE kinds
- Resolver generation and execution
- Gateway subgraph registration and composition

### Integration Tests

- End-to-end GraphQL queries across multiple apps
- Authentication and authorization with Grafana context
- Error handling and validation

### Performance Tests

- Schema composition performance with multiple subgraphs
- Query execution performance vs REST API equivalents
- Memory usage and garbage collection impact

## Success Criteria

### ✅ Phase 1 Success (Completed)

- [x] Basic schema generation from CUE works
- [x] Simple resolvers delegate to existing storage
- [x] Gateway can compose schemas from multiple subgraphs

### ✅ Phase 2 Success (Completed)

- [x] Single app (playlist) provides working GraphQL API
- [x] Basic CRUD operations work via GraphQL
- [x] Integration with existing App Platform patterns
- [x] Auto-discovery system finds GraphQL-capable apps
- [x] Storage bridge adapts existing REST storage

### 🚧 Phase 3 Success (Next Goals)

- [ ] Relationship support with `@relation` attributes in CUE
- [ ] Enhanced type mapping beyond JSON scalars
- [ ] Performance optimization (caching, batching)
- [ ] Field-level permissions and security features
- [ ] Multiple apps with cross-app relationships

### ✅ Overall Success (Achieved Core Goals)

- [x] GraphQL API provides CRUD functionality equivalent to REST APIs
- [x] App developers can add GraphQL with one interface method
- [x] Performance is acceptable (delegates to existing optimized storage)
- [x] Documentation and working examples are complete
- [x] Zero breaking changes to existing App Platform patterns

## Risk Mitigation

### Technical Risks (Addressed)

- **Schema Composition Complexity**: ✅ Solved with native Go implementation and field prefixing
- **Performance Concerns**: ✅ Mitigated through storage delegation to existing optimized layers
- **External Dependencies**: ✅ Avoided by choosing native implementation over external tools
- **CUE Integration**: ✅ Successfully generates GraphQL schemas from existing CUE kinds

### Process Risks (Managed)

- **Scope Creep**: ✅ Focused on basic CRUD first, achieved working implementation
- **Integration Complexity**: ✅ Leveraged existing App Platform patterns successfully
- **Breaking Changes**: ✅ Maintained complete backward compatibility with REST APIs
- **Operational Complexity**: ✅ Avoided by rejecting Node.js/external runtime approaches

This implementation plan provides a structured approach to building the federated GraphQL architecture while preserving the existing centralized GraphQL POC and maintaining compatibility with current App Platform patterns.
