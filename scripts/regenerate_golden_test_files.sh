#!/usr/bin/env sh

set -e

rootdir="$(git rev-parse --show-toplevel)"
testdir="${rootdir}/codegen/testing/golden_generated"

echo "Regenerating golden test files from current codegen output"

# Group by group
go run ./cmd/grafana-app-sdk/*.go generate -c="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --crdpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --kindgrouping=group --nomanifest \
  --selectors="customManifest,testManifest"
go run ./cmd/grafana-app-sdk/*.go generate -c="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --crdpath="${testdir}/crd" \
  --crdencoding=yaml \
  -t="${testdir}/typescript/versioned" \
  --kindgrouping=group \
  --selectors="customManifest,testManifest"
# Move the manifest files
mv "${testdir}/go/groupbygroup/testapp_manifest.go" "${testdir}/manifest/go/testkinds/testapp_manifest.go.txt"
mv "${testdir}/go/groupbygroup/customapp_manifest.go" "${testdir}/manifest/go/testkinds/customapp_manifest.go.txt"
mv "${testdir}/crd/test-app-manifest.yaml" "${testdir}/manifest/test-app-manifest.yaml"
mv "${testdir}/crd/custom-app-manifest.yaml" "${testdir}/manifest/custom-app-manifest.yaml"
# Group by kind (only customKind)
go run ./cmd/grafana-app-sdk/*.go generate -c="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbykind" \
  --crdpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --kindgrouping=kind --nomanifest --notypeinpath \
  --selectors="customManifest"

# Rename files to append .txt
find "${testdir}" -depth -name "*.go" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.ts" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.json" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.yaml" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;