# GraphQL Federation Architecture for Grafana App Platform

## Overview

This document outlines the design for a federated GraphQL API architecture that leverages the Grafana App Platform's existing patterns. Instead of a centralized GraphQL service, each App Platform app contributes its own GraphQL subgraph, which are then composed into a unified API.

## Background: App Platform & CUE Concepts

### What is the App Platform?

The Grafana App Platform allows developers to create applications that extend Grafana with custom resource types (called "kinds"). These apps follow Kubernetes-style APIs and can be deployed alongside Grafana core.

### CUE (Configure, Unify, Execute)

CUE is a data validation and configuration language used in the App Platform to define resource schemas:

```cue
// Example: Playlist kind definition
#PlaylistKind: {
    kind: "Playlist"
    apiVersion: "playlist.grafana.app/v0alpha1"

    spec: #PlaylistSpec
}

#PlaylistSpec: {
    title: string
    description?: string
    items: [...#PlaylistItem]
    interval: string | *"5m"
}

#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string
    title?: string
}
```

**Key CUE Concepts:**

- **Schemas**: Define the structure and validation rules for data
- **Types**: Reusable schema definitions (prefixed with `#`)
- **Constraints**: Built-in validation (required fields, types, defaults)
- **Composition**: Schemas can embed and extend other schemas

### Current App Platform Flow

```mermaid
graph LR
    CUE["CUE Kind Definition"] --> CODEGEN["App SDK Codegen"]
    CODEGEN --> GO["Go Types & REST APIs"]
    GO --> REGISTER["Register with Grafana"]
    REGISTER --> API["/apis/{group}/{version}/{resource}"]
```

## Federated GraphQL Architecture

### Core Concept

Instead of one centralized GraphQL service, each app provides its own GraphQL subgraph. The App SDK gateway composes these subgraphs into a unified API.

```mermaid
graph TB
    subgraph "GraphQL Gateway (App SDK)"
        GATEWAY["Federated Gateway"]
        COMPOSER["Schema Composer"]
    end

    subgraph "App Subgraphs"
        PLAYLIST["playlist.grafana.app<br/>- Playlist<br/>- PlaylistItem"]
        DASHBOARD["dashboard.grafana.app<br/>- Dashboard<br/>- DashboardSummary"]
        CUSTOM["myapp.company.com<br/>- CustomResource"]
    end

    CLIENT["GraphQL Client"] --> GATEWAY
    GATEWAY --> COMPOSER
    COMPOSER --> PLAYLIST
    COMPOSER --> DASHBOARD
    COMPOSER --> CUSTOM
```

### Schema Generation Strategy

#### Automatic Generation from CUE

The App SDK will automatically generate GraphQL schemas from existing CUE kind definitions:

**CUE Definition:**

```cue
#PlaylistKind: {
    kind: "Playlist"
    spec: {
        title: string
        description?: string
        items: [...#PlaylistItem]
    }
}
```

**Generated GraphQL Schema:**

```graphql
type Playlist {
  apiVersion: String!
  kind: String!
  metadata: ObjectMeta!
  spec: PlaylistSpec!
}

type PlaylistSpec {
  title: String!
  description: String
  items: [PlaylistItem!]!
}

type Query {
  playlist(namespace: String!, name: String!): Playlist
  playlists(namespace: String!): [Playlist!]!
}

type Mutation {
  createPlaylist(namespace: String!, input: PlaylistInput!): Playlist
  updatePlaylist(
    namespace: String!
    name: String!
    input: PlaylistInput!
  ): Playlist
  deletePlaylist(namespace: String!, name: String!): Boolean
}
```

#### Standard Patterns

Every CUE kind automatically gets:

1. **Object Types**: Generated from CUE spec structures
2. **Query Operations**:
   - `get{Kind}(namespace, name)` - Retrieve single resource
   - `list{Kind}s(namespace)` - List resources in namespace
3. **Mutation Operations**:
   - `create{Kind}(namespace, input)` - Create new resource
   - `update{Kind}(namespace, name, input)` - Update existing resource
   - `delete{Kind}(namespace, name)` - Delete resource
4. **Standard Fields**: All resources include `apiVersion`, `kind`, `metadata`

#### Type Mapping Rules

| CUE Type        | GraphQL Type       | Notes           |
| --------------- | ------------------ | --------------- |
| `string`        | `String`           |                 |
| `int`           | `Int`              |                 |
| `bool`          | `Boolean`          |                 |
| `[...T]`        | `[T]`              | Arrays          |
| `T?`            | `T` (nullable)     | Optional fields |
| `T \| *default` | `T` (with default) | Default values  |
| `#EmbeddedType` | `EmbeddedType`     | Type references |

### Relationship Handling

#### Defining Relationships in CUE

Relationships between kinds can be expressed through references:

```cue
#DashboardKind: {
    spec: {
        title: string
        // Reference to folder
        folderUID?: string @relation(kind: "Folder", field: "metadata.uid")
    }
}

#FolderKind: {
    spec: {
        title: string
    }
}
```

#### Generated Relationship Resolvers

