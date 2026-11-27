#!/usr/bin/env bash
set -euo pipefail

named="grafana db datasource agent"

weights="internal/cli=8 internal/runsession=4 internal/server/mcp=4 internal/activity=3 internal/audit=3 internal/auth=3 internal/findings=3 internal/mcpauth=2 internal/config=2 internal/folders=2 internal/llm/providers=2 internal/datasourceadmin=2 internal/public=2 internal/imports=2 internal/proposals=2"

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
  module="$(go list -m)"
  grep -v -x -F -f <(echo "$named_packages") <<<"$all" |
    awk -v module="$module/" -v weights="$weights" '
      BEGIN {
        n = split(weights, pairs, " ")
        for (i = 1; i <= n; i++) {
          split(pairs[i], kv, "=")
          weight[module kv[1]] = kv[2]
        }
      }
      { print ($0 in weight ? weight[$0] : 1), $0 }' |
    LC_ALL=C sort -k1,1nr -k2,2 |
    awk -v want="${shard#rest-}" '
      { target = (load[1] <= load[2]) ? 1 : 2; load[target] += $1; if (target == want) printf "%s ", $2 }
      END { printf "\n" }'
elif ! packages "$shard"; then
  echo "usage: $0 ${named// /|}|rest-1|rest-2|shards" >&2
  exit 2
fi
