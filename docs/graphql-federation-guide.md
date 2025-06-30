# GraphQL Federation Guide

## Overview

GraphQL Federation in the App Platform lets you automatically generate GraphQL APIs from your existing CUE kinds with **zero GraphQL knowledge required**. Your app continues to work exactly as before, but also gets a rich GraphQL API for free.

## Quick Start

### 1. Add GraphQL Support to Your App (One Line!)

```go
// In your app provider
func (p *MyAppProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    return subgraph.CreateSubgraphFromConfig(subgraph.SubgraphProviderConfig{
        GroupVersion: schema.GroupVersion{
            Group: "myapp.company.com", 
            Version: "v1",
        },
        Kinds: []resource.Kind{MyKind()}, // Your existing kinds
        StorageGetter: func(gvr schema.GroupVersionResource) subgraph.Storage {
            return &storageAdapter{storage: p.GetStorage(gvr)}
        },
    })
}
```

### 2. That's It!

Your app now provides GraphQL automatically. The App Platform's federated gateway will:
- Auto-discover your GraphQL subgraph
- Generate schemas from your CUE kinds
- Compose them with other apps
- Provide a unified `/graphql` endpoint

## Core Concepts

### Automatic Schema Generation

Your existing CUE kinds become GraphQL types automatically:

**CUE Definition:**
```cue
#PlaylistKind: {
    apiVersion: "playlist.grafana.app/v0alpha1"
    kind: "Playlist"
    spec: {
        title: string
        description?: string
        interval: "5s" | "10s" | "30s" | "1m" | "5m"
        items: [...#PlaylistItem]
    }
}

#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string
    title?: string
}
```

**Generated GraphQL:**
```graphql
enum PlaylistInterval {
  FIVE_SECONDS
  TEN_SECONDS
  THIRTY_SECONDS
  ONE_MINUTE  
  FIVE_MINUTES
}

type Playlist {
  apiVersion: String!
  kind: String!
  metadata: ObjectMeta!
  spec: PlaylistSpec!
}

type PlaylistSpec {
  title: String!
  description: String
  interval: PlaylistInterval!
  items: [PlaylistItem!]!
}

type PlaylistItem {
  type: PlaylistItemType!
  value: String!
  title: String
}

type Query {
  # Single resource
  playlist_playlist(namespace: String!, name: String!): Playlist
  
  # List resources
  playlist_playlists(namespace: String!): [Playlist!]!
}

type Mutation {
  playlist_createPlaylist(namespace: String!, input: PlaylistInput!): Playlist
  playlist_updatePlaylist(namespace: String!, name: String!, input: PlaylistInput!): Playlist
  playlist_deletePlaylist(namespace: String!, name: String!): Boolean
}
```

### Rich Type Mapping

The system automatically converts CUE constraints to proper GraphQL types:

| CUE Pattern | GraphQL Type | Example |
|-------------|--------------|---------|
| `string` | `String!` | `title: string` → `title: String!` |
| `string?` | `String` | `description?: string` → `description: String` |
| `"a" \| "b"` | `enum` | `"dashboard_by_uid" \| "dashboard_by_tag"` → `enum ItemType` |
| `[...T]` | `[T!]!` | `items: [...#Item]` → `items: [Item!]!` |
| `#EmbeddedType` | `EmbeddedType!` | `spec: #PlaylistSpec` → `spec: PlaylistSpec!` |

## Adding Relationships Between Apps

### Define Relationships in CUE

Use `@relation` attributes to define cross-app relationships:

```cue
#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"  
    value: string
    title?: string

    // Relationship to Dashboard kind
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"           // This field contains the reference
        target: "metadata.uid"   // Match against this field in Dashboard
        optional: true           // Relationship is optional
    )
}
```

### Query Related Data

Relationships automatically add fields to your GraphQL schema:

