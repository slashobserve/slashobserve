#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_DIR="$ROOT/deploy/compose"
PROJECT="slashobserve-evals"
WARM_MAX_AGE=1800
WARM_WAIT=600

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

  warm [--k8s] replay|<env-file>
                      prepare fixtures and series and start every service except the server, without
                      waiting for health, then return; 'up' adopts it within ${WARM_MAX_AGE}s and skips
                      prepare, so the synthetic history accumulates while the server image is built;
                      it needs the shop image, not the server image
  up [--k8s] [--no-history] replay
                      prepare fixtures and series, then boot the isolated ${PROJECT}
                      stack on the replay provider; deploy/compose/.env is not read
  up [--k8s] [--no-history] <env-file>
                      the same, with the provider settings of the named env file
  wait-history [--k8s]
                      wait until the synthetic series hold the history a retime step needs;
                      what 'up' without --no-history already waited for
  server [--k8s] <env-file>
                      recreate only the server with another env file, for matrix runs
  down                drop the volumes of ${PROJECT} only, and remove its containers
  status              list the containers of ${PROJECT}

up returns once Prometheus serves what the stack itself guarantees: node_uname_info for the node
and node-exporter (synthetic) jobs, rates of node_cpu_seconds_total (node job) and the shop's
http_requests_total, the synth exporter, and the synth samples of the history a retime step needs.
It waits for rates because they need two scrapes, about 30 seconds after the containers are healthy.
That history counts from Prometheus starting to scrape, so it is the last thing up waits for.
--no-history returns without them; wait-history then blocks until they are there, so work that
does not read them can run while they accumulate.
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

SERIES_TIMEOUT_SLACK=120
SYNTH_SCRAPE_SECONDS=15
NODE_SERIES_QUERIES=(
  'count(node_uname_info{job="node"})'
  'count(rate(node_cpu_seconds_total{job="node"}[1m]))'
  'count(rate(http_requests_total[1m]))'
)
SYNTH_SERIES_QUERIES=(
  'count(node_uname_info{job="node-exporter"})'
  'count(up{job="synth"} == 1)'
)

synth_history_minutes() {
  cat "$SLASHOBSERVE_EVALS_RUNTIME_DIR/synth/history-minutes"
}

synth_history_query() {
  local minutes
  minutes="$(synth_history_minutes)"
  printf 'count_over_time(up{job="synth"}[%sm]) >= %s' "$minutes" $((minutes * 60 / SYNTH_SCRAPE_SECONDS))
}

series_served() {
  curl -fsS --max-time 5 --get "http://127.0.0.1:${SLASHOBSERVE_PROMETHEUS_HOST_PORT}/api/v1/query" --data-urlencode "query=$1" 2>/dev/null |
    grep -q '"result":\[{'
}

