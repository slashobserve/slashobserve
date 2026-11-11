#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="slashobserve-replicas"
RUNTIME="/tmp/slashobserve-replicas"
MIN_COMPOSE="2.24.4"

LB_PORT="${SLASHOBSERVE_REPLICAS_PORT:-48080}"
ONE_PORT="${SLASHOBSERVE_REPLICAS_ONE_PORT:-48081}"
TWO_PORT="${SLASHOBSERVE_REPLICAS_TWO_PORT:-48082}"
DEX_PORT="${SLASHOBSERVE_REPLICAS_DEX_PORT:-45556}"
LB="http://localhost:${LB_PORT}"
ONE="http://localhost:${ONE_PORT}"
TWO="http://localhost:${TWO_PORT}"

export SLASHOBSERVE_HOST_PORT="$LB_PORT"
export SLASHOBSERVE_REPLICAS_ONE_PORT="$ONE_PORT"
export SLASHOBSERVE_REPLICAS_TWO_PORT="$TWO_PORT"
export SLASHOBSERVE_DEX_HOST_PORT="$DEX_PORT"
export SLASHOBSERVE_PROMETHEUS_HOST_PORT="${SLASHOBSERVE_REPLICAS_PROMETHEUS_PORT:-49090}"
export SLASHOBSERVE_SHOP_HOST_PORT="${SLASHOBSERVE_REPLICAS_SHOP_PORT:-48090}"
export SLASHOBSERVE_LOKI_HOST_PORT="${SLASHOBSERVE_REPLICAS_LOKI_PORT:-43100}"
export SLASHOBSERVE_TEMPO_HOST_PORT="${SLASHOBSERVE_REPLICAS_TEMPO_PORT:-43200}"
export SLASHOBSERVE_DATASOURCES_HOST_FILE=./datasources.replicas.yaml
export SLASHOBSERVE_LOADGEN_CURVE=flat
export SLASHOBSERVE_DEMO_MARKERS_URL=
export SLASHOBSERVE_IMAGE="slashobserve-replicas:local"
export SLASHOBSERVE_SHOP_IMAGE="slashobserve-replicas-shop:local"

COMPOSE_CMD=(docker compose
  -f "$ROOT/deploy/compose/compose.yaml"
  -f "$ROOT/deploy/compose/compose.build.yaml"
  -f "$ROOT/deploy/compose/compose.replicas.yaml"
  -p "$PROJECT")
SERVICES=(server haproxy pgbouncer frontend-0 checkout-0 checkout-1 payments-0 loadgen)

curl() { command curl --max-time 10 "$@"; }