```graphql
query PlaylistWithDashboards {
  playlist_playlist(namespace: "default", name: "my-playlist") {
    spec {
      title
      items {
        type
        value
        dashboard {  # ← Relationship field added automatically!
          metadata {
            name
            uid
          }
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

## Complete Example: Adding GraphQL to an Existing App

Let's say you have a "Task" app with this CUE definition:

```cue
#TaskKind: {
    apiVersion: "tasks.mycompany.com/v1"
    kind: "Task" 
    spec: {
        title: string
        description?: string
        status: "todo" | "in_progress" | "done"
        priority: "low" | "medium" | "high"
        assignee?: string
        dueDate?: string
        
        // Relationship to user
        assignedUser?: _ @relation(
            kind: "users.mycompany.com/User"
            field: "assignee"
            target: "spec.username"
            optional: true
        )
    }
}
```

### 1. Implement GraphQL Interface

```go
type TaskAppProvider struct {
    // Your existing fields
    storage storage.Interface
}

// Add this single method
func (p *TaskAppProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    return subgraph.CreateSubgraphFromConfig(subgraph.SubgraphProviderConfig{
        GroupVersion: schema.GroupVersion{
            Group: "tasks.mycompany.com",
            Version: "v1", 
        },
        Kinds: []resource.Kind{TaskKind()},
        StorageGetter: func(gvr schema.GroupVersionResource) subgraph.Storage {
            return &taskStorageAdapter{
                storage: p.storage,
                // Add any app-specific storage logic
            }
        },
    })
}
```

### 2. Create Storage Adapter (Usually Boilerplate)

```go
type taskStorageAdapter struct {
    storage storage.Interface
}

func (a *taskStorageAdapter) Get(ctx context.Context, gvr schema.GroupVersionResource, name string, opts metav1.GetOptions) (runtime.Object, error) {
    return a.storage.Get(ctx, gvr, name, opts)
}

func (a *taskStorageAdapter) List(ctx context.Context, gvr schema.GroupVersionResource, opts metav1.ListOptions) (runtime.Object, error) {
    return a.storage.List(ctx, gvr, opts) 
}

func (a *taskStorageAdapter) Create(ctx context.Context, gvr schema.GroupVersionResource, obj runtime.Object, opts metav1.CreateOptions) (runtime.Object, error) {
    return a.storage.Create(ctx, gvr, obj, opts)
}

func (a *taskStorageAdapter) Update(ctx context.Context, gvr schema.GroupVersionResource, obj runtime.Object, opts metav1.UpdateOptions) (runtime.Object, error) {
    return a.storage.Update(ctx, gvr, obj, opts)
}

func (a *taskStorageAdapter) Delete(ctx context.Context, gvr schema.GroupVersionResource, name string, opts metav1.DeleteOptions) error {
    return a.storage.Delete(ctx, gvr, name, opts)
}
```

### 3. Use the Generated GraphQL API

```graphql
# Query tasks
query GetTasks {
  tasks_tasks(namespace: "default") {
    metadata { name }
    spec {
      title
      status      # Enum: TODO | IN_PROGRESS | DONE
      priority    # Enum: LOW | MEDIUM | HIGH
      assignedUser {  # Cross-app relationship!
        metadata { name }
        spec {
          username
          email
        }
      }
    }
  }
}

# Create a task
mutation CreateTask {
  tasks_createTask(
    namespace: "default"
    input: {
      title: "Fix GraphQL docs"
      status: TODO
      priority: HIGH
      assignee: "alice"
    }
  ) {
    metadata { name }
    spec { title }
  }
}
```

## Advanced Features

### Complex Relationships

```cue
// One-to-many relationship
#Project: {
    spec: {
        name: string
        tasks: [...string] @relation(
            kind: "tasks.mycompany.com/Task"
            field: "tasks"         // Array of task names
            target: "metadata.name"
            cardinality: "many"    // One project, many tasks
        )
    }
}

// Cross-namespace relationships
#Dashboard: {
    spec: {
        folderUID?: string @relation(
            kind: "folders.grafana.com/Folder"
            field: "folderUID"
            target: "metadata.uid"
            crossNamespace: true    // Look across namespaces
        )
    }
}
```

### Custom Query Filtering

Add custom resolvers for advanced filtering:

```go
func (p *TaskAppProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    sg, err := subgraph.CreateSubgraphFromConfig(/* ... */)
    if err != nil {
        return nil, err
    }
    
    // Add custom queries
    sg.AddCustomQuery("tasksByStatus", &graphql.Field{
        Type: graphql.NewList(sg.GetType("Task")),
        Args: graphql.FieldConfigArgument{
            "status": &graphql.ArgumentConfig{Type: graphql.String},
        },
        Resolve: p.resolveTasksByStatus,
    })
    
    return sg, nil
}
```

## Best Practices

### 1. Keep Relationships Simple
```cue
// ✅ Good: Simple reference
assignedUser?: _ @relation(
    kind: "users.mycompany.com/User"
    field: "assignee"
    target: "spec.username"
)

