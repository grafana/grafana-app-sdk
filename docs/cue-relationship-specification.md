# CUE Relationship Specification

## Overview

This specification defines how App Platform apps can express relationships between kinds using CUE attributes. Relationships enable cross-app GraphQL queries, allowing data from multiple apps to be fetched in a single request.

## Relationship Attribute Syntax

### Basic `@relation` Attribute

```cue
fieldName?: TargetType @relation(
    kind: "group/Kind"           // Target kind (required)
    field: "localField"          // Local field containing reference value (required)
    target: "targetField"        // Target field to match against (optional, defaults to "metadata.name")
    optional: bool               // Whether relationship is optional (optional, defaults to true)
    cardinality: "one" | "many"  // Relationship cardinality (optional, defaults to "one")
)
```

### Examples

#### One-to-One Relationship: Playlist Item → Dashboard

```cue
#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string  // Contains dashboard UID
    title?: string

    // Relationship: this item references a dashboard
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"              // Local field containing UID
        target: "metadata.uid"      // Match against dashboard's UID
        optional: true
    )
}
```

#### One-to-Many Relationship: Folder → Dashboards

```cue
#FolderSpec: {
    title: string

    // Relationship: folder contains many dashboards
    dashboards?: [..._] @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "metadata.uid"       // Folder's UID
        target: "spec.folderUID"    // Dashboards reference folder UID
        cardinality: "many"
    )
}
```

#### Many-to-Many Relationship: Dashboard → Teams (via tags)

```cue
#DashboardSpec: {
    title: string
    tags: [...string]

    // Relationship: dashboards can be associated with teams via tags
    teams?: [..._] @relation(
        kind: "iam.grafana.app/Team"
        field: "tags"               // Dashboard tags (array)
        target: "spec.tag"          // Team tag (single value)
        cardinality: "many"
        match: "array_contains"     // Special matching logic for arrays
    )
}
```

## Attribute Parameters

### Required Parameters

#### `kind: string`

The fully qualified kind name of the target resource.

- Format: `"group/Kind"` or `"group.domain/Kind"`
- Example: `"dashboard.grafana.app/Dashboard"`

#### `field: string`

The local field path containing the reference value(s).

- Uses dot notation for nested fields: `"spec.folderUID"`
- For arrays: `"tags"` or `"spec.items[].value"`

### Optional Parameters

#### `target: string` (default: `"metadata.name"`)

The target field path to match against.

- Uses dot notation for nested fields
- Common targets: `"metadata.name"`, `"metadata.uid"`, `"spec.id"`

#### `optional: bool` (default: `true`)

Whether the relationship is optional.

- `true`: Relationship field can be null, queries succeed if target not found
- `false`: Relationship must resolve, queries fail if target not found

#### `cardinality: "one" | "many"` (default: `"one"`)

The relationship cardinality.

- `"one"`: Single target resource (generates nullable field)
- `"many"`: Multiple target resources (generates array field)

#### `match: string` (default: `"exact"`)

The matching strategy for complex relationships.

- `"exact"`: Exact value match (default)
- `"array_contains"`: Local array contains target value
- `"target_contains"`: Target array contains local value

## Generated GraphQL Schema

### One-to-One Relationship

**CUE:**

```cue
#PlaylistItem: {
    value: string
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"
        target: "metadata.uid"
    )
}
```

**Generated GraphQL:**

```graphql
type PlaylistItem {
  type: String!
  value: String!
  title: String

  # Auto-generated relationship field
  dashboard: Dashboard # Nullable for optional relationship
}
```

### One-to-Many Relationship

**CUE:**

```cue
#Folder: {
    spec: {
        dashboards?: [..._] @relation(
            kind: "dashboard.grafana.app/Dashboard"
            field: "metadata.uid"
            target: "spec.folderUID"
            cardinality: "many"
        )
    }
}
```

**Generated GraphQL:**

```graphql
type FolderSpec {
  title: String!

  # Auto-generated relationship field
  dashboards: [Dashboard!]! # Non-null array for "many" cardinality
}
```

## Relationship Resolution Logic

### Resolution Algorithm

1. **Parse Relationship**: Extract relationship metadata from CUE attributes
2. **Extract Reference Value**: Get value from local field path
3. **Find Target Subgraph**: Locate subgraph that provides target kind
4. **Query Target**: Execute query against target subgraph's storage
5. **Return Result**: Return resolved resource(s) or null/empty array

