# Custom API Tutorial: Understanding Extension API Servers

This tutorial explains when and how to build applications with custom API servers that go beyond standard CRUD operations.

## Prerequisites

**Complete [Tutorial 1 (CRD-Based)](../crd-based/) and [Tutorial 2 (Operator-Based)](../operator-based/) first.** This tutorial builds conceptually on those foundations.

## What is a Custom API?

A **custom API** (also called an **extension API server**) gives you full control over how API requests are handled. Instead of using the platform's generated CRUD endpoints, you implement your own API server that handles requests directly.

### When Do You Need This?

Use custom APIs when you need:

1. **Non-CRUD operations** - Actions that don't map to Create/Read/Update/Delete
   - Example: `/rollback`, `/scale`, `/export`

2. **External data sources** - Data computed on-demand from external systems
   - Example: Fetching billing data from cloud providers
   - Example: Aggregating metrics from multiple monitoring systems

3. **Custom query patterns** - Complex filtering or aggregation not supported by standard Kubernetes queries
   - Example: `/costs/by-team`, `/stats/summary`

4. **Custom storage** - Need to use a different storage backend than the platform provides
   - Example: Time-series data in InfluxDB
   - Example: Caching layer with Redis

5. **Custom authorization** - Fine-grained permissions beyond standard RBAC
   - Example: Finance team sees all costs, teams see only their own

### What You DON'T Need Custom APIs For

**Before building a custom API**, consider if these simpler alternatives work:

- **Simple validation** → Use [Admission Control webhooks](../../admission-control.md)
- **Async automation** → Use [Operators/Watchers](../../operators.md) (Tutorial 2)
- **Default values** → Use Mutating webhooks
- **Status tracking** → Use the `.status` subresource with reconcilers

## How Custom APIs Work: The Kubernetes Aggregation Layer

When you create a custom API, requests are routed through the **Kubernetes API Aggregation Layer**:

```
User Request
    ↓
Grafana API Server (authentication, rate limiting)
    ↓
Kubernetes API Aggregation Layer (routing)
    ↓
Your Custom API Server (your code handles the request)
    ↓
Your Storage Backend (or external system)
```

### Key Concepts

1. **API Registration**: Your API server registers with Kubernetes as an `APIService`
2. **Request Routing**: The aggregation layer proxies requests to your server based on the API group/version
3. **Authentication**: Grafana handles authentication; you receive authenticated user info
4. **You Control Everything Else**: Validation, authorization, storage, response format

### Example: Cost Analytics Plugin

Imagine a plugin that shows cloud costs:

```
GET /apis/costs.grafana.app/v1/namespaces/eng-team/costs/summary
```

**With Standard CRD**: You'd need to periodically fetch and store cost data as CRD objects. Complex queries require client-side processing.

**With Custom API**: Your server:
1. Receives the request
2. Calls AWS/GCP/Azure APIs directly
3. Aggregates the data
4. Returns formatted results
5. No storage needed (or uses Redis for caching)

## Working Example: API Server

The grafana-app-sdk includes a complete working example of a custom API server. We recommend studying this example rather than building your own from scratch in this tutorial.

### View the Example

The example is in `examples/apiserver/` in the SDK repository:

**[View API Server Example](../../../examples/apiserver/)**

### What the Example Demonstrates

1. **Custom subresources**: `/foo`, `/bar` endpoints on a custom Kind
2. **Custom version-level routes**: `/foobar` (cluster-scoped and namespaced)
3. **Request handling**: How to process custom route requests
4. **Validation**: Custom validation logic in the API server
5. **Reconciliation**: Operator logic alongside custom APIs
6. **Storage integration**: Using platform storage from custom endpoints

### Key Files to Study

- **`server.go`**: Main entry point, sets up the extension API server
- **`apis/example/v1alpha1/`**: Generated code for custom types
- **Custom route handlers**: Shows how to implement custom logic

### Running the Example

Follow the instructions in the [API Server Example README](../../../examples/apiserver/README.md):

```bash
# Start etcd (storage backend)
cd examples/apiserver
make etcd

# Run the API server
make run

# In another terminal, make requests
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds

# Call custom subresource
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo/foo
```