// ❌ Avoid: Complex nested references
assignedUser?: _ @relation(
    kind: "users.mycompany.com/User" 
    field: "spec.assignment.user.id"
    target: "metadata.annotations.externalId"
)
```

### 2. Use Meaningful Field Names
```cue
// ✅ Good: Clear relationship names
dashboard?: _ @relation(...)
assignedUser?: _ @relation(...)

// ❌ Avoid: Generic names
ref?: _ @relation(...)
data?: _ @relation(...)
```

### 3. Handle Optional Relationships
```cue
// Always specify if relationships are optional
assignedUser?: _ @relation(
    // ...
    optional: true  // ✅ Explicit
)
```

## Troubleshooting

### Common Issues

**Schema Not Updating**
- Restart the App Platform gateway
- Check that your app provider implements `GetGraphQLSubgraph()` 
- Verify your CUE definitions are valid

**Relationships Not Resolving**
- Ensure target app is registered and running
- Check that field names and targets match exactly
- Verify namespace access permissions

**Storage Errors**
- Ensure your storage adapter implements all required methods
- Check that storage layer is properly initialized
- Verify permissions for CRUD operations

### Debug Queries

```graphql
# Check available types
query IntrospectTypes {
  __schema {
    types {
      name
      kind
    }
  }
}

# Check specific type fields
query IntrospectTask {
  __type(name: "Task") {
    fields {
      name
      type {
        name
      }
    }
  }
}
```

## Migration Guide

### From REST to GraphQL

Your existing REST APIs continue to work! GraphQL is purely additive:

```bash
# Existing REST API (still works)
curl /apis/tasks.mycompany.com/v1/namespaces/default/tasks

# New GraphQL API (also works)
curl -X POST /graphql -d '{"query": "{ tasks_tasks(namespace: \"default\") { metadata { name } } }"}'
```

### Adding to Existing Apps

1. **No breaking changes required** - GraphQL is purely additive
2. **Implement one interface method** - `GetGraphQLSubgraph()`
3. **Reuse existing storage** - No data migration needed
4. **Add relationships gradually** - Start simple, add `@relation` attributes as needed

## Next Steps

- **Try the examples** - Start with a simple kind
- **Add relationships** - Connect your app to others
- **Explore advanced queries** - Use GraphQL's powerful query capabilities
- **Monitor performance** - GraphQL federation includes built-in metrics

## API Reference

### Storage Interface

All storage adapters must implement:

```go
type Storage interface {
    Get(ctx context.Context, gvr schema.GroupVersionResource, name string, opts metav1.GetOptions) (runtime.Object, error)
    List(ctx context.Context, gvr schema.GroupVersionResource, opts metav1.ListOptions) (runtime.Object, error) 
    Create(ctx context.Context, gvr schema.GroupVersionResource, obj runtime.Object, opts metav1.CreateOptions) (runtime.Object, error)
    Update(ctx context.Context, gvr schema.GroupVersionResource, obj runtime.Object, opts metav1.UpdateOptions) (runtime.Object, error)
    Delete(ctx context.Context, gvr schema.GroupVersionResource, name string, opts metav1.DeleteOptions) error
}
```

### Relationship Attributes

```cue
field?: _ @relation(
    kind: "group.domain.com/Kind"  // Required: Target kind
    field: "fieldName"              // Required: Source field name
    target: "targetField"           // Required: Target field to match
    optional: true | false          // Optional: Default false
    cardinality: "one" | "many"     // Optional: Default "one"
    crossNamespace: true | false    // Optional: Default false
)
```

---

**Next**: [GraphQL Federation Architecture](./graphql-federation-design.md) | [Implementation Status](./graphql-federation-status.md) 
