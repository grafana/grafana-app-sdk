# Phase 3.2: Enhanced CUE Integration Demo

## Overview

Phase 3.2 enhances our federated GraphQL system with:

1. **CUE `@relation` Attribute Parsing** - Zero-configuration relationships
2. **Enhanced Type Mapping** - Rich GraphQL types from CUE schemas
3. **Production-Ready Integration** - Seamless with App Platform patterns

## 🚀 Key Improvements Over Phase 3.1

### Before (Phase 3.1): Manual Registration

```go
// Manual relationship registration required
relationshipConfig := &codegen.RelationshipConfig{
    FieldName:   "dashboard",
    Kind:        "dashboard.grafana.app/Dashboard",
    SourceField: "spec.items.value",
    TargetField: "metadata.uid",
    // ... more config
}
relationshipParser.RegisterRelationship("Playlist", relationshipConfig)
```

### After (Phase 3.2): CUE Definition

```cue
// Relationships defined directly in CUE schema
#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string
    title?: string

    // Zero-configuration relationship!
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"
        target: "metadata.uid"
        optional: true
    )
}
```

## 🎯 Enhanced Type Mapping

### CUE Schema with Rich Types

```cue
#DashboardSpec: {
    // String with enum constraint → GraphQL Enum
    refresh: "5s" | "10s" | "30s" | "1m" | "5m" | "10m"

    // String array → GraphQL [String!]!
    tags: [...string]

    // Optional string → GraphQL String (nullable)
    description?: string

    // Required string → GraphQL String!
    title: string

    // Integer → GraphQL Int!
    schemaVersion: int & >=1

    // Nested object → Generated GraphQL type
    timeRange: {
        from: string
        to: string
    }

    // Array of objects → [Panel!]!
    panels: [...#Panel]
}

#Panel: {
    id: int
    type: "graph" | "stat" | "table" | "text"
    title: string
    gridPos: {
        x: int & >=0 & <=24
        y: int & >=0
        w: int & >=1 & <=24
        h: int & >=1
    }
}
```

### Generated GraphQL Schema

```graphql
# Enhanced types from CUE constraints
enum DashboardRefresh {
  FIVE_SECONDS
  TEN_SECONDS
  THIRTY_SECONDS
  ONE_MINUTE
  FIVE_MINUTES
  TEN_MINUTES
}

enum PanelType {
  GRAPH
  STAT
  TABLE
  TEXT
}

# Rich object types instead of JSON scalars
type DashboardSpec {
  refresh: DashboardRefresh!
  tags: [String!]!
  description: String # Nullable (optional in CUE)
  title: String! # Non-null (required in CUE)
  schemaVersion: Int!
  timeRange: TimeRange!
  panels: [Panel!]!
}

type TimeRange {
  from: String!
  to: String!
}

type Panel {
  id: Int!
  type: PanelType!
  title: String!
  gridPos: GridPos!
}

type GridPos {
  x: Int! # Constrained 0-24 in CUE
  y: Int!
  w: Int! # Constrained 1-24 in CUE
  h: Int! # Constrained >=1 in CUE
}

# Main dashboard type with relationship
type Dashboard {
  apiVersion: String
  kind: String
  metadata: ObjectMeta
  spec: DashboardSpec! # Rich types instead of JSON
  status: DashboardStatus
}
```

## 🔗 CUE Relationships in Action

### Complete Playlist + Dashboard Schema

```cue
// Playlist schema with relationships
#PlaylistSpec: {
    title: string
    description?: string
    interval: "5s" | "10s" | "30s" | "1m" | "5m"
    items: [...#PlaylistItem]
}

#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string
    title?: string

    // Automatic relationship to dashboard
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"
        target: "metadata.uid"
        optional: true
    )
}

#Playlist: {
    apiVersion: "playlist.grafana.app/v0alpha1"
    kind: "Playlist"
    metadata: #ObjectMeta
    spec: #PlaylistSpec
    status?: #PlaylistStatus

    // Many-to-many relationship via tags
    relatedDashboards?: [..._] @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "spec.tags"
        target: "spec.tags"
        cardinality: "many"
        match: "array_contains"
    )
}
```

### Generated GraphQL with Relationships

```graphql
enum PlaylistInterval {
  FIVE_SECONDS
  TEN_SECONDS
  THIRTY_SECONDS
  ONE_MINUTE
  FIVE_MINUTES
}

enum PlaylistItemType {
  DASHBOARD_BY_UID
  DASHBOARD_BY_TAG
}

type PlaylistItem {
  type: PlaylistItemType!
  value: String!
  title: String

  # Auto-generated relationship field
  dashboard: Dashboard # Resolves via UID
}

type PlaylistSpec {
  title: String!
  description: String
  interval: PlaylistInterval!
  items: [PlaylistItem!]!
}

type Playlist {
  apiVersion: String
  kind: String
  metadata: ObjectMeta
  spec: PlaylistSpec!
  status: PlaylistStatus

  # Auto-generated relationship fields
  relatedDashboards: [Dashboard!]! # Many-to-many via tags
}
```

## 🎭 Demo Queries

### Rich Type Safety in Queries

