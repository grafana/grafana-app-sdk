#!/usr/bin/env bash

set -e

echo 'Making sure we are on top of the main branch...'
git checkout main
git pull
# Release tags live on detached release commits (not reachable from main), so pull won't fetch them.
git fetch --tags origin

if [[ $(git rev-list --count main...origin/main) -ne 0 ]]; then
  echo 'Your main branch is ahead of origin, bailing out!'
  exit 1
fi

if [[ -n $(git status --porcelain) ]]; then
  echo 'Working tree is not clean, bailing out!'
  exit 1
fi

echo 'Making sure the build is OK...'
make

if ! which svu > /dev/null; then
  echo 'Installing github.com/caarlos0/svu@latest...'
  go install github.com/caarlos0/svu@latest
fi

# Release commits are tagged on a detached HEAD, so they are not reachable from main;
# --tag-mode all-branches makes svu see them.
TAG=$(svu current --tag-mode all-branches)
echo "Latest release tag is '${TAG}', calculating the next one..."

RELEASE=$1

case "${RELEASE}" in
patch)
  TAG=$(svu patch --tag-mode all-branches)
  ;;
minor)
  TAG=$(svu minor --tag-mode all-branches)
  ;;
major)
  TAG=$(svu major --tag-mode all-branches)
  ;;
*)
  echo "$0 {patch | minor | major}" >&2
  exit 1
  ;;
esac

echo "Making a '${RELEASE}' release with tag '${TAG}'..."

LOGGING_MOD="github.com/grafana/grafana-app-sdk/logging"

# Always return to main, even if something below fails
trap 'git checkout --quiet main' EXIT

# The logging tag must exist before the SDK tag: the SDK's go.mod at the release tag
# requires logging at this same version, so consumers can resolve both with one `go get`.
echo "Creating logging submodule release"
git tag -s "logging/${TAG}" -m "Logging submodule release ${TAG}"
git push origin "logging/${TAG}"

# main keeps a `logging v0.0.0` placeholder plus a local `replace` (for workspace development).
# Consumers ignore `replace` directives, so the tagged commit must reference the real version.
# Create a release commit on a detached HEAD (not pushed to any branch) with that rewrite.
echo "Creating release commit pinning logging to ${TAG}..."
git checkout --quiet --detach
go mod edit -dropreplace="${LOGGING_MOD}" -require="${LOGGING_MOD}@${TAG}"
# GOPROXY=direct avoids proxy lag for the tag we just pushed
GOWORK=off GOFLAGS=-mod=mod GOPROXY=direct go mod tidy

echo "Verifying logging resolves to ${TAG} outside of the workspace..."
RESOLVED=$(GOWORK=off GOPROXY=direct go list -m -f '{{.Version}}' "${LOGGING_MOD}")
if [ "${RESOLVED}" != "${TAG}" ]; then
  echo "Expected ${LOGGING_MOD} at ${TAG}, got '${RESOLVED}'. Bailing out!"
  echo "(logging/${TAG} was already pushed; delete it if you want to retry)"
  exit 1
fi

git commit -a -m "Release ${TAG}"

git tag -s "${TAG}" -m "Release ${TAG}"
git push origin "${TAG}"

echo "Returning to main branch"
git checkout main

exit 0
