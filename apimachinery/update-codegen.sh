#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(dirname "${BASH_SOURCE[0]}")" # this is the location of update-codegen.sh (the directory that holds it, not the file)
(cd "$SCRIPT_ROOT"/codegen && GO111MODULE=on go mod download) # ensure we download the dependencies
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "$SCRIPT_ROOT"/codegen && echo "$(go env GOPATH)"/pkg/mod/k8s.io/code-generator@v0.32.0)} # ensure we know where kube stuff is
source "${CODEGEN_PKG}/kube_codegen.sh" # makes kube:: functions available

pkg="${SCRIPT_ROOT}/apis/common"
vpkg="${pkg}/v0alpha1"
find "${vpkg}" -type f -iname "zz_generated*" -exec rm {} +

kube::codegen::gen_helpers --boilerplate "$(mktemp)" "${pkg}"

violations_file="${vpkg}/zz_generated.openapi_violation_exceptions.list"
go run k8s.io/kube-openapi/cmd/openapi-gen \
  --output-file zz_generated.openapi.go \
  --output-dir "${vpkg}" \
  --output-pkg "github.com/grafana/grafana-app-sdk/apimachinery/apis/common/v0alpha1" \
  --report-filename "${violations_file}" \
  k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/version "${vpkg}"
# we don't want an empty file lying around
if [ -f "${violations_file}" ] && ! grep -q . "${violations_file}"; then
  echo "Deleting ${violations_file} because it is empty"
  rm "${violations_file}"
fi