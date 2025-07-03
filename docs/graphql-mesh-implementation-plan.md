# GraphQL Mesh-Style Implementation Plan

## Overview

This document outlines the implementation plan for migrating from the current Apollo Federation-inspired approach to a Mesh-style GraphQL architecture using CUE-based relationship definitions.

## Current State Analysis

### ✅ What We Have

- **Native Go Gateway**: Runtime schema composition implemented
- **CUE Schema Generation**: Automatic GraphQL schema generation from CUE kinds
- **Storage Delegation**: GraphQL resolvers delegate to existing REST storage
- **Auto-Discovery**: Gateway automatically finds GraphQL-capable providers
- **Basic Subgraph Support**: Apps can implement `GetGraphQLSubgraph()` interface

### ❌ What Needs to Change

- **Federation-Style Relationships**: Currently uses `@key`, `@external`, `@extends` patterns
- **Manual Schema Modifications**: Apps must manually add federation directives
- **Complex Entity Resolution**: Each app needs custom entity resolvers
- **Limited Relationship Support**: Only supports basic federation patterns

## Implementation Phases

### Phase 1: CUE Relationship Parser (Foundation)

**Goal**: Build the foundation for extracting relationships from CUE metadata

#### 1.1 CUE Relationship Schema Design

```cue
// Define the standard _relationships schema
#RelationshipDefinition: {
    target: {
        kind: string
        group: string
        version: string
    }
    resolver: {
        sourceField: string
        condition?: string
        targetQuery: string
        targetArgs: [string]: string
        cardinality?: "one" | "many"
    }
}

// Example usage in kinds
#PlaylistKind: {
    // ... existing kind definition

    _relationships: {
        [string]: #RelationshipDefinition
    }
}
```

#### 1.2 CUE Parser Implementation

**Files to Create:**

- `grafana-app-sdk/graphql/codegen/cue_relationships.go`
- `grafana-app-sdk/graphql/codegen/cue_relationships_test.go`

**Key Components:**

- `CUERelationshipParser` struct
- `ParseRelationships()` method
- `RelationshipConfig` struct
- Integration with existing CUE loading infrastructure

#### 1.3 Unit Tests

**Test Cases:**

- Parse single relationship from CUE
- Parse multiple relationships from CUE
- Handle missing `_relationships` field
- Parse conditional relationships
- Parse reverse relationships
- Error handling for malformed CUE

#### 1.4 Integration Points

**Existing Integration:**

- Hook into `resource.Kind` interface
- Use existing CUE loading mechanisms
- Integrate with current codegen pipeline

### Phase 2: Mesh-Style Gateway (Core Implementation)

**Duration**:
**Goal**: Replace federation patterns with Mesh-style schema composition

#### 2.1 Gateway Architecture Refactoring

**Files to Modify:**

- `grafana-app-sdk/graphql/gateway/gateway.go`
- `grafana-app-sdk/graphql/gateway/relationships.go` (new)
- `grafana-app-sdk/graphql/gateway/mesh.go` (update existing)

**Key Changes:**

- Replace `FederatedGateway` with `MeshStyleGateway`
- Add relationship resolution logic
- Implement schema stitching instead of federation
- Add relationship field generation

#### 2.2 Relationship Resolution Engine

**Components:**

- `RelationshipResolver` interface
- `RelationshipConfig` processing
- Conditional relationship evaluation
- Cross-service query execution
- Batching and caching support

#### 2.3 Schema Composition Updates

**Features:**

- Automatic relationship field addition
- Type safety preservation
- Field prefixing for relationships
- Conditional field generation

#### 2.4 Testing Strategy

**Test Categories:**

- Unit tests for relationship resolution
- Integration tests with multiple subgraphs
- Performance tests for relationship queries
- Schema composition validation

### Phase 3: CUE Integration (Developer Experience)

**Duration**:
**Goal**: Seamless integration with existing CUE development workflow

#### 3.1 CUE Schema Updates

**Files to Update:**

- App-specific CUE files (playlist, dashboard, etc.)
- CUE schema validation
- Documentation examples

**Migration Path:**

- Convert existing federation patterns to `_relationships`
- Add relationship definitions to existing kinds
- Update CUE validation schemas

#### 3.2 Code Generation Updates

**Files to Modify:**

- `grafana-app-sdk/codegen/jennies/` (various files)
- Schema generation templates
- Relationship field templates

**Features:**

- Auto-generate relationship fields in GraphQL schema
- Maintain backward compatibility
- Support for conditional relationships

#### 3.3 App Provider Interface Updates

**Files to Update:**

- `grafana-app-sdk/graphql/subgraph/subgraph.go`
- App provider implementations

**Changes:**

- Extend `GraphQLSubgraph` interface
- Add `GetRelationships()` method
- Preserve existing `GetGraphQLSubgraph()` compatibility

### Phase 4: Migration and Validation (Rollout)

**Duration**:
**Goal**: Migrate existing apps and validate the new approach

#### 4.1 App Migration

**Apps to Migrate:**

- Playlist app (primary example)
- Dashboard app (relationship target)
- Investigation app (multiple relationships)

