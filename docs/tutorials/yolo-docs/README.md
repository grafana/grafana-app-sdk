# App Platform Quickstart

This is designed as the quickest possible way for you to figure out what 
app platform is, how you can benefit, and what you should do next.

## What is the App Platform?

App Platform is a system that lets you build Grafana plugins by **defining your data structure**. The platform **automatically generates a complete API** for you.  It has options to add operators which
can reconcile & watch resources, and ways you can plug in custom APIs of your own.

It's called app platform because this is the new default way to build things on the Grafana Platform.
Everything's an app, which can have front-end, back-end, and operator segments. Doing it this way reduces toil, gets many features for free, and has your team doing less heavy lifting.

**🚀 Generated APIs** - No need to write/maintain CRUD endpoints or OpenAPI specs. Define your schema and get a complete API automatically.

**🔒 Multi-tenancy, Terraform, RBAC Built-in** - Your data is automatically isolated by namespace/tenant. Different teams can have resources with the same names without conflicts. No customer feature
requests ("does this work with RBAC?"). No security incidents due to isolation bugs. 
Toil is bad, right?

**📊 Flexible Options** - React to changes async, do event driven stuff with operators and reconcilers, good for automation.

**🎯 Type-Safe** - Code generation gives you type-safe Go & Typescript code.

**Sane Versioning, Tool Compatibility** - use normal things like `kubectl` to interact with
your data, and get a versioning and API approach that will match what everyone else expects.

## So what should I do?

There are three basic patterns. Pick which one suits
what you want to do.

> **💡 Tip**: Most greenfield plugins start as CRD-based and evolve into operator-based as requirements grow. Custom APIs are relatively rare and only needed for specific use cases, but may be the go-to if you have something complex you need to migrate.

| Application Type | You Get | You Build/Provide | Use When |
|------------------|---------|-------------------|----------|
| **CRD-Based** | Complete backend service (API, storage, auth, multi-tenancy) | A `Kind` (your data schema) + Frontend UI to display and edit that data | You want a managed backend - all the infrastructure without writing backend code |
| **Operator-Based** | Everything above, plus event-driven automation | Backend logic that reacts to changes (reconcilers, validators, mutators) | Creating a resource should trigger automation, or you need custom backend behavior outside of the request/response loop |
| **Custom API** | Everything from Operator-Based, plus full control over API behavior | Custom API server with your own logic | You need non-standard endpoints or external data sources |

## What's Next?

To learn app platform concepts, we recommend learning by doing: 3 tutorials, in this order. Each one builds on the last.

- **[CRD-Based Tutorial](./crd-based/)** - Builds a simple issue tracker
- **[Operator-Based Tutorial](./operator-based/)** - Extends the issue tracker with
a watcher that flags issues based on keywords
- **[Custom API Tutorial](./custom-api/)** - Extends the operator-based issue tracker
with a custom service that looks up external related issues.

## Other Resources

If this learning style isn't your thing, you have these other options:

| [Concepts](CONCEPTS.md) | [Technologies](TECHNOLOGIES.md) | [Use Cases](USE-CASES.md) |
|-------------------------|---------------------------------|---------------------------|
| List key ideas app platform relies on, and how they interrelate | key tech used, and how | problems this SDK can help you solve |

- **[How the Platform Works](../../application-design/platform-concepts.md)**
- See [Kubernetes concepts](../../kubernetes.md) if you're new to Kubernetes patterns
- [Other documentation](../../README.md)
