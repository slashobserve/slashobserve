#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: release-tags.sh vX.Y.Z[-prerelease]}"
pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'

if [[ ! "$tag" =~ $pattern ]]; then
  echo "release-tags.sh: $tag is not a vMAJOR.MINOR.PATCH[-prerelease] tag" >&2
  exit 1
fi

version="${tag#v}"
echo "$version"
if [[ -z "${BASH_REMATCH[4]}" ]]; then
  echo "${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
  echo "latest"
fi
