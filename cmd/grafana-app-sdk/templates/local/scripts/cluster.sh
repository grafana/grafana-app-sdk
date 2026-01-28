#!/usr/bin/env bash
set -eufo pipefail

ctlptl_config=${1:-ctlptl-config.yaml}

create_cluster() {
  kind_bin=$(which kind)
  ctlptl_bin=$(which ctlptl)
  if [[ $kind_bin == "" ]]; then
    echo "kind (https://kind.sigs.k8s.io/) is required to run the local development environment."
    exit 1
  fi
  if [[ $ctlptl_bin == "" ]]; then
    echo "ctlptl (https://github.com/tilt-dev/ctlptl)is not installed, falling back to shell script."
    sh ./scripts/kind-cluster.sh "kind-{{.ClusterName}}"
  else
    ctlptl apply -f ${ctlptl_config}
  fi
}

delete_cluster() {
  kind_bin=$(which kind)
  ctlptl_bin=$(which ctlptl)
  if [[ $kind_bin == "" ]]; then
    echo "kind (https://kind.sigs.k8s.io/) is required to run the local development environment."
    exit 1
  fi
  if [[ $ctlptl_bin == "" ]]; then
    kind cluster delete --name="kind-{{.ClusterName}}"
  else
    ctlptl delete -f ${ctlptl_config}
  fi
}

if [ $# -lt 1 ]; then
  echo "Usage: ./cluster.sh [create|delete]"
  exit 1
fi

if [ $1 == "create" ]; then
  create_cluster $2
elif [ $1 == "delete" ]; then
  delete_cluster
else
  echo "Unknown argument ${1}"
fi