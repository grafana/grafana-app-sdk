#!/usr/bin/env sh

set -e

rootdir="$(git rev-parse --show-toplevel)"
testdir="${rootdir}/codegen/testing/golden_generated"

echo "Regenerating golden test files from current codegen output"

# Group by group
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="customManifest"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="testManifest"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  --defencoding=yaml \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="customManifest"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  --defencoding=yaml \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="testManifest"
# Move the manifest files
mv ${testdir}/go/groupbygroup/*.go "${testdir}/manifest/go/"
mv ${testdir}/crd/test-app-manifest.* "${testdir}/manifest/"
mv ${testdir}/crd/custom-app-manifest.* "${testdir}/manifest/"
# Group by kind (only customKind)
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbykind" \
  --defencoding="none" \
  -t="${testdir}/typescript/versioned" \
  --grouping=kind \
  --manifest="customManifest"

# Rename files to append .txt
find "${testdir}" -depth -name "*.go" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.ts" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.json" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.yaml" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;