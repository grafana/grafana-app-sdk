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

### Phase 4: Proof of Concept (Week 6)

#### 4.1 Single App POC

- Choose playlist app as first implementation
- Generate GraphQL schema from existing CUE kinds
- Implement basic CRUD resolvers
- Test federated gateway with single subgraph

#### 4.2 Multi-App Integration

- Add dashboard app subgraph
- Test schema composition with multiple subgraphs
- Verify query routing works across apps

#### 4.3 Integration Testing

```graphql
# Test basic queries
query {
  playlists(namespace: "default") {
    metadata {
      name
    }
    spec {
      title
    }
  }
}

# Test cross-app queries (if relationships exist)
query {
  playlist(namespace: "default", name: "test") {
    spec {
      items {
        # If playlist items reference dashboards
        dashboard {
          spec {
            title
          }
        }
      }
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
    // Mesh Compose + Hive Gateway clients (versions TBD)
)
```

#### Grafana Core Dependencies

```go
// No new dependencies needed - reuses existing App Platform infrastructure
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
mesh_compose_config = "./config/mesh-compose.yaml"
hive_gateway_config = "./config/hive-gateway.yaml"
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

### Phase 1 Success

- [ ] Basic schema generation from CUE works
- [ ] Simple resolvers delegate to existing storage
- [ ] Gateway can compose schemas from multiple subgraphs

### Phase 2 Success

- [ ] Single app (playlist) provides working GraphQL API
- [ ] Basic CRUD operations work via GraphQL
- [ ] Integration with existing App Platform patterns

### Phase 3 Success

- [ ] Multiple apps contribute subgraphs
- [ ] Federated schema composition works
- [ ] Query routing across subgraphs functions properly
- [ ] Authentication/authorization preserved

### Overall Success

- [ ] GraphQL API provides equivalent functionality to REST APIs
- [ ] App developers can add GraphQL with minimal effort
- [ ] Performance is acceptable compared to REST
- [ ] Documentation and examples are complete

## Risk Mitigation

### Technical Risks

- **Schema Composition Complexity**: Start with simple schemas, add complexity incrementally
- **Performance Concerns**: Benchmark early and often, optimize query planning
- **Mesh/Hive Integration**: Have fallback to simpler schema stitching if needed

### Process Risks

- **Scope Creep**: Focus on basic CRUD first, relationships and custom resolvers later
- **Integration Complexity**: Leverage existing App Platform patterns as much as possible
- **Breaking Changes**: Maintain backward compatibility with REST APIs

This implementation plan provides a structured approach to building the federated GraphQL architecture while preserving the existing centralized GraphQL POC and maintaining compatibility with current App Platform patterns.
