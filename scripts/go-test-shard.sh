#!/usr/bin/env bash
set -euo pipefail

named="grafana db datasource agent"

packages() {
  case "$1" in
    grafana) echo "./pkg/grafana/..." ;;
    db) echo "./internal/db/..." ;;
    datasource) echo "./internal/datasource/..." ;;
    agent) echo "./internal/agent/... ./pkg/spec/..." ;;
    *) return 1 ;;
  esac
}

shard="${1:-}"
if [[ "$shard" == shards ]]; then
  echo "$named rest-1 rest-2"
elif [[ "$shard" == rest-1 || "$shard" == rest-2 ]]; then
  taken=()
  for name in $named; do
    read -r -a patterns <<<"$(packages "$name")"
    taken+=("${patterns[@]}")
  done
  all="$(go list ./...)"
  named_packages="$(go list "${taken[@]}")"
  grep -v -x -F -f <(echo "$named_packages") <<<"$all" | awk -v half="${shard#rest-}" 'NR % 2 == half % 2' | tr '\n' ' '
elif ! packages "$shard"; then
  echo "usage: $0 ${named// /|}|rest-1|rest-2|shards" >&2
  exit 2
fi
