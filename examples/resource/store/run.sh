#!/usr/bin/env bash

ROOT_DIR=$(git rev-parse --show-toplevel)
CLUSTER_NAME=${1:-"operator-app-sdk-example"}

# Create the cluster
sh "${ROOT_DIR}/examples/create_cluster.sh"

# TODO: run in cluster?
go run store.go --kubecfg="${HOME}/.kube/config"