The system will automatically generate resolvers for relationships:

```graphql
type Dashboard {
  spec: DashboardSpec!
}

type DashboardSpec {
  title: String!
  folderUID: String
  # Auto-generated relationship resolver
  folder: Folder @relation(field: "folderUID")
}

type Query {
  dashboard(namespace: String!, name: String!): Dashboard
  # Automatically supports relationship traversal:
  # query { dashboard { spec { folder { spec { title } } } } }
}
```

## Gateway Implementation

### Architecture Components

#### 1. Subgraph Registration

```go
// In App SDK
type GraphQLSubgraph interface {
    GetSchema() *graphql.Schema
    GetResolvers() map[string]interface{}
    GetGroupVersion() schema.GroupVersion
}

type AppProvider interface {
    // Existing methods...
    GetGraphQLSubgraph() GraphQLSubgraph
}
```

#### 2. Schema Composition (Runtime)

```go
type FederatedGateway struct {
    subgraphs   map[string]GraphQLSubgraph
    meshCompose *MeshComposeClient
    hiveGateway *HiveGatewayClient
}

func (g *FederatedGateway) ComposeSchema() (*graphql.Schema, error) {
    // Use Mesh Compose to merge subgraph schemas
    // Use Hive Gateway for query planning and execution
}
```

#### 3. Integration with Existing App Platform

The gateway extends the existing app registration pattern:

```go
// In pkg/registry/apps/apps.go
func ProvideRegistryServiceSink(...) (*Service, error) {
    // Existing REST API setup...
    providers := []app.Provider{playlistAppProvider, dashboardAppProvider}

    // New GraphQL federation
    gateway := graphql.NewFederatedGateway()
    for _, provider := range providers {
        if gqlProvider, ok := provider.(GraphQLProvider); ok {
            subgraph := gqlProvider.GetGraphQLSubgraph()
            gateway.RegisterSubgraph(provider.GroupVersion(), subgraph)
        }
    }

    // Compose unified schema using Mesh Compose + Hive Gateway
    schema, err := gateway.ComposeSchema()

    // Register single federated GraphQL endpoint
    registrar.RegisterRoute("POST", "/graphql", gateway.HandleGraphQL)
}
```

### Mesh Compose + Hive Gateway Integration

Instead of Apollo Federation, we'll use:

- **Mesh Compose**: For schema composition and merging
- **Hive Gateway**: For query planning, execution, and optimization

This approach provides:

- Better performance than Apollo Federation
- More flexible schema composition
- Built-in query optimization
- No need for federation-specific SDL extensions

## Implementation Phases

### Phase 1: Basic Auto-Generation

- Generate GraphQL schemas from CUE kinds
- Implement standard CRUD resolvers
- Basic gateway with schema composition
- Single app proof-of-concept

### Phase 2: Relationship Support

- Define relationship syntax in CUE
- Auto-generate relationship resolvers
- Cross-subgraph query support
- Multi-app integration

### Phase 3: Advanced Features (Future)

- Custom resolver extension points
- Real-time subscriptions
- Query optimization
- Advanced filtering and pagination

## Migration Strategy

### For App Developers

**Existing Apps (using "new way" pattern):**

1. Apps already using `AppProvider` get GraphQL automatically
2. No code changes required for basic CRUD
3. Opt-in to relationship definitions in CUE

**Custom Requirements:**

1. Apps can implement custom resolvers as needed
2. Extension points for complex business logic
3. Backward compatibility with existing REST APIs

### For Grafana Core

1. Create new branch without centralized GraphQL
2. Implement federated gateway alongside existing `/apis` endpoints
3. GraphQL becomes an additional interface to existing data
4. No changes to underlying storage or business logic

## Benefits

### For App Developers

- **Zero GraphQL Knowledge Required**: Automatic generation from familiar CUE
- **Consistent Patterns**: Follows existing App Platform conventions
- **Relationship Support**: Express and query data relationships naturally
- **Flexibility**: Custom resolvers when needed

### For API Consumers

- **Single Endpoint**: One GraphQL endpoint for all app data
- **Efficient Queries**: Request exactly the data needed
- **Type Safety**: Generated types and schema validation
- **Relationship Traversal**: Query related data in single request

### For Platform

- **Decentralized**: Each app owns its GraphQL schema
- **Scalable**: Apps can be developed and deployed independently
- **Maintainable**: Auto-generation reduces manual schema maintenance
- **Extensible**: Easy to add new apps and capabilities

## Example Usage

```graphql
# Query playlist with related dashboard information
query GetPlaylistWithDashboards($namespace: String!, $name: String!) {
  playlist(namespace: $namespace, name: $name) {
    metadata {
      name
      namespace
      creationTimestamp
    }
    spec {
      title
      description
      items {
        type
        value
        title
        # If items reference dashboards, auto-resolved relationship
        dashboard {
          spec {
            title
            description
          }
        }
      }
    }
  }
}
```

This architecture provides a powerful, flexible GraphQL API that grows naturally with the App Platform ecosystem while maintaining the simplicity and conventions that developers already know.
