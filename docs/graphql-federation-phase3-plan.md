# GraphQL Federation Phase 3: Enhanced Features Plan

## Overview

Phase 3 builds upon our successful native Go implementation to add advanced federation capabilities. With working auto-discovery, schema composition, and real app integration, we now focus on relationships, performance, and production-readiness.

## Phase 3 Goals

### Primary Objectives

- **Relationship Support**: Enable cross-app data queries with CUE `@relation` attributes
- **Enhanced Type Mapping**: Move beyond JSON scalars to rich CUE type conversion
- **Performance Optimization**: Add caching, batching, and query optimization
- **Security & Permissions**: Implement field-level access control and rate limiting

### Success Criteria

- Cross-app relationship queries work seamlessly (e.g., playlist → dashboard)
- CUE types map properly to GraphQL (strings, numbers, booleans, arrays, objects)
- Query performance matches or exceeds REST API equivalents
- Production-ready security and monitoring capabilities

## 🎯 Phase 3.1: Relationship Support

### Goal: Enable Cross-App Data Relationships

Allow apps to define relationships in CUE and automatically resolve them in GraphQL queries.

#### 3.1.1 CUE Relationship Syntax Design

```cue
// Example: Playlist references Dashboards
#PlaylistKind: {
    apiVersion: "playlist.grafana.app/v0alpha1"
    kind: "Playlist"
    spec: {
        title: string
        items: [...#PlaylistItem]
    }
}

#PlaylistItem: {
    type: "dashboard_by_uid" | "dashboard_by_tag"
    value: string
    title?: string

    // Relationship definition
    dashboard?: _ @relation(
        kind: "dashboard.grafana.app/Dashboard"
        field: "value"           // Local field containing reference
        target: "metadata.uid"   // Target field to match against
        optional: true           // Relationship is optional
    )
}
```

**Design Decisions:**

- Use CUE attributes for relationship metadata
- Support multiple relationship types (one-to-one, one-to-many)
- Allow optional vs required relationships
- Field mapping between different kind schemas

#### 3.1.2 Relationship Resolver Generation

```go
// Enhanced schema generator
type RelationshipResolver struct {
    sourceField   string
    targetKind    resource.Kind
    targetField   string
    optional      bool
    registry      *gateway.AppProviderRegistry
}

func (g *GraphQLGenerator) generateRelationshipResolvers(kind resource.Kind) map[string]*graphql.Field {
    relations := g.parseRelationships(kind)
    resolvers := make(map[string]*graphql.Field)

    for fieldName, relation := range relations {
        resolvers[fieldName] = &graphql.Field{
            Type: g.getGraphQLTypeForKind(relation.TargetKind),
            Resolve: g.createRelationshipResolver(relation),
        }
    }

    return resolvers
}

func (g *GraphQLGenerator) createRelationshipResolver(relation RelationshipConfig) graphql.FieldResolveFn {
    return func(p graphql.ResolveParams) (interface{}, error) {
        // Extract reference value from source object
        refValue := g.extractFieldValue(p.Source, relation.SourceField)

        // Find target subgraph
        targetSubgraph := g.registry.GetSubgraphForKind(relation.TargetKind)

        // Query target resource
        return targetSubgraph.GetStorage().Get(
            p.Context,
            relation.TargetGVR,
            refValue,
            metav1.GetOptions{},
        )
    }
}
```

#### 3.1.3 Cross-Subgraph Query Support

```graphql
# Example query with relationships
query PlaylistWithDashboards {
  playlist_playlist(namespace: "default", name: "my-playlist") {
    metadata {
      name
    }
    spec {
      title
      items {
        type
        value
        title
        # Relationship field auto-resolved
        dashboard {
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

**Implementation:**

- Registry tracks all available subgraphs and their kinds
- Relationship resolvers query appropriate subgraphs
- Error handling for missing/unreachable relationships
- Query optimization to avoid N+1 problems

### 3.1 Deliverables

- [ ] CUE `@relation` attribute specification
- [ ] Relationship resolver generation in `graphql/codegen`
- [ ] Cross-subgraph query resolution in gateway
- [ ] Working example: playlist → dashboard relationships
- [ ] Unit tests for relationship parsing and resolution
- [ ] Documentation for app developers

## 🚀 Phase 3.2: Enhanced Type Mapping

### Goal: Rich CUE to GraphQL Type Conversion

Move beyond JSON scalars to properly typed GraphQL schemas that reflect CUE type constraints.

#### 3.2.1 CUE Type Analysis Enhancement

```go
type CUETypeMapper struct {
    cueContext *cue.Context
}

