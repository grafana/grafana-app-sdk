#!/usr/bin/env bash

ROOT_DIR=$(git rev-parse --show-toplevel)
CLUSTER_NAME=${1:-"operator-app-sdk-example"}

# Create the cluster
sh "${ROOT_DIR}/examples/create_cluster.sh"

# Not needed, custom resource definitions can be added in-code
#kubectl --context="k3d-${CLUSTER_NAME}" apply -f "${ROOT_DIR}/examples/operator/opinionated/opinionated.yaml"

# TODO: run in cluster?
go run opinionated.go --kubecfg="${HOME}/.kube/config"
