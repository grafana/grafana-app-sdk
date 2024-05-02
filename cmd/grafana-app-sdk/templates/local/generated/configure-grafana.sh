#!/usr/bin/env bash
set -eufo pipefail

GRAFANA_RUN_MODE="${1}"
GRAFANA_HOST="grafana.k3d.localhost:{{.Port}}"

if [ $GRAFANA_RUN_MODE == "in_cluster" ]; then
  KUBEAPI=($(kubectl run bash-local-env -it --rm --image=bash --restart=Never --command -- "bash" "-c" 'echo "$KUBERNETES_SERVICE_HOST $KUBERNETES_SERVICE_PORT"'))
elif [ $GRAFANA_RUN_MODE == "standalone" ]; then
  CONTROLPLANE=($(kubectl cluster-info | grep "control plane"))
  ENDPOINT=$(echo ${CONTROLPLANE[${#CONTROLPLANE[@]}-1]} | sed 's~http[s]*://~~g')
  IFS=: read -r -a KUBEAPI <<< "$ENDPOINT"
  read -p "Grafana API Server URL (host:port): " HOST
fi
HOST=${KUBEAPI[0]}
PORT=${KUBEAPI[1]}

{{ range .CRDs }}{{$crd:=.}}{{range .Versions}}curl \
  "http://${GRAFANA_HOST}/apis/apiregistration.k8s.io/v1/apiservices?fieldManager=kubectl-create&fieldValidation=Strict" \
  --request POST \
  --header "Content-Type: application/json" \
  --data @- << EOF
{
  "apiVersion": "apiregistration.k8s.io/v1",
  "kind": "APIService",
  "metadata": {
    "name": "{{.}}.{{$crd.Group}}"
  },
  "spec": {
    "version": "{{.}}",
    "insecureSkipTLSVerify": true,
    "group": "{{$crd.Group}}",
    "groupPriorityMinimum": 1000,
    "versionPriority": 15,
    "service": {
      "name": "example-apiserver",
      "namespace": "default",
      "port": ${PORT}
    }
  }
}
EOF
{{ end }}{{end}}

curl \
  "http://${GRAFANA_HOST}/apis/service.grafana.app/v0alpha1/namespaces/default/externalnames?fieldManager=kubectl-create&fieldValidation=Strict" \
  --request POST \
  --header "Content-Type: application/json" \
  --data @- << EOF
{
  "apiVersion": "service.grafana.app/v0alpha1",
  "kind": "ExternalName",
  "metadata": {
    "name": "example-apiserver",
    "namespace": "default"
  },
  "spec": {
    "host": "${HOST}"
  }
}
EOF