// Enhanced type mapping
func (m *CUETypeMapper) MapCUEToGraphQL(cueValue cue.Value) graphql.Type {
    switch cueValue.Kind() {
    case cue.StringKind:
        return m.mapStringType(cueValue)
    case cue.NumberKind:
        return m.mapNumberType(cueValue)
    case cue.BoolKind:
        return graphql.Boolean
    case cue.ListKind:
        return m.mapListType(cueValue)
    case cue.StructKind:
        return m.mapStructType(cueValue)
    default:
        return graphql.String // Fallback to JSON scalar
    }
}

func (m *CUETypeMapper) mapStringType(cueValue cue.Value) graphql.Type {
    // Check for string constraints/enums
    constraints := m.extractStringConstraints(cueValue)
    if len(constraints.enum) > 0 {
        return m.createEnumType(constraints.enum)
    }
    return graphql.String
}

func (m *CUETypeMapper) mapNumberType(cueValue cue.Value) graphql.Type {
    // Distinguish between Int and Float based on CUE constraints
    if m.isIntegerConstrained(cueValue) {
        return graphql.Int
    }
    return graphql.Float
}
```

#### 3.2.2 Nested Object Type Generation

```cue
// Complex CUE definition
#PlaylistSpec: {
    title: string
    description?: string
    interval: =~"^[0-9]+(s|m|h)$" | *"5m"  // Duration pattern
    tags: [...string]
    schedule?: #Schedule
    settings: #PlaylistSettings
}

#Schedule: {
    enabled: bool
    cron: string
    timezone?: =~"^[A-Za-z_/]+$"
}

#PlaylistSettings: {
    autoPlay: bool | *true
    loop: bool | *false
    theme?: "light" | "dark" | "auto"
}
```

**Generated GraphQL:**

```graphql
type PlaylistSpec {
  title: String!
  description: String
  interval: String! # Could be Duration scalar type
  tags: [String!]!
  schedule: Schedule
  settings: PlaylistSettings!
}

type Schedule {
  enabled: Boolean!
  cron: String!
  timezone: String
}

type PlaylistSettings {
  autoPlay: Boolean!
  loop: Boolean!
  theme: PlaylistTheme
}

enum PlaylistTheme {
  LIGHT
  DARK
  AUTO
}
```

### 3.2 Deliverables

- [ ] Enhanced CUE type analysis in `graphql/codegen`
- [ ] Nested object type generation
- [ ] Enum type generation from CUE constraints
- [ ] Input type generation for mutations
- [ ] Type validation integration
- [ ] Examples with complex playlist/dashboard schemas

## ⚡ Phase 3.3: Performance Optimization

### Goal: Production-Ready Performance

Optimize query execution, add caching, and implement batching to ensure GraphQL performance matches REST APIs.

#### 3.3.1 Query Batching & DataLoader Pattern

```go
type BatchingGateway struct {
    *FederatedGateway
    dataloaders map[string]*DataLoader
}

type DataLoader struct {
    batchFn    BatchLoadFn
    cache      map[string]interface{}
    batch      []BatchRequest
    batchTimer *time.Timer
}

// Batch similar requests together
func (g *BatchingGateway) batchRequests(requests []GraphQLRequest) []GraphQLRequest {
    batches := make(map[string][]GraphQLRequest)

    for _, req := range requests {
        batchKey := g.getBatchKey(req)
        batches[batchKey] = append(batches[batchKey], req)
    }

    var batchedRequests []GraphQLRequest
    for _, batch := range batches {
        if len(batch) > 1 {
            batchedRequests = append(batchedRequests, g.createBatchedRequest(batch))
        } else {
            batchedRequests = append(batchedRequests, batch[0])
        }
    }

    return batchedRequests
}
```

#### 3.3.2 Response Caching Strategy

```go
type CachedGateway struct {
    *BatchingGateway
    schemaCache    *cache.Cache  // Schema composition cache
    queryCache     *cache.Cache  // Query result cache
    resourceCache  *cache.Cache  // Individual resource cache
}

