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
  --selectors="customKind,testKind,testKind2"
# Group by kind (only customKind)
go run ./cmd/grafana-app-sdk/*.go generate -c="${rootdir}/codegen/cuekind/testing" \
  -g="${testdir}/go/groupbykind" \
  --crdpath="${testdir}/crd" \
  -t="${testdir}/typescript/versioned" \
  --kindgrouping=kind --nomanifest --notypeinpath \
  --selectors="customKind"
# Thema is deprecated, so re-generating the "unversioned" files is disabled and should be done by hand until thema is removed
# Since the tests try to check compliance with "unversioned" for both CUE and Thema, but you can't generated unversioned
# CUE output from the grafana-app-sdk command, we're leaving it alone until it is removed in a future release.

# Rename files to append .txt
find "${testdir}" -depth -name "*.go" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.ts" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.json" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;
find "${testdir}" -depth -name "*.yaml" -exec sh -c 'mv "$1" "${1}.txt"' _ {} \;