## Architecture Deep Dive

### Standard CRD-Based (Tutorials 1 & 2)

```
Request → Grafana API → Platform CRUD Logic → Storage → Response
                ↓ (async)
             Operator (watchers, reconcilers)
```

- ✅ Platform handles all request processing
- ✅ Simple to build
- ❌ Limited to CRUD operations
- ❌ No custom query patterns

### Custom API (Extension API Server)

```
Request → Grafana API → Aggregation Layer → Your API Server → Your Logic
                                                  ↓
                                        External Systems / Custom Storage
                                                  ↓
                                              Response
         ↓ (async)
      Operator (optional, can still use operators)
```

- ✅ Full control over request handling
- ✅ Custom endpoints and logic
- ✅ Can integrate external systems
- ✅ Custom storage options
- ❌ More complex to build and maintain
- ❌ You must implement standard CRUD if needed

## When to Use Each Pattern

| Requirement | Pattern |
|-------------|---------|
| Simple CRUD with forms | **CRD-Based (Tutorial 1)** |
| Auto-reactions to changes | **Operator-Based (Tutorial 2)** |
| Validation/mutation | **Operator-Based with webhooks** |
| Custom queries/aggregations | **Custom API** |
| External data (no storage) | **Custom API** |
| Non-CRUD actions | **Custom API** |
| Custom authorization | **Custom API** |

## Building Your Own Custom API

If you've determined you need a custom API, follow these steps:

### 1. Understand the Requirements

- What custom endpoints do you need?
- What external systems will you integrate with?
- Do you need custom storage or will you use platform storage?
- What's your authorization model?

### 2. Study the Example

Work through the [API Server Example](../../../examples/apiserver/) to understand:
- How to set up an extension API server
- How to register your API with Kubernetes
- How to handle custom routes
- How to integrate with storage

### 3. Implement Your Server

Key components:
- API server setup (using `k8s.io/apiserver`)
- Custom route handlers
- Storage backend integration
- Kubernetes API registration (`APIService`)

### 4. Deploy and Test

- Deploy your API server as a pod
- Register it with the aggregation layer
- Test custom endpoints
- Verify authentication and authorization

## Configuration Note: Kubernetes Aggregation

To use custom APIs in your local environment, you need to enable the Kubernetes aggregation layer in `local/config.yaml`:

```yaml
# Enable API aggregation (required for extension API servers)
grafanaKubernetesAggregator: true
```

This enables TLS certificates and configures the aggregation layer to route requests to your custom API server.

> **Note**: This is NOT needed for Tutorial 1 (CRD-based) or Tutorial 2 (Operator-based with simple.App CustomRoutes). Only full extension API servers require aggregation.

## What You Learned

- **When custom APIs are needed** - Non-CRUD operations, external data, custom storage
- **How the aggregation layer works** - Request routing to custom API servers
- **Architecture differences** - CRD vs Operator vs Custom API patterns
- **Decision framework** - Choosing the right pattern for your needs

## Additional Resources

### Complete Example
- **[API Server Example](../../../examples/apiserver/)** - Working implementation with custom routes

### Architecture Documentation
- **[Application Design Patterns](../../application-design/README.md#applications-with-custom-apis)** - Detailed pattern comparison
- **[Platform Concepts](../../application-design/platform-concepts.md)** - Understanding the platform architecture

### Kubernetes Resources
- **[Kubernetes Aggregation Layer](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/)** - Official Kubernetes documentation
- **[Extension API Servers](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-extension-api-server/)** - Kubernetes guide

### SDK Documentation
- **[Resource Objects](../../resource-objects.md)** - Understanding resource abstractions
- **[Resource Stores](../../resource-stores.md)** - Storage interfaces

## Next Steps

1. **Study the Example**: Clone the SDK and run `examples/apiserver/`
2. **Prototype Your API**: Define your custom endpoints and data flow
3. **Implement Incrementally**: Start with one custom endpoint, add more as needed
4. **Test Thoroughly**: Custom APIs require more testing than generated CRUD

Remember: **Start with the simplest pattern that meets your needs**. You can always evolve from CRD-based → Operator-based → Custom API as requirements grow.


