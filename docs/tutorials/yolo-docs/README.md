# App Platform Quickstart

This is designed as the quickest possible way for you to figure out what 
app platform is, how you can benefit, and what you should do next.

## What is the App Platform?

App Platform is a system that lets you build Grafana plugins by **defining your data structure**, and the platform **automatically generates a complete API** for you.

1. You describe your data
2. The platform creates a full REST API with endpoints for create, read, update, delete, list, and watch
3. Your plugin gets authentication, authorization, and multi-tenancy **for free**
4. Manage your data easily through your UI, CLI tools like `kubectl`, or GitOps tools like Flux

### Wait, what do I get?

**🚀 Generated APIs** - No need to write CRUD endpoints. Define your schema and get a complete API automatically.

**🔒 Multi-tenancy & RBAC Built-in** - Your data is automatically isolated by namespace/tenant. Different teams can have resources with the same names without conflicts. Avoid your team doing undifferentiated heavy lifting 
(like RBAC implementation). Toil is bad.

**📊 Options for Event-Driven** - React to changes asynchronously with operators and reconcilers, good for automation.

**🎯 Type-Safe** - Code generation gives you type-safe Go & Typescript code.

## So what should I do?

There are only three basic patterns for different needs. Pick which one suits
what you want to do.

> **💡 Tip**: Most plugins start as CRD-based and evolve into operator-based as requirements grow. Custom APIs are relatively rare and only needed for specific use cases, but may be the go-to if you have something complex you need to migrate.

### 1. CRD-Based Applications
**You get**: Complete backend service (API, storage, auth, multi-tenancy)  
**You provide**: A `Kind` (your data schema)  
**You build**: Frontend UI to display and edit that data  
**Use when**: You want a managed backend - all the infrastructure without writing backend code  
**The platform handles**: Storage, API endpoints, validation, RBAC

### 2. Operator-Based Applications
**You get**: Everything from pattern 1, plus...  
**You build**: Backend logic that reacts to changes (reconcilers, validators, mutators)  
**Use when**: Creating a resource should trigger automation, or when
you need custom backend behavior outside of the request/response loop

### 3. Custom API Applications
**You get**: Everything from pattern 2, plus full control over API behavior  
**You build**: Custom API server with your own logic  
**Use when**: You need non-standard endpoints or external data sources

## Which Pattern Do I Need?

These are _notional examples only_

### CRD-Based: **Runbooks Catalog**

**What it does**: Allows teams to create a catalog of their runbooks with metadata (owner, on-call team, related dashboards, etc).

**Why this pattern**:
- Just needs to store and display runbook metadata
- Platform provides the complete backend automatically
- Basic schema validation is sufficient
- Users manage runbooks through the UI, but can also use `kubectl` or GitOps tools

---

### Operator-Based: **SLO Monitor Plugin**

**What it does**: Users define SLOs (Service Level Objectives) as custom resources. The operator automatically creates corresponding alerts, recording rules, and status dashboards.

**Why this pattern**:
- When a user creates an SLO, the backend must automatically provision multiple resources
- Set default values if not specified
- Validate complex business rules
- Async work that shouldn't block the API request
- Cleanup generated resources when SLO is deleted

---

### Custom API: **Cross-Cloud Cost Analytics Plugin**

**What it does**: Aggregates billing data from AWS, GCP, and Azure APIs to show cost breakdowns by service, team, or time period.

**Why this pattern**:
- Data is computed on-demand from external APIs, not stored in our platform
- Needs custom endpoints like `/costs/by-team`, `/costs/forecast` that don't map to standard CRUD
- Custom authorization (finance sees all, teams see their own)
- Queries go to external cloud APIs
- Custom caching for expensive operations

## What's Next?

Choose the pattern that fits your needs and follow the corresponding tutorial:

- **[CRD-Based Tutorial](./crd-based/)** - Build a runbooks catalog with managed backend (coming soon)
- **[Operator-Based Tutorial](./operator-based/)** - Build an SLO monitor with automation (coming soon)
- **[Custom API Tutorial](./custom-api/)** - Build a cost analytics plugin (coming soon)

If you're not sure which to choose, start with the **CRD-Based Tutorial** - it's the simplest and you can always add operator functionality later.

## Other Resources

- **[Platform Concepts](../../application-design/platform-concepts.md)** - Deep dive into how the platform works
- **[Application Design Patterns](../../application-design/README.md)** - Detailed explanation of all three patterns
- **[Issue Tracker Tutorial](../issue-tracker/)** - Complete walkthrough of building an operator-based app
- **[API Server Example](../../../examples/apiserver/)** - Working example of a custom API server

## Questions?

- Check the [main documentation](../../README.md)
- See [Kubernetes concepts](../../kubernetes.md) if you're new to Kubernetes patterns
- Browse the [examples](../../../examples/) for working code

