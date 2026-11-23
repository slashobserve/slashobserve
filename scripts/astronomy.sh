#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASTRONOMY_DIR="$ROOT/deploy/astronomy"
UPSTREAM_DIR="$ASTRONOMY_DIR/.upstream"
STATE_DIR="${ASTRONOMY_STATE_DIR:-$ASTRONOMY_DIR/.state}"
PROJECT="slashobserve-astronomy"
MIN_COMPOSE="2.24.4"
WAIT_TIMEOUT=900
CHECK_WAIT=180

export SLASHOBSERVE_HOST_PORT="${SLASHOBSERVE_HOST_PORT:-58080}"
export SLASHOBSERVE_DEX_HOST_PORT="${SLASHOBSERVE_DEX_HOST_PORT:-55556}"
export ASTRONOMY_SHOP_HOST_PORT="${ASTRONOMY_SHOP_HOST_PORT:-58081}"
export ASTRONOMY_DIR
export ASTRONOMY_STATE_DIR="$STATE_DIR"
export SLASHOBSERVE_COMPOSE_DIR="$ROOT/deploy/compose"
export OTEL_COLLECTOR_CONFIG_EXTRAS="$ASTRONOMY_DIR/otelcol/config.yaml"
export COMPOSE_PROJECT_NAME="$PROJECT"

SERVER="http://localhost:${SLASHOBSERVE_HOST_PORT}"
KEY_FILE="$STATE_DIR/keys/astronomy.key"
FLAGS_FILE="$STATE_DIR/flagd/demo.flagd.json"

usage() {
  cat <<EOF
Usage: scripts/astronomy.sh <command>

  up [--search]       fetch the pinned upstream release if needed, build the server image from
                      this checkout, then start the shop and the SlashObserve stack and wait
                      until everything is healthy; --search adds Elasticsearch and OpenSearch
                      and feeds them the shop's logs
  down [--volumes]    stop the stack; --volumes also drops its data and the flag state
  status              list the containers
  check [--search]    check the health of every datasource through the SlashObserve API and
                      read one row of the shop's data from each
  flag [<name> [<variant>]]
                      without a name, list the feature flags and their variants;
                      with one, set it to <variant> (default 'on'); 'off' switches it off
  key                 print the path of the file that holds the service-account key

Environment:
  SLASHOBSERVE_HOST_PORT      host port of the server (default 58080)
  SLASHOBSERVE_DEX_HOST_PORT  host port of Dex (default 55556)
  ASTRONOMY_SHOP_HOST_PORT    host port of the shop, its Jaeger UI and its flag UI (default 58081)
  SLASHOBSERVE_IMAGE          the tag the server image is built as (default slashobserve-astronomy-server:local)

Model provider: put SLASHOBSERVE_LLM_PROVIDER, SLASHOBSERVE_LLM_MODEL and
SLASHOBSERVE_LLM_API_KEY in deploy/astronomy/.env (git-ignored), then run 'up' again.

Requirements: docker with compose v${MIN_COMPOSE} or newer, git, curl, jq and openssl.
EOF
}

die() {
  printf 'astronomy: %s\n' "$1" >&2
  exit 1
}

