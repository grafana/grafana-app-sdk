#!/usr/bin/env bash
set -eufo pipefail

# Script to replicate ctlptl apply -f ctlptl-config.yaml
# This script creates a local registry and kind cluster with the same configuration
# It is used as a backup if ctlptl isn't installed

REGISTRY_NAME="kind-registry"
REGISTRY_PORT=5000
CLUSTER_NAME=${1:-"kind"}
HOST_PORT_80=9999
HOST_PORT_443=9443
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"\

# Step 1: Create the registry container if it doesn't exist
echo "Setting up local registry..."
if ! docker ps -a --format '{{.Names}}' | grep -q "^${REGISTRY_NAME}$"; then
    echo "Creating registry container ${REGISTRY_NAME}..."
    docker run -d \
        --restart=always \
        -p "127.0.0.1:${REGISTRY_PORT}:5000" \
        --name "${REGISTRY_NAME}" \
        registry:2
else
    echo "Registry container ${REGISTRY_NAME} already exists"
    # Make sure it's running
    if ! docker ps --format '{{.Names}}' | grep -q "^${REGISTRY_NAME}$"; then
        echo "Starting registry container..."
        docker start "${REGISTRY_NAME}"
    fi
fi

# Step 2: Create kind cluster if it doesn't exist
echo "Setting up kind cluster..."
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster ${CLUSTER_NAME} already exists"
else
    echo "Creating kind cluster ${CLUSTER_NAME}..."
    
    # Create a temporary kind config file
    KIND_CONFIG=$(mktemp)
    cat > "$KIND_CONFIG" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  - |
    kind: ClusterConfiguration
    containerdConfigPatches:
    - |-
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${REGISTRY_PORT}"]
        endpoint = ["http://${REGISTRY_NAME}:5000"]
  extraPortMappings:
  - containerPort: 80
    hostPort: ${HOST_PORT_80}
    protocol: TCP
  - containerPort: 443
    hostPort: ${HOST_PORT_443}
    protocol: TCP
  extraMounts:
  - hostPath: ${PROJECT_ROOT}/local/mounted-files
    containerPath: /tmp/k3d/mounted-files
EOF

    kind create cluster --config "$KIND_CONFIG"
    rm "$KIND_CONFIG"
fi

# Step 3: Connect registry to kind network
echo "Connecting registry to kind network..."
if docker network inspect kind >/dev/null 2>&1; then
    # Check if registry is already connected
    if ! docker network inspect kind | grep -q "\"${REGISTRY_NAME}\""; then
        echo "Connecting ${REGISTRY_NAME} to kind network..."
        docker network connect kind "${REGISTRY_NAME}" || true
    else
        echo "Registry ${REGISTRY_NAME} is already connected to kind network"
    fi
else
    echo "Warning: kind network not found. Registry connection will happen automatically when cluster is created."
fi

# Step 4: Apply local registry hosting ConfigMap (for tooling discovery)
echo "Configuring local registry discovery..."
kubectl apply -f - <<EOF || true
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

echo ""
echo "Setup complete!"
echo "Registry: ${REGISTRY_NAME} running on localhost:${REGISTRY_PORT}"
echo "Cluster: ${CLUSTER_NAME}"
echo ""
echo "To use the registry:"
echo "  docker tag <image> localhost:${REGISTRY_PORT}/<image>"
echo "  docker push localhost:${REGISTRY_PORT}/<image>"