### Example Resolution Flow

```graphql
query {
  playlist_playlist(namespace: "default", name: "my-playlist") {
    spec {
      items {
        value # "dashboard-123"
        dashboard {
          # Relationship resolution triggered
          spec {
            title
          }
        }
      }
    }
  }
}
```

**Resolution Steps:**

1. Query `playlist_playlist` → returns playlist with items
2. For each item, extract `value` field → `"dashboard-123"`
3. Find dashboard subgraph for `dashboard.grafana.app/Dashboard`
4. Query dashboard storage: `Get(uid="dashboard-123")`
5. Return dashboard resource or null if not found

## Error Handling

### Missing Target Resource

**Optional Relationship (`optional: true`):**

```json
{
  "data": {
    "playlist_playlist": {
      "spec": {
        "items": [
          {
            "value": "nonexistent-dashboard",
            "dashboard": null // Graceful degradation
          }
        ]
      }
    }
  }
}
```

**Required Relationship (`optional: false`):**

```json
{
  "errors": [
    {
      "message": "Required relationship 'dashboard' not found for value 'nonexistent-dashboard'",
      "path": ["playlist_playlist", "spec", "items", 0, "dashboard"]
    }
  ],
  "data": null
}
```

### Unreachable Subgraph

```json
{
  "errors": [
    {
      "message": "Subgraph for 'dashboard.grafana.app/Dashboard' is unreachable",
      "path": ["playlist_playlist", "spec", "items", 0, "dashboard"]
    }
  ],
  "data": {
    "playlist_playlist": {
      "spec": {
        "items": [
          {
            "value": "dashboard-123",
            "dashboard": null // Null for unreachable relationships
          }
        ]
      }
    }
  }
}
```

## Performance Considerations

### N+1 Query Problem

**Problem:** Naive resolution executes one query per relationship.

**Solution:** Implement DataLoader pattern for batching:

```go
type RelationshipDataLoader struct {
    subgraph SubgraphInterface
    batchFn  func(keys []string) ([]interface{}, error)
    cache    map[string]interface{}
}

// Batch multiple relationship requests
func (loader *RelationshipDataLoader) LoadMany(keys []string) []interface{} {
    // Collect unique keys and batch query to target subgraph
    // Return results in same order as keys
}
```

### Caching Strategy

- **Resource Cache**: Cache resolved resources by key
- **Relationship Cache**: Cache relationship mappings
- **TTL Strategy**: Configurable cache expiration

## Validation Rules

### Compile-Time Validation

1. **Kind Exists**: Target kind must be registered in federation
2. **Field Paths Valid**: Local and target field paths must be valid
3. **Cardinality Consistency**: Array fields for "many", single fields for "one"
4. **Circular Dependencies**: Detect and prevent circular relationships

### Runtime Validation

1. **Subgraph Availability**: Target subgraph must be reachable
2. **Field Value Types**: Reference values must match expected types
3. **Permission Checks**: User must have read access to target resources

## Implementation Phases

### Phase 1: Basic One-to-One Relationships

- [ ] CUE attribute parsing
- [ ] Simple relationship resolution
- [ ] Error handling for missing targets

### Phase 2: Advanced Relationships

- [ ] One-to-many cardinality
- [ ] Complex matching strategies
- [ ] Performance optimization with batching

### Phase 3: Production Features

- [ ] Caching layer
- [ ] Permission integration
- [ ] Monitoring and metrics

## Examples for Hackathon Demo

### Demo Scenario: Playlist → Dashboard Relationships

```cue
// In playlist app
#PlaylistKind: {
    spec: {
        items: [...#PlaylistItem]
    }
}

#PlaylistItem: {
    type: "dashboard_by_uid"
    value: string  // Dashboard UID

    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"
        target: "metadata.uid"
    )
}
```

**Demo Query:**

```graphql
query PlaylistDemo {
  playlist_playlist(namespace: "default", name: "demo-playlist") {
    metadata {
      name
    }
    spec {
      title
      items {
        type
        value
        dashboard {
          metadata {
            name
            uid
          }
          spec {
            title
            description
            tags
          }
        }
      }
    }
  }
}
```

This specification provides a complete foundation for implementing cross-app relationships in our federated GraphQL system.
