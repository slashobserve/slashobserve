#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_DIR="$ROOT/deploy/compose"
PROJECT="slashobserve-evals"

export SLASHOBSERVE_HOST_PORT="${SLASHOBSERVE_EVALS_PORT:-28080}"
export SLASHOBSERVE_DEX_HOST_PORT="${SLASHOBSERVE_EVALS_DEX_PORT:-25556}"
export SLASHOBSERVE_PROMETHEUS_HOST_PORT="${SLASHOBSERVE_EVALS_PROMETHEUS_PORT:-29090}"
export SLASHOBSERVE_SHOP_HOST_PORT="${SLASHOBSERVE_EVALS_SHOP_PORT:-28081}"
export SLASHOBSERVE_LOKI_HOST_PORT="${SLASHOBSERVE_EVALS_LOKI_PORT:-23100}"
export SLASHOBSERVE_TEMPO_HOST_PORT="${SLASHOBSERVE_EVALS_TEMPO_PORT:-23200}"
export SLASHOBSERVE_EVALS_SHOP1_PORT="${SLASHOBSERVE_EVALS_SHOP1_PORT:-28082}"
export SLASHOBSERVE_EVALS_RUNTIME_DIR="${SLASHOBSERVE_EVALS_RUNTIME_DIR:-/tmp/slashobserve-evals}"
export SLASHOBSERVE_API_CONTRACT_CHECK=false
export SLASHOBSERVE_SERVICE_ACCOUNTS_FILE=
export SLASHOBSERVE_DEMO_MARKERS_URL=

usage() {
  cat <<EOF
Usage: scripts/evals-stack.sh <command>

  up [--k8s] replay   prepare fixtures and series, then boot the isolated ${PROJECT}
                      stack on the replay provider; deploy/compose/.env is not read
  up [--k8s] <env-file>
                      the same, with the provider settings of the named env file
  server [--k8s] <env-file>
                      recreate only the server with another env file, for matrix runs
  down                drop the volumes of ${PROJECT} only, and remove its containers
  status              list the containers of ${PROJECT}

up returns once Prometheus serves what the stack itself guarantees: node_uname_info for the node
and node-exporter (synthetic) jobs, rates of node_cpu_seconds_total (node job) and the shop's
http_requests_total, and the synth exporter.
It waits for rates because they need two scrapes, about 30 seconds after the containers are healthy.
Series that depend on the host kernel, such as node_pressure_*, are never waited for.
With --k8s only the node job and the shop are required, because the synthetic series are empty.

--k8s adds the prom_k8s datasource of the local Kubernetes demo (scripts/demo-k8s.sh up),
joins the server to the cluster's docker network and serves an empty synthetic series file.

Environment:
  SLASHOBSERVE_EVALS_PORT             host port of the server (default 28080)
  SLASHOBSERVE_EVALS_DEX_PORT         host port of Dex (default 25556)
  SLASHOBSERVE_EVALS_PROMETHEUS_PORT  host port of Prometheus (default 29090)
  SLASHOBSERVE_EVALS_SHOP_PORT        host port of the checkout-0 fault API (default 28081)
  SLASHOBSERVE_EVALS_SHOP1_PORT       host port of the checkout-1 fault API (default 28082)
  SLASHOBSERVE_EVALS_RUNTIME_DIR      fixtures and series written by the runner (default /tmp/slashobserve-evals)
  SLASHOBSERVE_IMAGE                  run this prebuilt server image instead of building the checkout
  SLASHOBSERVE_SHOP_IMAGE             the prebuilt demo shop image, required with SLASHOBSERVE_IMAGE

Requirements: docker with compose v2.20+ and go.
EOF
}

die() {
  printf 'evals-stack: %s\n' "$1" >&2
  exit 1
}

if [[ -n "${SLASHOBSERVE_IMAGE:-}" ]]; then
  [[ -n "${SLASHOBSERVE_SHOP_IMAGE:-}" ]] || die "SLASHOBSERVE_SHOP_IMAGE is required with SLASHOBSERVE_IMAGE"
  BUILD=0
else
  export SLASHOBSERVE_IMAGE="slashobserve-evals:local"
  export SLASHOBSERVE_SHOP_IMAGE="slashobserve-evals-shop:local"
  BUILD=1
fi

compose() {
  local env_file="$1"
  shift
  docker compose \
    -f "$COMPOSE_DIR/compose.yaml" \
    -f "$COMPOSE_DIR/compose.evals.yaml" \
    -p "$PROJECT" --env-file "$env_file" "$@"
}

compose_build() {
  local env_file="$1"
  shift
  docker compose \
    -f "$COMPOSE_DIR/compose.yaml" \
    -f "$COMPOSE_DIR/compose.build.yaml" \
    -f "$COMPOSE_DIR/compose.evals.yaml" \
    -p "$PROJECT" --env-file "$env_file" "$@"
}

