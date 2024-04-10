#!/usr/bin/env bash

ROOT_DIR=$(git rev-parse --show-toplevel)
CLUSTER_NAME=${2:-"operator-app-sdk-example"}

# Create the cluster
sh "${ROOT_DIR}/examples/create_cluster.sh"

# Not needed, custom resource definitions can be added in-code
#kubectl --context="k3d-${CLUSTER_NAME}" apply -f "${ROOT_DIR}/examples/operator/opinionated/opinionated.yaml"

# TODO: run in cluster?
case "$1" in
  watcher)
    go run watcher.go opinionated.go --kubecfg="${HOME}/.kube/config"
    ;;
  reconciler)
    go run reconciler.go opinionated.go --kubecfg="${HOME}/.kube/config"
    ;;
  *)
    echo "unknown option: $1" >&2
    exit 2
    ;;
esac
