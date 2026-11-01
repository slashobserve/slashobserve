#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: release-notes.sh vX.Y.Z[-prerelease] [CHANGELOG.md]}"
changelog="${2:-CHANGELOG.md}"
version="${tag#v}"

section() {
  awk -v heading="$1" '
    BEGIN { prefix = "## [" heading "]" }
    /^\[[^]]+\]: / { exit }
    index($0, prefix) == 1 { found = 1; next }
    found && /^## \[/ { exit }
    found { print }
  ' "$changelog" | awk 'NF { started = 1 } started { lines[++n] = $0 } END { while (n > 0 && lines[n] !~ /[^[:space:]]/) n--; for (i = 1; i <= n; i++) print lines[i] }'
}

notes="$(section "$version")"
if [[ -z "$notes" && "$version" == *-* ]]; then
  notes="$(section "${version%%-*}")"
  [[ -n "$notes" ]] || notes="$(section Unreleased)"
fi

if [[ -z "$notes" ]]; then
  echo "release-notes.sh: no section for $version in $changelog" >&2
  exit 1
fi
printf '%s\n' "$notes"
