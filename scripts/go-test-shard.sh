#!/usr/bin/env bash
set -euo pipefail

named="grafana db datasource agent"

rest=4

weights="internal/server/mcp=104 internal/revisions=80 internal/inboundmcp=79 internal/audit=58 internal/observations=58 internal/threads=49 internal/dashboards=38 internal/runsession=38 internal/findings=37 cmd/slashobserve-server=36 internal/datasourceadmin=35 internal/activity=30 internal/events/stream=27 internal/auth=23 internal/mcpauth=21 internal/grafanaconn=21 internal/public=20 internal/placement=20 internal/search=19 internal/webhooks=17 internal/auditfolder=16 internal/folders=16 internal/claims=15 internal/imports=15 internal/proposals=15 internal/agentsview=14 internal/health=13 internal/queryproxy/proxyhttp=12 internal/provision=12 internal/surface=12 internal/repairs=11 internal/savedsearches=11 internal/audit/trail=11 internal/users=11 internal/cli=8"

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
  echo "$named $(seq -f 'rest-%g' -s ' ' 1 "$rest")"
elif [[ "$shard" =~ ^rest-([1-9][0-9]*)$ ]] && ((BASH_REMATCH[1] <= rest)); then
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
    awk -v want="${shard#rest-}" -v n="$rest" '
      { target = 1; for (i = 2; i <= n; i++) if (load[i] < load[target]) target = i
        load[target] += $1; if (target == want) printf "%s ", $2 }
      END { printf "\n" }'
elif ! packages "$shard"; then
  echo "usage: $0 ${named// /|}|rest-1..rest-$rest|shards" >&2
  exit 2
fi
