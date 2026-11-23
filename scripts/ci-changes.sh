#!/usr/bin/env bash
set -euo pipefail

GO_READS_FROM_WEB=(web/src/spec/defaults.gen.ts observe-agent/src/protocol.ts)

usage() {
  cat >&2 <<USAGE
Usage: scripts/ci-changes.sh [all|audit-only|go-reads|draft]

Reads changed file paths on stdin, one per line, and prints which CI job groups
they need as name=true|false lines: go, web, build, stack, image, audit.
  all         print every group as true without reading stdin
  audit-only  print only audit as true without reading stdin
  go-reads    print the web and agent files that Go tests read
  draft       read stdin as usual but leave stack false, for a draft pull request
USAGE
  exit 2
}

matches_any() {
  local path="$1" pattern
  shift
  for pattern in "$@"; do
    # shellcheck disable=SC2053
    [[ "$path" == $pattern ]] && return 0
  done
  return 1
}

is_prose() {
  matches_any "$1" 'docs/*' && return 0
  [[ "$1" == *.md ]] || return 1
  ! matches_any "$1" 'internal/*' 'pkg/*' 'web/*' 'observe-agent/*' 'e2e/*' 'spec/*' 'demo/*'
}

is_unit_test() {
  matches_any "$1" '*_test.go' 'web/src/*.test.ts' 'web/src/*.test.tsx' 'observe-agent/*.test.ts'
}

emit() {
  local name
  for name in go web build stack image audit; do
    printf '%s=%s\n' "$name" "${!name}"
  done
}

skip_compose_jobs=false

case "${1:-}" in
  "") ;;
  draft) skip_compose_jobs=true ;;
  all)
    go=true web=true build=true stack=true image=true audit=true
    emit
    exit 0
    ;;
  audit-only)
    go=false web=false build=false stack=false image=false audit=true
    emit
    exit 0
    ;;
  go-reads)
    printf '%s\n' "${GO_READS_FROM_WEB[@]}"
    exit 0
    ;;
  *) usage ;;
esac

go=false web=false build=false stack=false image=false audit=false

while IFS= read -r path; do
  [[ -n "$path" ]] || continue

  if matches_any "$path" '.github/*' Makefile scripts/ci-changes.sh scripts/go-test-shard.sh; then
    go=true web=true build=true stack=true image=true audit=true
    continue
  fi

  if ! matches_any "$path" 'web/*' 'observe-agent/*' || matches_any "$path" "${GO_READS_FROM_WEB[@]}"; then
    go=true
  fi

  if ! { matches_any "$path" '*.go' go.mod go.sum 'demo/go.*' 'tools/*' || is_prose "$path"; }; then
    web=true
  fi

  if ! is_prose "$path"; then
    build=true
    is_unit_test "$path" || stack=true
  fi

  if matches_any "$path" Dockerfile .dockerignore; then
    image=true
  fi

  if matches_any "$path" go.mod go.sum 'demo/go.*' 'tools/*' web/package.json web/bun.lock \
    observe-agent/package.json observe-agent/bun.lock; then
    audit=true
  fi
done

if [[ "$skip_compose_jobs" == true ]]; then
  stack=false
fi

emit
