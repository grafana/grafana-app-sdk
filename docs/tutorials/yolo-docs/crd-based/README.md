# YOLO Quick Start Guide

Get a working Grafana app with custom resources deployed locally in minutes. We're
going to build a simple issue tracker as a Grafana plugin, with a backend generated
for us by app platform, and a simple boilerplate front-end.

## Prerequisites

Before starting, ensure you have the following installed:

- **Go** (for compiling backend code) - [go.dev](https://go.dev/)
- **Node.js tools**:
  - [Yarn](https://yarnpkg.com/) - for frontend dependencies
  - [Mage](https://magefile.org/) - for build tasks
- **Container runtime**:
  - [Docker](https://www.docker.com/get-started/) OR [Podman](https://podman.io/getting-started/installation)
- **Local Kubernetes**:
  - [K3D](https://k3d.io) - for local k8s cluster
  - [Tilt](https://tilt.dev) - for managing local deployment

## Quick Start

### 1. Install the SDK CLI

```bash
go install github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk@latest
```

Ensure `$(go env GOPATH)/bin` is in your `$PATH` for CLI access.

Verify installation:

```bash
grafana-app-sdk --help
```

### 2. Initialize Your Project

Create and initialize a new project directory.

```bash
mkdir issue-tracker-project && cd issue-tracker-project
grafana-app-sdk project init "github.com/grafana/issue-tracker-project"
```

This creates the basic project structure with go modules, CUE modules, and local deployment config.

> **Learn More**: [Project Initialization Details](../../issue-tracker/01-project-init.md)

### 3. Define Your Kind

Download the example Issue kind definition.

```bash
curl -o kinds/issue.cue https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/yolo-docs/crd-based/examples/issue-v1.cue
```

This defines an `Issue` resource with `title`, `description`, `status`, and `flagged` fields.

> **Note**: The `flagged` field will be used later if you add operator functionality (see "What's Next" below).

### 4. Update the Manifest

Add your kind to the app manifest. Edit `kinds/manifest.cue` and change the `kinds` array in the `v1alpha1` section:

```cue
v1alpha1: {
    kinds: [issuev1alpha1]  // Add this
    served: true
    // ... rest unchanged
}
```

The manifest tells Grafana which resources your app provides.

> **Learn More**: [Defining Kinds & Schemas](../../issue-tracker/02-defining-our-kinds.md)

### 5. Generate Code

Generate Go and TypeScript code from your CUE definitions.

```bash
make generate
```

This creates type-safe Go structs, TypeScript interfaces, and CRD definitions in `pkg/generated/` and `plugin/src/generated/`.

> **Learn More**: [Code Generation Details](../../issue-tracker/03-generate-kind-code.md)

### 6. Add Boilerplate Components

Generate frontend and backend boilerplate.

```bash
grafana-app-sdk project component add frontend backend
```

This scaffolds the plugin UI and API handlers.

> **Learn More**: [Boilerplate Generation](../../issue-tracker/04-boilerplate.md)

### 7. Install Dependencies

Fetch all Go dependencies needed by generated and boilerplate code.

```bash
make deps
```

### 8. Build Everything

Build the plugin backend and frontend.

```bash
make build/plugin-backend && make build/plugin-frontend
```

This compiles the Go binaries and bundles the React app.

Note: if you run `make build` it will try to build all three components (backend, frontend, and operator) and fail due to having no operator component.

> **Learn More**: [Build & Deployment Details](../../issue-tracker/05-local-deployment.md)

### 9. Configure Local Deployment

Before deploying, edit `local/config.yaml` to disable the operator (since this is a CRD-only app):

```yaml
# Comment out or empty the operatorImage line:
operatorImage: ""
```

### 10. Deploy Locally

Start your local k3d cluster and deploy the app.

```bash
make local/up
```

Wait for the cluster to initialize, then deploy the plugin:

```bash
make local/deploy_plugin
```

The plugin is deployed to the cluster. The platform will handle all backend functionality automatically.

### 11. Verify Your Deployment

Access Grafana at [http://grafana.k3d.localhost:9999](http://grafana.k3d.localhost:9999) (credentials: `admin`/`admin`).

Test the API directly:

```bash
curl -u admin:admin http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues
```

You should see an empty list of issues:

```json
{
  "apiVersion": "issuetrackerproject.ext.grafana.com/v1alpha1",
  "items": [],
  "kind": "IssueList",
  "metadata": {
    "continue": "",
    "resourceVersion": "..."
  }
}
```

### 12. Create a Test Resource

Create an issue via the API:

```bash
curl -u admin:admin -X POST -H "content-type:application/json" \
  -d '{"kind":"Issue","apiVersion":"issuetrackerproject.ext.grafana.com/v1alpha1","metadata":{"name":"test-issue","namespace":"default"},"spec":{"title":"Test","description":"A test issue","status":"open"}}' \
  http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues
```

Or use kubectl:

```bash
kubectl get issues
```

View your app UI at [http://grafana.k3d.localhost:9999/a/issuetrackerproject-app/](http://grafana.k3d.localhost:9999/a/issuetrackerproject-app/)

## What You Built

You now have a **complete CRD-based application** where the platform provides ALL backend functionality - you only defined the schema!

You get automatically:

- **Custom Resource Type**: `Issue` resources stored in Kubernetes
- **REST API**: Full CRUD operations at `/apis/issuetrackerproject.ext.grafana.com/v1alpha1/`
- **Grafana Plugin**: Basic UI at `/a/issuetrackerproject-app/`
- **Authentication & Authorization**: Built-in RBAC
- **Multi-tenancy**: Namespace isolation
- **kubectl Support**: Manage issues via CLI
- **GitOps Support**: Use tools like Flux for declarative management
- **Local Environment**: K3d cluster with Grafana and your app

## Iterate & Develop

To make changes and redeploy:

```bash
# After code changes
make build

# Redeploy plugin
make local/deploy_plugin
```

## Clean Up

Stop the deployment but keep the cluster:

```bash
make local/down
```

Delete the cluster entirely:

```bash
make local/clean
```

## What's Next? Use-Case Focused Guide

Now that you have a working CRD-based app, you can extend it or explore other concepts:

### Next: Add Operator Functionality

**Want your app to automatically react to changes?** Continue to the next tutorial to add a watcher that automatically flags urgent issues:

- **[Operator-Based Tutorial](../operator-based/)** - Add event-driven automation
- **Concepts**: [Platform Concepts](../../application-design/platform-concepts.md#asynchronous-business-logic)
- **Deep Dive**: [Operators Guide](../../operators.md)

### I want to customize the frontend UI

Build a custom React UI for managing your resources:

- **Tutorial**: [Writing Our Front-End](../../issue-tracker/06-frontend.md)
- **Topics**: API client creation, state management, CRUD operations

### I want a custom backend service, not a generated one

You'll need to use the Kubernetes aggregation layer / extension API servers:

- **[Custom API Tutorial](../custom-api/)** - Add custom endpoints with external data
- **Example**: [API Server Example](../../../examples/apiserver/)
- **See also**: [Applications with Custom APIs](../../application-design/README.md#applications-with-custom-apis)

### I want to understand CUE schemas better

Learn how to evolve schemas, add complex types, and use CUE features:

- **Tutorial**: [Defining Kinds & Schemas](../../issue-tracker/02-defining-our-kinds.md)
- **Deep Dive**: [Writing Custom Kinds](../../../custom-kinds/writing-kinds.md)
- **Topics**: Schema evolution, versioning, complex field types

### I want to add multiple kinds to my app

Manage multiple resource types in a single application:

- **Topics**: Multi-kind manifests, relationships between kinds
- **Documentation**: [Custom Kinds Overview](../../../custom-kinds/)

### I want to understand the resource model

Learn about the underlying abstractions (Object, Kind, Schema, Client, Store):

- **Documentation**: [Resource Objects](../../../resource-objects.md)
- **Documentation**: [Resource Stores](../../../resource-stores.md)

### I want to work with resources from other apps

Access and react to resources managed by other Grafana apps:

- **Documentation**: [Watching Unowned Resources](../../../watching-unowned-resources.md)
- **Tutorial Section**: [07-operator-watcher.md](../../issue-tracker/07-operator-watcher.md) (mentions cross-app events)

### I want to understand the full development workflow

Walk through the complete tutorial with detailed explanations:

- **Start Here**: [Full Tutorial README](../../issue-tracker/README.md)
- **All Sections**: Covers everything from concepts to production considerations

## Troubleshooting

**Plugin won't load**: Ensure you ran `make local/deploy_plugin` after building.

**Port conflicts**: Check that ports 9999 (Grafana) and 10350 (Tilt) are available.

**kubectl can't find resources**: Ensure your kubeconfig points to the k3d cluster:
```bash
kubectl config current-context  # Should show k3d-issue-tracker-project
```

**Grafana fails to start with TLS cert error**: If using an older SDK version and seeing `failed to create aggregator server: missing filename for serving cert`, upgrade to the latest SDK version or add `grafanaKubernetesAggregator: false` to your `local/config.yaml` and run `make local/generate`.

## Additional Resources

### Core Concepts
- [Platform Concepts](../../application-design/platform-concepts.md) - Understanding the platform architecture
- [Application Design Patterns](../../application-design/README.md) - When to use each pattern

### Reference Documentation
- [SDK Documentation](../../README.md)
- [CLI Reference](../../cli.md)
- [Kubernetes Primer](../../kubernetes.md)
- [App Manifest Specification](../../app-manifest.md)