check_tools() {
  local tool version
  for tool in docker git curl jq openssl; do
    command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
  done
  version="$(docker compose version --short 2>/dev/null || true)"
  [[ -n "$version" ]] || die "docker compose is required"
  [[ "$(printf '%s\n%s\n' "$MIN_COMPOSE" "${version#v}" | sort -V | head -n 1)" == "$MIN_COMPOSE" ]] ||
    die "docker compose $MIN_COMPOSE or newer is required, found $version"
}

fetch_upstream() {
  # shellcheck source=/dev/null
  . "$ASTRONOMY_DIR/upstream.env"
  if [[ ! -d "$UPSTREAM_DIR/.git" ]]; then
    git init -q "$UPSTREAM_DIR"
    git -C "$UPSTREAM_DIR" remote add origin "$UPSTREAM_REPO"
  fi
  if [[ "$(git -C "$UPSTREAM_DIR" rev-parse -q --verify HEAD 2>/dev/null || true)" != "$UPSTREAM_COMMIT" ]]; then
    printf 'astronomy: fetching opentelemetry-demo %s\n' "$UPSTREAM_TAG"
    git -C "$UPSTREAM_DIR" fetch -q --depth 1 origin "$UPSTREAM_COMMIT"
  fi
  git -C "$UPSTREAM_DIR" reset -q --hard "$UPSTREAM_COMMIT"
  [[ "$(git -C "$UPSTREAM_DIR" rev-parse HEAD)" == "$UPSTREAM_COMMIT" ]] ||
    die "the upstream checkout is not at $UPSTREAM_COMMIT"
}

prepare_state() {
  mkdir -p "$STATE_DIR/keys" "$STATE_DIR/flagd"
  chmod 755 "$STATE_DIR/keys"
  chmod 777 "$STATE_DIR/flagd"
  if [[ ! -f "$KEY_FILE" ]]; then
    printf 'sok_%s\n' "$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')" >"$KEY_FILE"
  fi
  chmod 644 "$KEY_FILE"
  if [[ ! -f "$FLAGS_FILE" ]]; then
    cp "$UPSTREAM_DIR/src/flagd/demo.flagd.json" "$FLAGS_FILE"
  fi
  chmod 666 "$FLAGS_FILE"
}

compose() {
  local files=(-f "$UPSTREAM_DIR/compose.yaml" -f "$UPSTREAM_DIR/compose.full.yaml" -f "$ASTRONOMY_DIR/compose.yaml" -f "$ASTRONOMY_DIR/compose.build.yaml")
  local envs=(--env-file "$UPSTREAM_DIR/.env" --env-file "$ASTRONOMY_DIR/astronomy.env")
  local profiles=()
  if [[ "$SEARCH" == 1 ]]; then
    files+=(-f "$ASTRONOMY_DIR/compose.search.yaml")
    profiles+=(--profile search)
  fi
  if [[ -f "$ASTRONOMY_DIR/.env" ]]; then envs+=(--env-file "$ASTRONOMY_DIR/.env"); fi
  docker compose --project-directory "$UPSTREAM_DIR" "${envs[@]}" "${files[@]}" ${profiles[@]+"${profiles[@]}"} "$@"
}

parse_options() {
  SEARCH=0
  VOLUMES=0
  local arg
  for arg in "$@"; do
    case "$arg" in
      --search) SEARCH=1 ;;
      --volumes) VOLUMES=1 ;;
      *) die "unknown option $arg" ;;
    esac
  done
}

cmd_up() {
  parse_options "$@"
  check_tools
  fetch_upstream
  prepare_state
  export SLASHOBSERVE_IMAGE="${SLASHOBSERVE_IMAGE:-slashobserve-astronomy-server:local}"
  compose build server
  compose up -d --no-build --remove-orphans --wait --wait-timeout "$WAIT_TIMEOUT"
  cat <<EOF
The Astronomy Shop is running at http://localhost:${ASTRONOMY_SHOP_HOST_PORT}
Its Jaeger UI is at http://localhost:${ASTRONOMY_SHOP_HOST_PORT}/jaeger/ui and its feature flags at http://localhost:${ASTRONOMY_SHOP_HOST_PORT}/feature/
SlashObserve is running at ${SERVER}
Sign in as admin@example.com / password (Dex, development only)
The service-account key is in ${KEY_FILE}
EOF
}

cmd_down() {
  parse_options "$@"
  check_tools
  [[ -d "$UPSTREAM_DIR/.git" ]] || die "nothing to stop: no upstream checkout"
  SEARCH=1
  prepare_state
  if [[ "$VOLUMES" == 1 ]]; then
    compose down --remove-orphans --volumes
    rm -f "$FLAGS_FILE"
  else
    compose down --remove-orphans
  fi
}

cmd_status() {
  parse_options
  SEARCH=1
  check_tools
  [[ -d "$UPSTREAM_DIR/.git" ]] || die "nothing is running: no upstream checkout"
  prepare_state
  compose ps
}

api() {
  curl -fsS --max-time 30 -H "Authorization: Bearer $(cat "$KEY_FILE")" "$@"
}

datasource_healthy() {
  [[ "$(api -X POST "$SERVER/api/v1/datasources/$1/recheck" | jq -r '.healthy')" == true ]]
}

