#!/usr/bin/env bash
# Creates a k3d cluster with all volume mounts required for testing the apps.

set -eufo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel)
CLUSTER_NAME="operator-app-sdk-example"

K3D_MAJOR_VERSION="$(k3d --version | head -n1 | grep -E -o "v([0-9])" | grep -E -o "[0-9]")"

if ! k3d cluster list "${CLUSTER_NAME}" >/dev/null 2>&1; then
  echo "Creating cluster"

  # Array of extra options to add to the k3d cluster create command
  EXTRA_K3D_OPTS=()

  # Bug in k3d for btrfs filesystems workaround, see https://k3d.io/v5.2.2/faq/faq/#issues-with-btrfs
  ROOTFS="$(stat -f --format="%T" "/")"
  if [[ "${ROOTFS}" == "btrfs" ]]; then
    EXTRA_K3D_OPTS+=("-v" "/dev/mapper:/dev/mapper")
  fi

  k3d cluster create "${CLUSTER_NAME}" ${EXTRA_K3D_OPTS[@]+"${EXTRA_K3D_OPTS[@]}"} \
    --api-port "0.0.0.0:6443" \
    -p "8080:80@loadbalancer" \
    --volume "${ROOT_DIR}"/examples/plugin/testdata/grafana/plugins/operator-app-sdk-plugin-example:/tmp/k3d/operator-app-sdk-plugin-example@server:* \
    --volume "${ROOT_DIR}"/examples/plugin/testdata/grafana/kubeconfig:/tmp/k3d/kubeconfig@server:*
else
  echo "Cluster already exists"
fi

K3D_KUBECONFIG=$(k3d kubeconfig write "${CLUSTER_NAME}")
export KUBECONFIG=${K3D_KUBECONFIG}
kubectl config use-context "k3d-${CLUSTER_NAME}"