wait_for_series() {
  local queries=("$@")
  local timeout=$((SERIES_TIMEOUT_SLACK + $(synth_history_minutes) * 60))
  local deadline=$((SECONDS + timeout)) missing query
  while true; do
    missing=()
    for query in "${queries[@]}"; do
      series_served "$query" || missing+=("$query")
    done
    [[ ${#missing[@]} -eq 0 ]] && return 0
    if ((SECONDS >= deadline)); then
      printf 'evals-stack: Prometheus on port %s did not serve these within %ss:\n' "$SLASHOBSERVE_PROMETHEUS_HOST_PORT" "$timeout" >&2
      printf '  %s\n' "${missing[@]}" >&2
      return 1
    fi
    sleep 2
  done
}

warm_marker() {
  printf '%s/warm' "$SLASHOBSERVE_EVALS_RUNTIME_DIR"
}

take_warm() {
  local marker waited=0 state="" stamp=""
  marker="$(warm_marker)"
  while [[ -f "$marker" ]]; do
    read -r state stamp <"$marker" || true
    [[ "$state" == pending && "$stamp" =~ ^[0-9]+$ ]] && kill -0 "$stamp" 2>/dev/null && ((waited++ < WARM_WAIT)) || break
    sleep 1
  done
  rm -f "$marker"
  [[ "$state" == ready && "$stamp" =~ ^[0-9]+$ ]] && (($(date +%s) - stamp <= WARM_MAX_AGE))
}

warm_services() {
  local names
  if [[ "$BUILD" == 1 ]]; then
    names="$(compose_build "$ENV_FILE" config --services)"
  else
    names="$(compose "$ENV_FILE" config --services)"
  fi
  grep -vx server <<<"$names"
}

prepare_runtime() {
  go run ./evals/runner prepare --out "$SLASHOBSERVE_EVALS_RUNTIME_DIR"
  if [[ "$K8S" == 1 ]]; then
    printf '[]\n' >"$SLASHOBSERVE_EVALS_RUNTIME_DIR/synth/series.json"
  fi
}

cmd_warm() {
  local services=() marker
  while [[ "${1:-}" == --* ]]; do
    case "$1" in
      --k8s) use_k8s ;;
      *) die "unknown option $1" ;;
    esac
    shift
  done
  [[ $# -eq 1 ]] || die "warm needs 'replay' or an env file"
  select_provider "$1"
  cd "$ROOT"
  marker="$(warm_marker)"
  rm -f "$marker"
  prepare_runtime
  printf 'pending %s\n' "$$" >"$marker"
  mapfile -t services < <(warm_services)
  [[ ${#services[@]} -gt 0 ]] || { rm -f "$marker"; die "docker compose config listed no services"; }
  if [[ "$BUILD" == 1 ]]; then
    compose_build "$ENV_FILE" up -d --build "${services[@]}" || { rm -f "$marker"; die "the warm services did not start"; }
  else
    compose "$ENV_FILE" up -d --no-build --pull missing "${services[@]}" || { rm -f "$marker"; die "the warm services did not start"; }
  fi
  printf 'ready %s\n' "$(date +%s)" >"$marker"
}

cmd_up() {
  local wait_history=1 queries
  while [[ "${1:-}" == --* ]]; do
    case "$1" in
      --k8s) use_k8s ;;
      --no-history) wait_history=0 ;;
      *) die "unknown option $1" ;;
    esac
    shift
  done
  [[ $# -eq 1 ]] || die "up needs 'replay' or an env file"
  select_provider "$1"
  cd "$ROOT"
  take_warm || prepare_runtime
  if [[ "$BUILD" == 1 ]]; then
    compose_build "$ENV_FILE" up -d --build --wait --wait-timeout 300
  else
    compose "$ENV_FILE" up -d --no-build --pull missing --wait --wait-timeout 300
  fi
  queries=("${NODE_SERIES_QUERIES[@]}")
  if [[ "$K8S" == 0 ]]; then
    queries+=("${SYNTH_SERIES_QUERIES[@]}")
    if [[ "$wait_history" == 1 ]]; then queries+=("$(synth_history_query)"); fi
  fi
  wait_for_series "${queries[@]}" || die "the stack is up but its metrics are not ready"
  if [[ "$K8S" == 1 ]]; then
    join_cluster
  fi
}

cmd_wait_history() {
  if [[ "${1:-}" == "--k8s" ]]; then
    return 0
  fi
  [[ $# -eq 0 ]] || die "wait-history takes no argument but --k8s"
  wait_for_series "$(synth_history_query)" || die "the synthetic series did not reach $(synth_history_minutes) minutes of history"
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
  rm -f "$(warm_marker)"
  compose /dev/null down -v --remove-orphans
}

cmd_status() {
  compose /dev/null ps
}

command="${1:-}"
[[ $# -gt 0 ]] && shift
case "$command" in
  warm) cmd_warm "$@" ;;
  up) cmd_up "$@" ;;
  server) cmd_server "$@" ;;
  wait-history) cmd_wait_history "$@" ;;
  down) cmd_down ;;
  status) cmd_status ;;
  -h | --help | help) usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