type CacheConfig struct {
    SchemaTTL    time.Duration
    QueryTTL     time.Duration
    ResourceTTL  time.Duration
    MaxSize      int
}

func (g *CachedGateway) executeQuery(query string, variables map[string]interface{}) (*graphql.Result, error) {
    // Check query cache first
    cacheKey := g.generateCacheKey(query, variables)
    if cached := g.queryCache.Get(cacheKey); cached != nil {
        return cached.(*graphql.Result), nil
    }

    // Execute query
    result, err := g.BatchingGateway.executeQuery(query, variables)
    if err != nil {
        return result, err
    }

    // Cache successful results
    g.queryCache.Set(cacheKey, result, g.config.QueryTTL)
    return result, nil
}
```

### 3.3 Deliverables

- [ ] DataLoader pattern implementation for batching
- [ ] Multi-level caching (schema, query, resource)
- [ ] Query complexity analysis and limits
- [ ] Performance benchmarks vs REST APIs
- [ ] Cache invalidation strategies
- [ ] Monitoring and metrics integration

## 🔒 Phase 3.4: Security & Production Features

### Goal: Enterprise-Ready Security

Add field-level permissions, rate limiting, and production monitoring capabilities.

#### 3.4.1 Field-Level Permissions

```go
type PermissionMiddleware struct {
    rbac *RBACEngine
}

func (m *PermissionMiddleware) checkFieldPermission(
    ctx context.Context,
    user User,
    field string,
    resource string,
) error {
    permission := fmt.Sprintf("graphql:%s.%s", resource, field)
    if !m.rbac.HasPermission(user, permission) {
        return fmt.Errorf("insufficient permissions for field %s", field)
    }
    return nil
}

// Enhanced resolver with permission checks
func (g *GraphQLGenerator) createSecureResolver(
    field string,
    originalResolver graphql.FieldResolveFn,
) graphql.FieldResolveFn {
    return func(p graphql.ResolveParams) (interface{}, error) {
        user := getUserFromContext(p.Context)

        // Check field permissions
        if err := g.permissionMiddleware.checkFieldPermission(
            p.Context,
            user,
            field,
            g.getResourceFromResolveParams(p),
        ); err != nil {
            return nil, err
        }

        return originalResolver(p)
    }
}
```

#### 3.4.2 Rate Limiting & Query Analysis

```go
type QueryAnalyzer struct {
    maxDepth      int
    maxComplexity int
    costAnalysis  *CostAnalysis
}

func (a *QueryAnalyzer) analyzeQuery(query string) (*QueryAnalysis, error) {
    parsed, err := parser.Parse(parser.ParseParams{Source: query})
    if err != nil {
        return nil, err
    }

    analysis := &QueryAnalysis{
        Depth:      a.calculateDepth(parsed),
        Complexity: a.calculateComplexity(parsed),
        Cost:       a.calculateCost(parsed),
    }

    if analysis.Depth > a.maxDepth {
        return nil, fmt.Errorf("query depth %d exceeds limit %d", analysis.Depth, a.maxDepth)
    }

    if analysis.Complexity > a.maxComplexity {
        return nil, fmt.Errorf("query complexity %d exceeds limit %d", analysis.Complexity, a.maxComplexity)
    }

    return analysis, nil
}

type RateLimiter struct {
    limiters map[string]*rate.Limiter
    config   RateLimitConfig
}

