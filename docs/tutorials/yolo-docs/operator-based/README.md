# Operator-Based Tutorial: Add Event-Driven Automation

This tutorial extends the CRD-based issue tracker from Tutorial 1 by adding an operator that automatically flags issues containing urgent keywords.

## Prerequisites

**You must complete [Tutorial 1 (CRD-Based)](../crd-based/) first.** You should have a working issue tracker with:
- Issue CRD defined
- Frontend and backend components
- Local k3d environment running

## What We're Adding

A **watcher** that monitors Issue resources and automatically sets `flagged: true` when an issue's description contains urgent keywords like "URGENT", "CRITICAL", "ASAP", or "EMERGENCY".

This demonstrates the **event-driven pattern** - your code reacts asynchronously to resource changes without blocking the API request.

## Step 1: Add Operator Component

From your project root, generate the operator boilerplate:

```bash
grafana-app-sdk project component add operator
```

This creates:
- `cmd/operator/` - Operator entry point (main.go, config loading)
- `pkg/watchers/` - Watcher stub for each Kind
- `pkg/app/` - App initialization code

## Step 2: Implement the Watcher

Download the complete watcher implementation:

```bash
curl -o pkg/watchers/watcher_issue.go https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/yolo-docs/operator-based/examples/watcher_issue.go
```

Or view and copy the file from [examples/watcher_issue.go](examples/watcher_issue.go).

### How It Works

The watcher implements four methods:

- **Add(ctx, obj)**: Called when a new Issue is created
  - Checks description for urgent keywords
  - Sets `flagged: true` and updates the resource

- **Update(ctx, old, new)**: Called when an Issue is modified
  - Re-uses the Add logic to check the new description

- **Delete(ctx, obj)**: Called when an Issue is deleted
  - No action needed (just logs)

- **Sync(ctx, obj)**: Called for objects that may have changed during operator downtime
  - Re-uses the Add logic to ensure consistency

## Step 3: Update local/config.yaml

Edit `local/config.yaml` and uncomment/set the operator image:

```yaml
# Before:
# operatorImage: ""

# After:
operatorImage: "github.com/grafana/issue-tracker-project:latest"
```

This tells the local deployment to build and deploy the operator container.

## Step 4: Build and Deploy

Regenerate code (includes operator now):

```bash
make generate
```

Build everything including the operator:

```bash
make build
```

Regenerate k8s manifests with operator configuration:

```bash
make local/generate
```

Deploy the operator image to k3d:

```bash
make local/push_operator
```

Restart your tilt deployment to pick up the changes:

```bash
cd local && tilt down && tilt up
```

Or if tilt is already running, it should auto-reload.

## Step 5: Update Frontend (Optional)

To visualize flagged issues, download the updated component:

```bash
curl -o plugin/src/pages/IssueList.tsx https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/yolo-docs/operator-based/examples/IssueList.tsx
```

Or view and copy from [examples/IssueList.tsx](examples/IssueList.tsx).

This adds a simple flag icon (🚩) next to flagged issues in the list view.

Rebuild the frontend:

```bash
make build/plugin-frontend
make local/deploy_plugin
```

## Step 6: Test

### Create an urgent issue

```bash
curl -u admin:admin -X POST -H "content-type:application/json" \
  -d '{"kind":"Issue","apiVersion":"issuetrackerproject.ext.grafana.com/v1alpha1","metadata":{"name":"urgent-bug","namespace":"default"},"spec":{"title":"Critical Bug","description":"URGENT: Production is down!","status":"open"}}' \
  http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues
```

Check that it was flagged:

```bash
curl -u admin:admin http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues/urgent-bug | jq '.spec.flagged'
# Should output: true
```

### Create a normal issue

```bash
curl -u admin:admin -X POST -H "content-type:application/json" \
  -d '{"kind":"Issue","apiVersion":"issuetrackerproject.ext.grafana.com/v1alpha1","metadata":{"name":"feature-request","namespace":"default"},"spec":{"title":"New Feature","description":"It would be nice to have dark mode","status":"open"}}' \
  http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues
```

Check that it was NOT flagged:

```bash
curl -u admin:admin http://grafana.k3d.localhost:9999/apis/issuetrackerproject.ext.grafana.com/v1alpha1/namespaces/default/issues/feature-request | jq '.spec.flagged'
# Should output: null or false
```

### View operator logs

```bash
kubectl logs -l name=issuetrackerproject-app-operator -n default -f
```

You should see log entries showing when issues are flagged.

## What You Learned

- **Event-Driven Pattern**: Your operator reacts to changes asynchronously, after the API request completes
- **Watcher Lifecycle**: Add/Update/Delete hooks let you respond to all resource events
- **Operator vs User Fields**: The `flagged` field is set by the operator, not the user
- **Kubernetes Deployment**: Operators run as containers in the cluster, separate from the API server
- **Separation of Concerns**: Validation happens synchronously, automation happens asynchronously

## What's Next?

### Continue Learning: Custom API Tutorial

Add custom endpoints that don't fit the CRUD model:

- **[Custom API Tutorial](../custom-api/)** - Search for similar issues using GitHub API

### Deep Dive: Advanced Operator Concepts

- **[Writing a Reconciler](../../writing-a-reconciler.md)** - More robust alternative to watchers with automatic retries
- **[Admission Control](../../admission-control.md)** - Validation and mutation webhooks for synchronous logic
- **[Operators Guide](../../operators.md)** - Complete guide to operator patterns

### Understand the Architecture

- **[Platform Concepts: Asynchronous Business Logic](../../application-design/platform-concepts.md#asynchronous-business-logic)** - Why async matters
- **[Operator-Based Applications](../../application-design/README.md#operator-based-applications)** - When to use operators

### Working Example

- **[Simple Operator Example](../../../examples/operator/simple/)** - Complete working example with both watcher and reconciler

## Troubleshooting

**Operator pod not starting**: Check the logs with `kubectl get pods -n default` and `kubectl logs <pod-name>`.

**Changes not appearing**: Ensure you ran `make local/push_operator` to update the image in k3d.

**Watcher not triggering**: Verify the watcher is registered in `pkg/app/app.go` (should be auto-generated).

**Field not updating**: Remember that updates to `.spec` trigger new events, so avoid infinite loops. The watcher checks `!issue.Spec.Flagged` before updating.


