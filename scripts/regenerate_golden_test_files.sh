#!/usr/bin/env sh

set -e

rootdir="$(git rev-parse --show-toplevel)"
testdir="${rootdir}/codegen/testing/golden_generated"
gomod="codegen-tests"

echo "Regenerating golden test files from current codegen output"

# Group by group
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="customManifest" \
  --gomodule="${gomod}" \
  --crdmanifest \
  --gomodgenpath="pkg/generated"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="testManifest" \
  --gomodule="${gomod}" \
  --crdmanifest \
  --gomodgenpath="pkg/generated"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  --defencoding=yaml \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="customManifest" \
  --gomodule="${gomod}" \
  --crdmanifest \
  --gomodgenpath="pkg/generated"
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbygroup" \
  --defpath="${testdir}/crd" \
  --defencoding=yaml \
  -t="${testdir}/typescript/versioned" \
  --grouping=group \
  --manifest="testManifest" \
  --gomodule="${gomod}" \
  --crdmanifest \
  --gomodgenpath="pkg/generated"
# Move the manifest files
echo "Moving generated Manifest files to ${testdir}/manifest/"
mv ${testdir}/crd/test-app-manifest.* "${testdir}/manifest/"
mv ${testdir}/crd/custom-app-manifest.* "${testdir}/manifest/"
# Group by kind (only customKind)
go run ./cmd/grafana-app-sdk/*.go generate -s="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbykind" \
  --defencoding="none" \
  -t="${testdir}/typescript/versioned" \
  --grouping=kind \
  --manifest="customManifest" \
  --gomodule="${gomod}" \
  --crdmanifest \
  --gomodgenpath="pkg/generated"

# Rename files to append .txt
find "${testdir}" -depth -name "*.go" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.ts" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.json" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.yaml" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;