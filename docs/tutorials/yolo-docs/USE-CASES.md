# Use Cases & Application Scenarios

What real-world problems can you solve with app platform?  The examples provided her demonstrate
the SDK's patterns (CRD-based, operator-based, and Custom API) and show how they can be applied to
solve practical problems.

## Complete Tutorial Examples

- **Automatic Issue Flagging** - Event-driven operator that automatically flags urgent issues based on keywords (URGENT, CRITICAL, ASAP, EMERGENCY)  
  *Tutorial: [Operator-Based Tutorial](./operator-based/README.md) | [Read more: Operators](../../operators.md)*

## Cross-App Integration

Applications that watch or interact with resources managed by other Grafana apps, enabling multi-app workflows.

- **Playlist Integration** - Watch or interact with playlist resources from other Grafana apps  
  *Read more: [Watching Unowned Resources](../../watching-unowned-resources.md) | [App Manifest](../../app-manifest.md)*

- **Resource Watching from Multiple Apps** - Monitor resources across applications for cross-app automation  
  *Read more: [Watching Unowned Resources](../../watching-unowned-resources.md)*

## Service Management

Applications for managing services and their associated infrastructure, commonly used for service catalogs and automated provisioning.

- **Service Catalogue** - Create and manage services, linking them to dashboards and alert rules  
  *Read more: [Application Design Patterns](../../application-design/README.md#frontend-only-applications)*

- **Automated Service Provisioning** - Automatically provision dashboards and alerts when a service is created  
  *Read more: [Application Design Patterns](../../application-design/README.md#operator-based-applications) | [Writing a Reconciler](../../writing-a-reconciler.md)*

- **Service Rollback System** - Custom API endpoint for rolling back services to previous versions, useful for responding to failures  
  *Read more: [Application Design Patterns](../../application-design/README.md#applications-with-custom-apis) | [Custom API Tutorial](./custom-api/README.md)*

## Cost & Financial Management

Applications that track, analyze, and report on cloud infrastructure costs and financial metrics.

- **Cost Analytics Plugin** - Real-time cloud cost tracking by fetching data from AWS/GCP/Azure APIs  
  *Read more: [Custom API Tutorial](./custom-api/README.md) | [Application Design Patterns](../../application-design/README.md#applications-with-custom-apis)*

- **Cost Summary by Team** - Aggregated cost views with custom authorization (finance team sees all, teams see only their own)  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

- **Billing Data Integration** - On-demand billing data from external cloud providers without storage requirements  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

## Observability & Monitoring

Applications that extend Grafana's observability capabilities by managing dashboards, alerts, and integrating with monitoring systems.

- **Dashboard Management** - Apps that interact with or watch Grafana dashboard resources for automation  
  *Read more: [Watching Unowned Resources](../../watching-unowned-resources.md) | [Writing an App](../../writing-an-app.md)*

- **Alert Rule Management** - Create, manage, and link alert rules to services or other resources  
  *Read more: [Application Design Patterns](../../application-design/README.md#operator-based-applications)*

- **Panel and Query Management** - Manage Grafana panels and queries as custom resources through the API  
  *Read more: [Platform Concepts](../../application-design/platform-concepts.md#standardized-restful-apis)*

- **Metrics Aggregation** - Aggregate metrics from multiple monitoring systems in real-time  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

## Data Storage & Custom Backends

Applications requiring specialized storage strategies beyond standard CRUD operations.

- **Time-Series Data Applications** - Apps using custom storage backends like InfluxDB for specialized data types  
  *Read more: [Custom API Tutorial](./custom-api/README.md) | [Application Design Patterns](../../application-design/README.md#applications-with-custom-apis)*

- **Read-Only System Metrics** - Expose read-only data from external systems without storage  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

- **Custom Storage Backend** - Use Redis, memcached, or other backends when platform storage doesn't fit requirements  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

## Resource Validation & Mutation

Applications that enforce data quality, business rules, and automatically enhance resources during creation/update.

- **Custom Validation Logic** - Implement validation beyond schema constraints (e.g., preventing specific resource names)  
  *Read more: [Admission Control](../../admission-control.md) | [Tutorial: Adding Admission Control](../issue-tracker/08-adding-admission-control.md)*

- **Automatic Default Values** - Mutate resources during creation to add labels, annotations, or default field values  
  *Read more: [Admission Control](../../admission-control.md)*

- **Multi-Version Resource Management** - Handle conversion and migration between different API versions  
  *Read more: [Managing Multiple Versions](../../custom-kinds/managing-multiple-versions.md)*

## Third-Party Integration

Applications that sync resource state with external systems or trigger actions in external services.

- **Third-Party Service Integration** - Sync resource state to external APIs and services  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md#an-example-reconciler) | [Operators](../../operators.md)*

- **External System Automation** - Trigger workflows in external systems when resources are created/updated/deleted  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md) | [Platform Concepts](../../application-design/platform-concepts.md#asynchronous-business-logic)*

## DevOps & GitOps

Applications designed for declarative infrastructure management and continuous deployment workflows.

- **GitOps-Enabled Applications** - CRD-based apps automatically support declarative management with Flux and ArgoCD  
  *Read more: [CRD-Based Tutorial](./crd-based/README.md) | [Application Design Patterns](../../application-design/README.md#frontend-only-applications)*

- **kubectl-Compatible Applications** - All CRD-based apps support kubectl CLI for resource management  
  *Read more: [CRD-Based Tutorial](./crd-based/README.md) | [Kubernetes Concepts](../../kubernetes.md)*

- **Backup-Enabled Applications** - Apps that automatically support backup tools like Velero  
  *Read more: [Application Design Patterns](../../application-design/README.md#frontend-only-applications)*

## Extension API Servers

Advanced applications with custom API endpoints, subresources, and specialized request handling.

- **Custom Subresources** - Extension API servers with custom subresources like `/foo`, `/bar`, `/rollback`  
  *Read more: [API Server Example](../../../examples/apiserver/README.md) | [Custom API Tutorial](./custom-api/README.md)*

- **Custom Route Handlers** - Non-standard API routes for specialized operations  
  *Read more: [API Server Example](../../../examples/apiserver/README.md)*

- **Custom Query Patterns** - Complex filtering, aggregation, and search not supported by standard Kubernetes queries  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

- **Custom Authorization** - Fine-grained permissions beyond standard RBAC  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

## Learning & Development Examples

Simple demonstration applications for learning SDK concepts and patterns.

- **Basic Custom Resource Operator** - Simple operator that monitors custom resources and logs events  
  *Example: [Simple Operator](../../../examples/operator/simple/README.md)*

- **Resource Store CRUD Operations** - Demonstration of CRUD operations without full kubernetes client complexity  
  *Example: [Resource Store Example](../../../examples/resource/store/README.md)*

- **Watcher-Based Operator** - Example operator using watcher pattern for event handling  
  *Example: [Watcher Example](../../../examples/operator/simple/watcher/main.go)*

- **Reconciler-Based Operator** - Example operator using reconciler pattern for state management  
  *Example: [Reconciler Example](../../../examples/operator/simple/reconciler/main.go)*

## Pattern Selection Guide

Choose the right pattern for your use case:

- **Use CRD-Based** - Simple CRUD with forms, basic validation, no backend logic  
  *Read more: [CRD-Based Tutorial](./crd-based/README.md)*

- **Use Operator-Based** - Auto-reactions to changes, validation/mutation webhooks, async reconciliation  
  *Read more: [Operator-Based Tutorial](./operator-based/README.md)*

- **Use Custom API** - Custom endpoints, external data sources, non-CRUD operations, custom storage  
  *Read more: [Custom API Tutorial](./custom-api/README.md)*

## Architecture Resources

- **[Application Design Patterns](../../application-design/README.md)** - Comprehensive guide to all three patterns
- **[Platform Concepts](../../application-design/platform-concepts.md)** - Understanding the platform architecture
- **[Operators Guide](../../operators.md)** - Deep dive into operator patterns
- **[Writing a Reconciler](../../writing-a-reconciler.md)** - Best practices for reconciliation logic
- **[Admission Control](../../admission-control.md)** - Validation and mutation webhooks

