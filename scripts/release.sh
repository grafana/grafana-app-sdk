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

echo "Tagging submodules with the release tag"
git tag -s "plugin/${TAG}" -m "Plugin submodule release ${TAG}"
git push origin "plugin/${TAG}"

exit 0
