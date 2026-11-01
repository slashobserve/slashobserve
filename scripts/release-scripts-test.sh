#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

expect() {
  local name="$1" want="$2" got="$3"
  if [[ "$got" != "$want" ]]; then
    printf 'FAIL  %s\n  want: %q\n  got:  %q\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
}

tags() { "$here/release-tags.sh" "$1" 2>&1 | paste -sd, -; }

expect "stable release" "0.1.0,0.1,latest" "$(tags v0.1.0)"
expect "patch release" "1.12.3,1.12,latest" "$(tags v1.12.3)"
expect "release candidate" "0.1.0-rc.1" "$(tags v0.1.0-rc.1)"
expect "beta" "2.0.0-beta" "$(tags v2.0.0-beta)"
for bad in 0.1.0 v0.1 v01.2.3 v1.2.3+build v1.2.3- vfoo; do
  if "$here/release-tags.sh" "$bad" >/dev/null 2>&1; then
    printf 'FAIL  %s should be rejected\n' "$bad" >&2
    failures=$((failures + 1))
  fi
done

cat >"$work/CHANGELOG.md" <<'MD'
# Changelog

## [Unreleased]

### Added

- Something not shipped.

## [0.1.0] - 2026-11-05

### Added

- First release.
- Second line.

### Fixed

- A fix.

## [0.0.1] - 2026-10-01

- Old.

[Unreleased]: https://example.com/compare/v0.1.0...HEAD
[0.1.0]: https://example.com/releases/tag/v0.1.0
MD

notes() { "$here/release-notes.sh" "$1" "$work/CHANGELOG.md" 2>&1; }

expect "middle section" $'### Added\n\n- First release.\n- Second line.\n\n### Fixed\n\n- A fix.' "$(notes v0.1.0)"
expect "last section stops before link references" "- Old." "$(notes v0.0.1)"
expect "prerelease falls back to its base version" "$(notes v0.1.0)" "$(notes v0.1.0-rc.1)"
expect "prerelease without a base falls back to Unreleased" $'### Added\n\n- Something not shipped.' "$(notes v0.2.0-rc.1)"
expect "missing stable section" "release-notes.sh: no section for 0.2.0 in $work/CHANGELOG.md" "$(notes v0.2.0 || true)"
expect "a version is not matched by its prefix" "release-notes.sh: no section for 0.1 in $work/CHANGELOG.md" "$(notes v0.1 || true)"

if ((failures > 0)); then
  exit 1
fi
echo "release scripts: ok"