**Migration Steps:**

1. Add `_relationships` metadata to CUE kinds
2. Remove federation-specific directives
3. Update app providers to use new interface
4. Test relationship resolution

#### 4.2 Integration Testing

**Test Scenarios:**

- Cross-service relationship queries
- Conditional relationship resolution
- Reverse relationship navigation
- Performance comparison with federation approach

#### 4.3 Documentation Updates

**Files to Update:**

- `grafana-app-sdk/docs/graphql-federation-design.md` (already done)
- App development guides
- Relationship definition examples
- Migration guides

#### 4.4 Backwards Compatibility

**Compatibility Strategy:**

- Support both federation and Mesh-style approaches during transition
- Gradual migration path for existing apps
- Feature flag for enabling new approach

### Phase 5: Enhancement and Optimization (Future)

**Duration**:
**Goal**: Add advanced Mesh-style features and optimizations

#### 5.1 Advanced Relationship Features

**Features to Add:**

- Nested relationship resolution
- Relationship filtering and sorting
- Pagination support for relationships
- Complex relationship conditions

#### 5.2 Performance Optimizations

**Optimizations:**

- Query batching for relationships
- Relationship result caching
- Efficient schema composition
- Memory usage optimization

#### 5.3 Developer Tools

**Tools to Create:**

- Relationship visualization
- CUE relationship validation
- GraphQL schema introspection
- Performance monitoring

## Implementation Details

### Key Components

#### 1. CUE Relationship Parser

```go
type CUERelationshipParser struct {
    kinds []resource.Kind
    cueLoader CUELoader
}

func (p *CUERelationshipParser) ParseRelationships() ([]RelationshipConfig, error) {
    // Implementation details from previous conversation
}
```

#### 2. Mesh Gateway

```go
type MeshStyleGateway struct {
    subgraphs     map[string]CUEAwareSubgraph
    relationships []RelationshipConfig
    schema        *graphql.Schema
}

func (g *MeshStyleGateway) ComposeSchema() (*graphql.Schema, error) {
    // Schema composition with relationship fields
}
```

#### 3. Relationship Configuration

```go
type RelationshipConfig struct {
    SourceType        string
    SourceField       string
    TargetType        string
    TargetService     string
    TargetQuery       string
    TargetArguments   map[string]string
    Condition         string
    Transform         TransformFunc
}
```

### Migration Path

#### Step 1: Prepare CUE Definitions

```cue
// Before (federation-style)
#PlaylistItem: {
    type: "dashboard_by_uid"
    value: string
    dashboard: #DashboardReference  // Manual reference type
}

// After (Mesh-style)
#PlaylistKind: {
    // ... existing definition

    _relationships: {
        "spec.items.dashboard": {
            target: {
                kind: "Dashboard"
                group: "dashboard.grafana.app"
                version: "v1alpha1"
            }
            resolver: {
                sourceField: "value"
                condition: "type == 'dashboard_by_uid'"
                targetQuery: "dashboard"
                targetArgs: {
                    namespace: "default"
                    name: "{source.value}"
                }
            }
        }
    }
}

#PlaylistItem: {
    type: "dashboard_by_uid"
    value: string
    // dashboard field added automatically
}
```

#### Step 2: Update App Providers

```go
// Before (federation-style)
func (p *PlaylistProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    // Complex federation setup with entity resolvers
}

// After (Mesh-style)
func (p *PlaylistProvider) GetGraphQLSubgraph() (subgraph.GraphQLSubgraph, error) {
    // Simple CUE-based subgraph creation
    return subgraph.NewCUEAwareSubgraph(p.kinds)
}
```

#### Step 3: Gateway Configuration

```go
// Before (federation-style)
gateway := gateway.NewFederatedGateway(config)

// After (Mesh-style)
gateway := gateway.NewMeshStyleGateway(config)
err := gateway.ParseAndConfigureRelationships()
```

## Success Metrics

### Developer Experience

- **Reduced Complexity**: Apps no longer need federation knowledge
- **Faster Development**: Relationships defined in familiar CUE patterns
- **Fewer Bugs**: Automatic relationship generation reduces manual errors

### Performance

- **Query Efficiency**: Batch relationship resolution
- **Memory Usage**: Efficient schema composition
- **Startup Time**: Fast relationship parsing and caching

### Functionality

- **Relationship Support**: All use cases covered by new approach
- **Type Safety**: Preserved through automatic generation
- **Backwards Compatibility**: Smooth migration path

## Process

```
Phase 1: CUE Relationship Parser
Phase 2: Mesh-Style Gateway
Phase 3: CUE Integration
Phase 4: Migration and Validation
Phase 5: Enhancement (Future)
```

## Conclusion

This implementation plan provides a structured approach to migrating from federation-style to Mesh-style GraphQL architecture. The phased approach ensures minimal disruption while providing significant benefits in developer experience and architectural simplicity.

The key success factor is maintaining the CUE-first development workflow while adding powerful relationship capabilities through metadata-driven configuration rather than complex federation patterns.
