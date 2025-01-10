#!/usr/bin/env sh

set -e

echo 'Making sure we are on top of the main branch...'
git checkout main
git pull

if [[ $(git rev-list --count main...origin/main) -ne 0 ]]; then
  echo 'Your main branch is ahead of origin, bailing out!'
  exit 1
fi

echo 'Making sure the build is OK...'
make

if ! which svu > /dev/null; then
  echo 'Installing github.com/caarlos0/svu@latest...'
  go install github.com/caarlos0/svu@latest
fi

TAG=$(svu current)
echo "Latest release tag is '${TAG}', calculating the next one..."

RELEASE=$1

case "${RELEASE}" in
patch)
  TAG=$(svu patch)
  ;;
minor)
  TAG=$(svu minor)
  ;;
major)
  TAG=$(svu major)
  ;;
*)
  echo "$0 {patch | minor | major}" >&2
  exit 1
  ;;
esac

echo "Making a '${RELEASE}' release with tag '${TAG}'..."

git tag -s "${TAG}" -m "Release ${TAG}"
git push origin "${TAG}"

echo "Creating logging submodule release"
git tag -s "logging/${TAG}" -m "Logging submodule release ${TAG}"
git push origin "logging/${TAG}"

echo "Updating the plugin module to use the new release for the main module"
git checkout -b "plugin-release/${TAG}"
cd $(git rev-parse --show-toplevel)/plugin
go get github.com/grafana/grafana-app-sdk@${TAG}
git add go.mod go.sum
git commit -m "Plugin module release branch ${TAG}"
git push -u origin "plugin-release/${TAG}"

echo "Tagging submodule with the release tag"
git tag -s "plugin/${TAG}" -m "Plugin submodule release ${TAG}"
git push origin "plugin/${TAG}"

echo "Returning to main branch"
git checkout main

exit 0
