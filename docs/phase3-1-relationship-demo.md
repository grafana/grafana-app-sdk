# Phase 3.1: Relationship Support Demo

## Overview

Phase 3.1 adds support for relationships between kinds in federated GraphQL. This enables cross-app queries where data from multiple apps can be fetched in a single GraphQL request.

## How It Works

### 1. Relationship Configuration

Apps can register relationships explicitly:

```go
package main

import (
    "fmt"
    "cuelang.org/go/cue/cuecontext"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "github.com/grafana/grafana-app-sdk/graphql/codegen"
)

func main() {
    // Create relationship parser
    cueCtx := cuecontext.New()
    relationshipParser := codegen.NewRelationshipParser(cueCtx)

    // Register relationship: PlaylistItem.dashboard -> Dashboard (by UID)
    relationshipConfig := &codegen.RelationshipConfig{
        FieldName:   "dashboard",           // GraphQL field name
        Kind:        "dashboard.grafana.app/Dashboard", // Target kind
        SourceField: "spec.items.value",    // Local field with reference
        TargetField: "metadata.uid",        // Target field to match
        Optional:    true,                  // Can be null
        Cardinality: "one",                 // One dashboard per item
        Match:       "exact",               // Exact matching
        TargetGVK: schema.GroupVersionKind{
            Group:   "dashboard.grafana.app",
            Version: "v0alpha1",
            Kind:    "Dashboard",
        },
    }

    // Register for playlist kind
    relationshipParser.RegisterRelationship("Playlist", relationshipConfig)

    fmt.Printf("Registered relationship: %s -> %s\n",
        relationshipConfig.FieldName,
        relationshipConfig.Kind)
}
```

### 2. Generated GraphQL Schema

The relationship automatically adds fields to the GraphQL schema:

```graphql
type Playlist {
  apiVersion: String
  kind: String
  metadata: ObjectMeta
  spec: JSON
  status: JSON

  # Automatically added relationship field
  dashboard: Dashboard # Resolves dashboard by UID
}

type PlaylistItem {
  type: String
  value: String # Contains dashboard UID
  title: String

  # Relationship field added here too
  dashboard: Dashboard
}
```

### 3. GraphQL Generator Integration

The GraphQL generator automatically includes relationship fields:

```go
// Create generator with relationship support
generator := codegen.NewGraphQLGenerator(kinds, gv, storageGetter)
    .WithRelationships(relationshipParser, subgraphRegistry)

// Generate schema - includes relationship fields automatically
schema, err := generator.GenerateSchema()
```

### 4. Cross-App Queries

This enables powerful cross-app queries:

```graphql
# Basic playlist query
query {
  playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
      uid
    }
    spec {
      title
      items {
        type
        value
      }
    }
  }
}

# Playlist with dashboard relationships
query {
  playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
      uid
    }
    spec {
      title
      items {
        type
        value
        dashboard {
          # This resolves the referenced dashboard!
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

## Benefits

### For Developers

- **No GraphQL Knowledge Required**: Just register relationship configs
- **Automatic Resolution**: Fields are added to schema automatically
- **Type Safety**: Relationships use existing storage interfaces

### For API Consumers

- **Single Request**: Fetch related data in one GraphQL query
- **Avoid N+1 Problems**: Relationships can be batched and cached
- **Rich Queries**: Traverse relationships between apps seamlessly

### For Platform

- **Decentralized**: Each app defines its own relationships
- **Performance**: Built-in optimization opportunities
- **Extensible**: Easy to add new relationship types

## Implementation Status

✅ **Relationship Configuration API**: Complete
✅ **GraphQL Field Generation**: Complete  
✅ **Basic Resolution Logic**: Complete
✅ **Integration with Generator**: Complete

## Next Steps (Phase 3.2)

- **CUE Attribute Parsing**: Parse `@relation` from CUE definitions
- **Enhanced Type Mapping**: Rich CUE to GraphQL type conversion
- **Performance Optimization**: DataLoader batching and caching
- **Advanced Matching**: Array contains, complex matching strategies

## Demo Impact

This feature enables the core value proposition of federated GraphQL:

**Before**: Multiple REST API calls needed

```bash
# Get playlist
curl /api/playlists/my-playlist

# For each item, get dashboard
curl /api/dashboards/dashboard-456
curl /api/dashboards/dashboard-789
# ... N+1 requests
```

**After**: Single GraphQL query

```graphql
query {
  playlist(name: "my-playlist") {
    spec {
      items {
        value
        dashboard {
          spec {
            title
          }
        } # Resolved automatically!
      }
    }
  }
}
```

This demonstrates the power of federated GraphQL for the App Platform ecosystem.
