# Issue Tracker Project

A Grafana App SDK application that provides issue tracking capabilities with custom Kubernetes resources.

## Architecture

This project consists of:

- **Grafana Plugin**: Provides UI and REST endpoints through Grafana
- **Kubernetes Operator**: Watches and manages custom resources in Kubernetes  
- **Custom Resource Definitions (CRDs)**: Define Issue resources in the Kubernetes API
- **API Access**: Direct access to Kubernetes API for programmatic usage

## API Access

Your Issue resources are served by the **Kubernetes API server** as custom resources. Here are the different ways to access them:

### 🎯 Method 1: Using kubectl (Recommended)

This is the simplest method for interacting with your Issue resources:

```bash
# List all issues
kubectl get issues

# Get detailed JSON output
kubectl get issues -o json

# Get a specific issue
kubectl get issue test-issue-1 -o yaml

# Create an issue from a YAML file
kubectl apply -f sample-issue.yaml

# Delete an issue  
kubectl delete issue test-issue-1

# Watch for changes in real-time
kubectl get issues -w
```

### 🌐 Method 2: HTTP API via kubectl proxy

For HTTP-based access without dealing with authentication:

```bash
# Start kubectl proxy (runs in background)
kubectl proxy --port=8080 &

# List all issues
curl http://localhost:8080/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues

# Get a specific issue
curl http://localhost:8080/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues/test-issue-1

# Create an issue (POST request)
curl -X POST http://localhost:8080/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "issuetrackerproject.ext.grafana.com/v1",
    "kind": "Issue",
    "metadata": {
      "name": "api-created-issue",
      "namespace": "default"
    },
    "spec": {
      "title": "Issue created via API",
      "description": "This issue was created using the REST API",
      "status": "open"
    }
  }'

# Update an issue (PUT request)
curl -X PUT http://localhost:8080/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues/api-created-issue \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "issuetrackerproject.ext.grafana.com/v1",
    "kind": "Issue",
    "metadata": {
      "name": "api-created-issue",
      "namespace": "default"
    },
    "spec": {
      "title": "Updated Issue Title",
      "description": "This issue was updated via API",
      "status": "in-progress"
    }
  }'

# Delete an issue (DELETE request)
curl -X DELETE http://localhost:8080/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues/api-created-issue

# Stop the proxy when done
pkill kubectl
```

### 🔐 Method 3: Direct HTTPS API (with authentication)

For direct access to the Kubernetes API server:

```bash
# Get the API server URL
APISERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

# Get authentication token
TOKEN=$(kubectl config view --raw -o jsonpath='{.users[0].user.token}')

# Make authenticated requests
curl -k -H "Authorization: Bearer $TOKEN" \
  $APISERVER/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/default/issues
```

## Sample Issue Resource

Here's an example Issue resource you can create:

```yaml
# sample-issue.yaml
apiVersion: issuetrackerproject.ext.grafana.com/v1
kind: Issue
metadata:
  name: sample-issue
  namespace: default
spec:
  title: "Bug: Application crashes on startup"
  description: "The application crashes when starting in production environment. Stack trace shows null pointer exception in main method."
  status: "open"
```

Create it with:
```bash
kubectl apply -f sample-issue.yaml
```

## Issue Resource Schema

Each Issue resource has the following structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.title` | string | ✅ | The title of the issue |
| `spec.description` | string | ✅ | Detailed description of the issue |
| `spec.status` | string | ✅ | Current status (e.g., "open", "in-progress", "closed") |

## API Endpoints

All endpoints are served under the Kubernetes API:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/{namespace}/issues` | List all issues in namespace |
| `GET` | `/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/{namespace}/issues/{name}` | Get specific issue |
| `POST` | `/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/{namespace}/issues` | Create new issue |
| `PUT` | `/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/{namespace}/issues/{name}` | Update existing issue |
| `DELETE` | `/apis/issuetrackerproject.ext.grafana.com/v1/namespaces/{namespace}/issues/{name}` | Delete issue |

## Troubleshooting

### ❌ Common Mistake: Wrong API Endpoint

**Don't use**: `http://grafana.k3d.localhost:9999/apis/...`  
**Use instead**: The Kubernetes API server directly (methods above)

The Grafana aggregated API endpoint (`grafana.k3d.localhost:9999`) is for external API servers, not for standard Kubernetes CRDs.

### ✅ Verify Your Setup

```bash
# Check if CRD is installed
kubectl get crd | grep issue

# Check if API service is registered  
kubectl get apiservices | grep issuetrackerproject

# Check if operator is running
kubectl get pods | grep issuetrackerproject-app-operator

# Check if issues exist
kubectl get issues
```

### 📝 Enable Debug Logging

If you need more detailed logs from the operator:

```bash
# Check operator logs
kubectl logs deployment/issuetrackerproject-app-operator -f
```

## Development

To modify the API schema:

1. Edit `kinds/issue.cue`
2. Run `grafana-app-sdk generate`
3. Apply the updated CRDs: `kubectl apply -f definitions/`
4. Rebuild and redeploy the operator

## Next Steps

- Access the Grafana UI at `http://grafana.k3d.localhost:9999`
- Create issues through the plugin interface
- Monitor operator behavior through logs
- Build additional functionality using the API

---

For more information, see the [Grafana App SDK documentation](https://github.com/grafana/grafana-app-sdk). 