select_provider() {
  local target="$1"
  if [[ "$target" == "replay" ]]; then
    ENV_FILE=/dev/null
    unset SLASHOBSERVE_LLM_MODEL SLASHOBSERVE_LLM_API_KEY SLASHOBSERVE_LLM_BASE_URL
    export SLASHOBSERVE_LLM_PROVIDER=replay
    return
  fi
  [[ -f "$target" ]] || die "env file not found: $target"
  ENV_FILE="$target"
  unset SLASHOBSERVE_LLM_PROVIDER SLASHOBSERVE_LLM_MODEL SLASHOBSERVE_LLM_API_KEY SLASHOBSERVE_LLM_BASE_URL \
    SLASHOBSERVE_LLM_MAX_TOKENS SLASHOBSERVE_LLM_REQUEST_TIMEOUT SLASHOBSERVE_LLM_EFFORT
}

K8S=0

use_k8s() {
  "$ROOT/scripts/demo-k8s.sh" status >/dev/null || die "the Kubernetes demo is not running; run scripts/demo-k8s.sh up first"
  K8S=1
  export SLASHOBSERVE_DATASOURCES_HOST_FILE=./datasources.evals-k8s.yaml
}

join_cluster() {
  "$ROOT/scripts/demo-k8s.sh" connect "${PROJECT}-server-1"
}

SERIES_TIMEOUT=300
# A retime step is bounded to 2 minutes (retimeMaxStep in evals/runner/tasks.go); a shorter history leaves no point with data.
SYNTH_SCRAPE_SECONDS=15
RETIME_HISTORY_MINUTES=3
SYNTH_HISTORY_SAMPLES=$((RETIME_HISTORY_MINUTES * 60 / SYNTH_SCRAPE_SECONDS))

series_served() {
  curl -fsS --max-time 5 --get "http://127.0.0.1:${SLASHOBSERVE_PROMETHEUS_HOST_PORT}/api/v1/query" --data-urlencode "query=$1" 2>/dev/null |
    grep -q '"result":\[{'
}

wait_for_series() {
  local queries=(
    'count(node_uname_info{job="node"})'
    'count(rate(node_cpu_seconds_total{job="node"}[1m]))'
    'count(rate(http_requests_total[1m]))'
  )
  if [[ "$K8S" == 0 ]]; then
    queries+=(
      'count(node_uname_info{job="node-exporter"})'
      'count(up{job="synth"} == 1)'
      "count_over_time(up{job=\"synth\"}[${RETIME_HISTORY_MINUTES}m]) >= ${SYNTH_HISTORY_SAMPLES}"
    )
  fi
  local deadline=$((SECONDS + SERIES_TIMEOUT)) missing query
  while true; do
    missing=()
    for query in "${queries[@]}"; do
      series_served "$query" || missing+=("$query")
    done
    [[ ${#missing[@]} -eq 0 ]] && return 0
    if ((SECONDS >= deadline)); then
      printf 'evals-stack: Prometheus on port %s did not serve these within %ss:\n' "$SLASHOBSERVE_PROMETHEUS_HOST_PORT" "$SERIES_TIMEOUT" >&2
      printf '  %s\n' "${missing[@]}" >&2
      return 1
    fi
    sleep 2
  done
}

cmd_up() {
  if [[ "${1:-}" == "--k8s" ]]; then
    use_k8s
    shift
  fi
  [[ $# -eq 1 ]] || die "up needs 'replay' or an env file"
  select_provider "$1"
  cd "$ROOT"
  go run ./evals/runner prepare --out "$SLASHOBSERVE_EVALS_RUNTIME_DIR"
  if [[ "$K8S" == 1 ]]; then
    printf '[]\n' >"$SLASHOBSERVE_EVALS_RUNTIME_DIR/synth/series.json"
  fi
  if [[ "$BUILD" == 1 ]]; then
    compose_build "$ENV_FILE" up -d --build --wait --wait-timeout 300
  else
    compose "$ENV_FILE" up -d --no-build --pull missing --wait --wait-timeout 300
  fi
  wait_for_series || die "the stack is up but its metrics are not ready"
  if [[ "$K8S" == 1 ]]; then
    join_cluster
  fi
}

cmd_server() {
  if [[ "${1:-}" == "--k8s" ]]; then
    use_k8s
    shift
  fi
  [[ $# -eq 1 && "$1" != "replay" ]] || die "server needs an env file"
  select_provider "$1"
  compose "$ENV_FILE" up -d --force-recreate --no-deps --wait server
  if [[ "$K8S" == 1 ]]; then
    join_cluster
  fi
}

cmd_down() {
  printf 'evals-stack: dropping the volumes of compose project %s only\n' "$PROJECT"
  compose /dev/null down -v --remove-orphans
}

cmd_status() {
  compose /dev/null ps
}

command="${1:-}"
[[ $# -gt 0 ]] && shift
case "$command" in
  up) cmd_up "$@" ;;
  server) cmd_server "$@" ;;
  down) cmd_down ;;
  status) cmd_status ;;
  -h | --help | help) usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