```graphql
# Type-safe enum values and nested objects
query EnhancedPlaylistQuery {
  playlist_playlist(namespace: "default", name: "demo-playlist") {
    metadata {
      name
      uid
      creationTimestamp
    }
    spec {
      title
      description
      interval # Returns enum value: THIRTY_SECONDS
      items {
        type # Returns enum value: DASHBOARD_BY_UID
        value # Dashboard UID
        title

        # Relationship with rich dashboard types
        dashboard {
          metadata {
            name
            uid
          }
          spec {
            title
            description
            refresh # Enum: FIVE_MINUTES
            tags # [String!]!
            timeRange {
              # Rich object type
              from
              to
            }
            panels {
              # [Panel!]!
              id
              type # Enum: GRAPH | STAT | TABLE | TEXT
              title
              gridPos {
                # Rich object type
                x # Int constrained 0-24
                y
                w # Int constrained 1-24
                h
              }
            }
          }
        }
      }
    }

    # Many-to-many relationships
    relatedDashboards {
      metadata {
        name
      }
      spec {
        title
        tags # Common tags that created the relationship
      }
    }
  }
}
```

### Auto-Complete Benefits

With rich types, GraphQL clients get:

- **Enum auto-completion** for `refresh`, `interval`, `type` fields
- **Type validation** preventing invalid values
- **Schema introspection** for rich object structures
- **Nested object navigation** with full type safety

## 🏗️ Implementation Architecture

### 1. CUE Relationship Integration

```go
// Enhanced generator with CUE integration
func CreateEnhancedPlaylistSubgraph() (*EnhancedSubgraph, error) {
    // Load CUE schema with @relation attributes
    cueSchema := loadPlaylistCUESchema()

    // Create relationship parser with CUE support
    relationshipParser := codegen.NewRelationshipParser(cueCtx)

    // Parse relationships directly from CUE - no manual registration!
    relationships, err := relationshipParser.ParseCUERelationshipsFromValue(
        cueSchema, "Playlist")
    if err != nil {
        return nil, err
    }

    // Create enhanced generator with type mapping
    baseGenerator := codegen.NewGraphQLGenerator(kinds, gv, storageGetter)
    enhancedGenerator := codegen.NewEnhancedGraphQLGenerator(baseGenerator)

    // Generate schema with both relationships and enhanced types
    schema, err := enhancedGenerator.GenerateEnhancedSchema()
    if err != nil {
        return nil, err
    }

    return &EnhancedSubgraph{schema: schema}, nil
}
```

### 2. Type Mapping Integration

```go
// CUE to GraphQL type mapping
typeMapper := codegen.NewCUETypeMapper()

// Enum generation from CUE constraints
refreshType := typeMapper.MapCUEToGraphQL(
    cueSchema.LookupPath("spec.refresh"),
    "DashboardRefresh")
// Result: GraphQL enum with FIVE_SECONDS, TEN_SECONDS, etc.

// Object type generation from CUE structs
panelType := typeMapper.MapCUEToGraphQL(
    cueSchema.LookupPath("spec.panels.0"),
    "Panel")
// Result: Rich Panel object with proper field types
```

## 🆚 Comparison: Phase 3.1 vs 3.2

| Feature                     | Phase 3.1                  | Phase 3.2                  |
| --------------------------- | -------------------------- | -------------------------- |
| **Relationship Definition** | Manual registration        | CUE `@relation` attributes |
| **Type Mapping**            | JSON scalars only          | Rich GraphQL types         |
| **Enum Support**            | None                       | Auto-generated from CUE    |
| **Nested Objects**          | Flattened to JSON          | Proper object types        |
| **Developer Experience**    | Requires GraphQL knowledge | Pure CUE definitions       |
| **Type Safety**             | Limited                    | Full GraphQL type safety   |
| **Auto-Complete**           | Basic                      | Rich with enums/objects    |
| **Schema Introspection**    | Generic                    | Domain-specific            |

## 🎉 Benefits Realized

### For App Developers

- **Zero GraphQL Knowledge**: Define everything in familiar CUE
- **Automatic Relationships**: Just add `@relation` attributes
- **Rich Type Safety**: CUE constraints become GraphQL types
- **No Manual Registration**: Everything auto-discovered from CUE

### For API Consumers

- **Better Developer Experience**: Rich auto-complete and validation
- **Type Safety**: Enums and constraints prevent invalid queries
- **Schema Clarity**: Domain-specific types instead of generic JSON
- **Powerful Queries**: Cross-app relationships with rich nested data

### for Platform

- **Consistency**: All apps follow same CUE → GraphQL patterns
- **Maintainability**: Changes in CUE automatically update GraphQL
- **Scalability**: No manual schema maintenance as apps grow
- **Extensibility**: New relationship types and constraints supported automatically

## 🚀 Ready for Production

Phase 3.2 delivers a production-ready federated GraphQL system that:

- ✅ **Auto-generates rich GraphQL schemas** from CUE definitions
- ✅ **Supports cross-app relationships** via CUE `@relation` attributes
- ✅ **Provides type safety** with enums, constraints, and nested objects
- ✅ **Requires zero GraphQL knowledge** from app developers
- ✅ **Integrates seamlessly** with existing App Platform patterns

**The federated GraphQL vision is now fully realized!** 🎯