read -r -d '' REDACT_PERL <<'PERL' || true
my $key = qr/(?!\w*tokens["']?\s*[:=]\s*\d+(?![\w.~+\/=-]))\w*(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|access[_-]?key)\w*/i;
s{(://)[^/\s:\@]+:[^/\s\@]+\@}{${1}\[redacted]\@}g;
s/sok_[A-Za-z0-9_-]+/sok_\[redacted]/gi;
s/so[ar]_[A-Za-z0-9_-]+/so_\[redacted]/gi;
s/(Bearer\s+)[A-Za-z0-9._~+\/=-]+/${1}\[redacted]/gi;
s/\b((?:authorization|cookie|set-cookie|x-api-key|so_session)["']?\s*[:=]\s*)(?:(["'])(?:(?:\\.|(?!\2)[^\\])*\2|(?:(?!\2).)*\2)|.*)/defined $2 ? "${1}${2}\[redacted]${2}" : "${1}\[redacted]"/gie;
s/(so_session\s+)\S+/${1}\[redacted]/gi;
s/($key)(["']?\s*[:=]\s*)(["'])(?:(?:\\.|(?!\3)[^\\])*\3|(?:(?!\3).)*\3)/${1}${2}${3}\[redacted]${3}/g;
s/($key)(["']?\s*[:=]\s*)(?!\[redacted\])[^\s,;&"'}\])]+/${1}${2}\[redacted]/g;
s/([?&]code=)[^&\s#"']+/${1}\[redacted]/gi;
PERL

redact() {
  perl -pe "$REDACT_PERL"
}

usage() {
  cat <<EOF
Usage: scripts/replicas.sh <command>

  up           build this checkout and boot the isolated ${PROJECT} stack: two servers behind
               HAProxy and PgBouncer (transaction mode), then create the load test's keys
  down         remove the ${PROJECT} containers and volumes and ${RUNTIME}; nothing else
  status       list the containers of ${PROJECT}
  logs [n]     print the last n (default 200) log lines of the servers, HAProxy and PgBouncer
  keys         create the load test's service-account keys under ${RUNTIME}
  stop <1|2>   stop one server gracefully (30 s grace)
  kill <1|2>   kill one server
  start <1|2>  start a stopped server and wait for it to be ready
  workers <n>  recreate the servers with SLASHOBSERVE_AGENT_WORKERS=n
  help         print this text

Ports (all on 127.0.0.1, all overridable):
  SLASHOBSERVE_REPLICAS_PORT             load balancer, round-robin (default 48080)
  SLASHOBSERVE_REPLICAS_ONE_PORT         server 1 only (default 48081)
  SLASHOBSERVE_REPLICAS_TWO_PORT         server 2 only (default 48082)
  SLASHOBSERVE_REPLICAS_DEX_PORT         Dex (default 45556)
  SLASHOBSERVE_REPLICAS_PROMETHEUS_PORT  Prometheus (default 49090)
  SLASHOBSERVE_REPLICAS_SHOP_PORT        shop fault API (default 48090)
  SLASHOBSERVE_REPLICAS_LOKI_PORT        reserved, Loki is not started (default 43100)
  SLASHOBSERVE_REPLICAS_TEMPO_PORT       reserved, Tempo is not started (default 43200)

Requirements: docker with compose ${MIN_COMPOSE} or newer, curl, perl, python3.
EOF
}

die() {
  printf 'replicas: %s\n' "$(printf '%s' "$1" | redact)" >&2
  exit 1
}

say() {
  printf 'replicas: %s\n' "$1"
}

TMP=""
scratch() {
  [[ -n "$TMP" ]] || TMP="$(mktemp -d)"
}

cleanup() {
  [[ -z "$TMP" ]] || rm -rf "$TMP"
}
trap cleanup EXIT

require_compose() {
  local have
  have="$(docker compose version --short)"
  [[ "$(printf '%s\n%s\n' "$MIN_COMPOSE" "$have" | sort -V | head -1)" == "$MIN_COMPOSE" ]] ||
    die "docker compose $have is older than $MIN_COMPOSE, which the stack's !reset and !override need"
}

psql_app() {
  "${COMPOSE_CMD[@]}" exec -T postgres psql -U slashobserve -d slashobserve -qAt -c "$1"
}

psql_bouncer() {
  "${COMPOSE_CMD[@]}" exec -T postgres psql -U slashobserve -h pgbouncer -p 6432 -d pgbouncer -qAt -c "$1"
}

server_port() {
  case "$1" in
    1) printf '%s' "$ONE_PORT" ;;
    2) printf '%s' "$TWO_PORT" ;;
    *) die "the server is 1 or 2, not '$1'" ;;
  esac
}

wait_ready() {
  local url="$1" seconds="$2" i
  for ((i = 0; i < seconds; i++)); do
    if curl -fsS -o /dev/null "$url/readyz" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  die "$url/readyz did not answer 200 within ${seconds}s"
}

dex_login() {
  local jar="$1" authorize form_url final
  scratch
  authorize="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code} %{redirect_url}' \
    --get --data-urlencode "return_to=/" "$LB/api/v1/auth/login")"
  [[ "$authorize" == "302 http://localhost:${DEX_PORT}/dex/auth?"* ]] || die "login redirects to Dex ($authorize)"
  form_url="$(curl -sS -L -c "$jar" -b "$jar" -o "$TMP/dex-form.html" -w '%{url_effective}' "${authorize#* }")"
  grep -q 'name="login"' "$TMP/dex-form.html" || die "Dex login form not reached ($form_url)"
  final="$(curl -sS -L -c "$jar" -b "$jar" -o /dev/null -w '%{url_effective}' \
    --data-urlencode "login=admin@example.com" --data-urlencode password=password "$form_url")"
  [[ "$final" == "$LB/" ]] || die "the code flow lands on $LB/ ($final)"
}

session_post() {
  local jar="$1" path="$2" body="$3" out="$4" code tries=0
  while ((tries < 5)); do
    code="$(curl -sS -b "$jar" -o "$out" -w '%{http_code}' -X POST \
      -H "Origin: $LB" -H 'Content-Type: application/json' -d "$body" "$LB$path")"
    [[ "$code" != 429 ]] && break
    tries=$((tries + 1))
    sleep 1
  done
  printf '%s' "$code"
}

make_key() {
  local jar="$1" name="$2" role="$3" out id code token
  out="$TMP/key-response.json"
  code="$(session_post "$jar" /api/v1/service-accounts "{\"name\":\"$name\",\"role\":\"$role\"}" "$out")"
  [[ "$code" == 201 ]] || die "create service account $name ($code: $(head -c 300 "$out"))"
  id="$(grep -o '"id":"[^"]*"' "$out" | head -1 | cut -d'"' -f4)"
  code="$(session_post "$jar" "/api/v1/service-accounts/$id/keys" '{"name":"loadtest"}' "$out")"
  [[ "$code" == 201 ]] || die "create a key for $name ($code: $(head -c 300 "$out"))"
  token="$(grep -o '"token":"sok_[^"]*"' "$out" | cut -d'"' -f4)"
  [[ "$token" == sok_* ]] || die "the key response for $name has no token"
  printf '%s\n' "$token"
}

keys() {
  local jar="$RUNTIME/admin.jar" i name
  mkdir -p -m 700 "$RUNTIME"
  if [[ -s "$RUNTIME/viewer.keys" && -s "$RUNTIME/editor.keys" && -s "$RUNTIME/admin.key" ]]; then
    say "keys exist in $RUNTIME"
    return 0
  fi
  scratch
  dex_login "$jar"
  chmod 600 "$jar"
  (
    umask 077
    : >"$TMP/viewer.keys"
    : >"$TMP/editor.keys"
    for ((i = 1; i <= 100; i++)); do
      name="$(printf 'loadtest-viewer-%03d' "$i")"
      make_key "$jar" "$name" viewer >>"$TMP/viewer.keys"
    done
    for ((i = 1; i <= 20; i++)); do
      name="$(printf 'loadtest-editor-%02d' "$i")"
      make_key "$jar" "$name" editor >>"$TMP/editor.keys"
    done
    make_key "$jar" replicas-admin admin >"$TMP/admin.key"
  )
  install -m 600 "$TMP/viewer.keys" "$RUNTIME/viewer.keys"
  install -m 600 "$TMP/editor.keys" "$RUNTIME/editor.keys"
  install -m 600 "$TMP/admin.key" "$RUNTIME/admin.key"
  say "created 100 viewer, 20 editor and 1 admin key in $RUNTIME"
}

up() {
  require_compose
  "${COMPOSE_CMD[@]}" up -d --build --wait --wait-timeout 600 --scale server=2 "${SERVICES[@]}" ||
    {
      logs 100
      die "the stack did not become healthy"
    }
  wait_ready "$LB" 60
  wait_ready "$ONE" 60
  wait_ready "$TWO" 60
  keys
  say "up: $LB (round-robin), $ONE (server 1), $TWO (server 2)"
}

down() {
  say "deleting project $PROJECT only"
  "${COMPOSE_CMD[@]}" down -v --remove-orphans
  rm -rf "$RUNTIME"
}

status() {
  "${COMPOSE_CMD[@]}" ps --format 'table {{.Name}}\t{{.Status}}'
}

logs() {
  "${COMPOSE_CMD[@]}" logs --no-color --tail "${1:-200}" server haproxy pgbouncer 2>&1 | redact
}

stop_server() {
  server_port "$1" >/dev/null
  docker stop -t 30 "${PROJECT}-server-$1" >/dev/null
  say "server $1 stopped"
}

kill_server() {
  server_port "$1" >/dev/null
  docker kill "${PROJECT}-server-$1" >/dev/null
  say "server $1 killed"
}

start_server() {
  local port
  port="$(server_port "$1")"
  docker start "${PROJECT}-server-$1" >/dev/null
  wait_ready "http://localhost:${port}" 90
  say "server $1 ready"
}

set_workers() {
  local n="${1:-}" count i
  [[ "$n" =~ ^[0-9]+$ ]] && ((n >= 1 && n <= 64)) || die "workers is a whole number from 1 to 64"
  require_compose
  count="$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" --filter "label=com.docker.compose.service=server" | wc -l | tr -d ' ')"
  ((count >= 1)) || die "no server containers in $PROJECT; run up first"
  SLASHOBSERVE_REPLICAS_AGENT_WORKERS="$n" "${COMPOSE_CMD[@]}" up -d --no-deps --force-recreate --scale "server=$count" --wait server
  wait_ready "$LB" 60
  for ((i = 1; i <= count; i++)); do
    wait_ready "http://localhost:$(server_port "$i")" 60
    docker logs "${PROJECT}-server-$i" 2>&1 | grep 'agent workers configured' | tail -1 | grep -q "\"workers\":$n" ||
      die "server $i did not log agent workers configured with workers $n"
  done
  say "servers run with $n agent workers"
}

main() {
  local cmd="${1:-help}"
  shift || true
  case "$cmd" in
    up) up ;;
    down) down ;;
    status) status ;;
    logs) logs "$@" ;;
    keys) keys ;;
    stop) stop_server "${1:-}" ;;
    kill) kill_server "${1:-}" ;;
    start) start_server "${1:-}" ;;
    workers) set_workers "${1:-}" ;;
    help | -h | --help) usage ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