func (r *RateLimiter) checkRateLimit(ctx context.Context, user User) error {
    key := r.getUserKey(user)
    limiter := r.getLimiterForUser(key)

    if !limiter.Allow() {
        return fmt.Errorf("rate limit exceeded for user %s", user.ID)
    }

    return nil
}
```

### 3.4 Deliverables

- [ ] Field-level RBAC integration
- [ ] Query complexity analysis and limits
- [ ] Rate limiting per user/role
- [ ] Audit logging for GraphQL operations
- [ ] Schema introspection controls
- [ ] Security documentation and best practices

## 📊 Phase 3 Integration & Testing

### Integration Testing Strategy

#### 3.1 End-to-End Testing

```go
func TestCrossAppRelationships(t *testing.T) {
    // Setup test environment with multiple apps
    playlistProvider := &testPlaylistProvider{}
    dashboardProvider := &testDashboardProvider{}

    registry, err := gateway.AutoDiscovery(playlistProvider, dashboardProvider)
    require.NoError(t, err)

    gateway := registry.GetFederatedGateway()

    // Test relationship query
    query := `
        query {
            playlist_playlist(namespace: "test", name: "my-playlist") {
                spec {
                    items {
                        dashboard {
                            spec {
                                title
                            }
                        }
                    }
                }
            }
        }
    `

    result, err := gateway.ExecuteQuery(query, nil)
    require.NoError(t, err)
    assert.NotNil(t, result.Data)
}

func TestPerformanceVsREST(t *testing.T) {
    // Benchmark GraphQL vs REST API performance
    // Ensure GraphQL performance is within acceptable range
}

func TestSecurityControls(t *testing.T) {
    // Test field-level permissions
    // Test rate limiting
    // Test query complexity limits
}
```

#### 3.2 Performance Benchmarks

- GraphQL query performance vs equivalent REST calls
- Memory usage under load
- Cache hit rates and effectiveness
- Concurrent request handling

#### 3.3 Security Validation

- Field-level permission enforcement
- Rate limiting effectiveness
- Query complexity protection
- Audit log completeness

## 🎯 Phase 3 Success Criteria

### Functional Requirements

- [ ] Cross-app relationship queries work seamlessly
- [ ] Complex CUE types map correctly to GraphQL
- [ ] Query performance within 20% of REST API equivalents
- [ ] Field-level permissions enforced correctly
- [ ] Rate limiting prevents abuse

### Non-Functional Requirements

- [ ] < 100ms additional latency for relationship queries
- [ ] > 90% cache hit rate for repeated queries
- [ ] Zero breaking changes to existing Phase 2 functionality
- [ ] Complete test coverage for new features
- [ ] Production-ready monitoring and alerting

### Documentation Requirements

- [ ] Updated architecture documentation
- [ ] App developer guide for relationships
- [ ] Operations guide for production deployment
- [ ] Security configuration guide
- [ ] Performance tuning guide

## 🔄 Phase 3 Rollout Strategy

### Staged Deployment

1. **Internal Testing**: Deploy to development environment
2. **Limited Beta**: Enable for select apps/teams
3. **Gradual Rollout**: Increase traffic percentage
4. **Full Production**: Complete migration with monitoring

### Rollback Plan

- Feature flags for each Phase 3 capability
- Ability to disable relationships if issues arise
- Cache bypass mechanisms
- Performance monitoring with alerts

## 📈 Phase 3 Monitoring & Metrics

### Key Metrics to Track

- **Query Performance**: Average response time, p95, p99
- **Cache Effectiveness**: Hit rates, eviction rates
- **Relationship Usage**: Cross-app query patterns
- **Security Events**: Permission denials, rate limit hits
- **Error Rates**: Failed queries, timeout rates

### Alerting Strategy

- Query performance degradation
- Cache hit rate drops
- High error rates
- Security threshold breaches
- Resource utilization limits

---

## 🚀 Getting Started with Phase 3

### Prerequisites

- ✅ Phase 1 & 2 complete (working foundation)
- ✅ Real app integration (playlist app)
- ✅ Documentation updated

### Phase 3.1 Kickoff

1. Create relationship specification document
2. Design CUE `@relation` attribute syntax
3. Implement relationship parser in `graphql/codegen`
4. Create cross-subgraph resolver framework

Phase 3 builds on our solid foundation to create a production-ready, feature-rich GraphQL federation system that truly serves the needs of the App Platform ecosystem.