datasource_rows() {
  local body
  body="$(jq -n --arg q "$2" '{query: $q, kind: "instant", from: "now-15m", to: "now", limit: 5}')"
  api -H 'Content-Type: application/json' -d "$body" "$SERVER/api/v1/datasources/$1/query" |
    jq '[.frames[].fields[0].values | length] | add // 0'
}

check_datasource() {
  local uid="$1" what="$2" query="$3" deadline=$((SECONDS + CHECK_WAIT)) rows=0
  if [[ " $LISTED " != *" $uid "* ]]; then
    printf 'FAIL %-8s not in the datasource list\n' "$uid"
    return 1
  fi
  until datasource_healthy "$uid" 2>/dev/null; do
    if ((SECONDS >= deadline)); then
      printf 'FAIL %-8s unhealthy\n' "$uid"
      return 1
    fi
    sleep 3
  done
  until rows="$(datasource_rows "$uid" "$query" 2>/dev/null)" && ((rows > 0)); do
    if ((SECONDS >= deadline)); then
      printf 'FAIL %-8s healthy, but no %s\n' "$uid" "$what"
      return 1
    fi
    sleep 3
  done
  printf 'ok   %-8s healthy, %s: %s rows\n' "$uid" "$what" "$rows"
}

cmd_check() {
  parse_options "$@"
  check_tools
  [[ -f "$KEY_FILE" ]] || die "no key yet; run 'up' first"
  local failed=0
  LISTED="$(api "$SERVER/api/v1/datasources" | jq -r '[.datasources[].uid] | sort | join(" ")')"
  printf 'datasources: %s\n' "$LISTED"
  check_datasource prom "shop span metrics" 'sum(traces_span_metrics_calls_total{service_name="frontend"})' || failed=1
  check_datasource ch "frontend log lines" "SELECT Timestamp, Body FROM otel_logs WHERE ServiceName = 'frontend' AND \$__timeFilter(Timestamp) LIMIT 5" || failed=1
  check_datasource ch "frontend spans" "SELECT TraceId, SpanName FROM otel_traces WHERE ServiceName = 'frontend' AND \$__timeFilter(Timestamp) LIMIT 5" || failed=1
  check_datasource loki "frontend log lines" '{service_name="frontend"}' || failed=1
  check_datasource tempo "frontend traces" '{ resource.service.name = "frontend" }' || failed=1
  check_datasource jaeger "frontend traces" 'service=frontend minDuration=0s' || failed=1
  if [[ "$SEARCH" == 1 ]]; then
    check_datasource es "frontend log lines" 'service.name: "frontend"' || failed=1
    check_datasource os "frontend log lines" 'resource.service.name: "frontend"' || failed=1
  fi
  return "$failed"
}

cmd_flag() {
  [[ -f "$FLAGS_FILE" ]] || die "no flag file; run 'up' first"
  local name="${1:-}" variant="${2:-on}" updated
  if [[ -z "$name" ]]; then
    jq -r '.flags | to_entries[] | [.key, .value.defaultVariant, (.value.variants | keys | join(","))] | @tsv' "$FLAGS_FILE" |
      column -t -s "$(printf '\t')"
    return
  fi
  jq -e --arg f "$name" '.flags | has($f)' "$FLAGS_FILE" >/dev/null || die "no flag named $name; run 'flag' to list them"
  jq -e --arg f "$name" --arg v "$variant" '.flags[$f].variants | has($v)' "$FLAGS_FILE" >/dev/null ||
    die "$name has no variant $variant; it has $(jq -r --arg f "$name" '.flags[$f].variants | keys | join(", ")' "$FLAGS_FILE")"
  updated="$(jq --arg f "$name" --arg v "$variant" '.flags[$f].defaultVariant = $v' "$FLAGS_FILE")"
  printf '%s\n' "$updated" >"$FLAGS_FILE"
  printf '%s is now %s; flagd reloads the file within a few seconds\n' "$name" "$variant"
}

cmd_key() {
  [[ -f "$KEY_FILE" ]] || die "no key yet; run 'up' first"
  printf '%s\n' "$KEY_FILE"
}

command="${1:-}"
if [[ $# -gt 0 ]]; then shift; fi
case "$command" in
  up) cmd_up "$@" ;;
  down) cmd_down "$@" ;;
  status) cmd_status ;;
  check) cmd_check "$@" ;;
  flag) cmd_flag "$@" ;;
  key) cmd_key ;;
  -h | --help | help | "") usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
