#!/usr/bin/env bash
set -euo pipefail

named="grafana evals db datasource agent"

packages() {
  case "$1" in
    grafana) echo "./pkg/grafana/..." ;;
    evals) echo "./evals/..." ;;
    db) echo "./internal/db/..." ;;
    datasource) echo "./internal/datasource/..." ;;
    agent) echo "./internal/agent/... ./pkg/spec/..." ;;
    *) return 1 ;;
  esac
}

shard="${1:-}"
if [[ "$shard" == rest ]]; then
  taken=()
  for name in $named; do
    read -r -a patterns <<<"$(packages "$name")"
    taken+=("${patterns[@]}")
  done
  go list ./... | grep -v -x -F -f <(go list "${taken[@]}") | tr '\n' ' '
elif ! packages "$shard"; then
  echo "usage: $0 ${named// /|}|rest" >&2
  exit 2
fi
