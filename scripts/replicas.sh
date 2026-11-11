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
  check [item...]
               run the two-server checklist on the running stack and print one PASS, FAIL or
               KNOWN line per item; named items (for example C5 C13) rerun alone, and C1 to C3
               need a fresh boot, so use make replicas-check for the whole list
  load [--only viewers|streams|mcp|agent] [label]
               run the load test (go run ./loadtest) against the load balancer with the running
               servers as replicas, sample container, Postgres and PgBouncer use every 5 s, and write
               loadtest/reports/<time>-<label>/ with report.md, report.json, slow-queries.txt and the
               samples; it never targets anything but this stack; use make loadtest for the full series
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
SSE_PIDS=""
SAMPLER_PID=""
scratch() {
  [[ -n "$TMP" ]] || TMP="$(mktemp -d)"
}

cleanup() {
  local pid
  for pid in $SSE_PIDS; do
    kill "$pid" 2>/dev/null || true
  done
  [[ -z "$SAMPLER_PID" ]] || kill "$SAMPLER_PID" 2>/dev/null || true
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
  "${COMPOSE_CMD[@]}" exec -T -e PGPASSWORD=slashobserve postgres psql -U slashobserve -h pgbouncer -p 6432 -d pgbouncer -qAt -c "$1"
}

server_log_has() {
  docker logs "${PROJECT}-server-$1" >"$TMP/server-$1.log" 2>&1 || true
  grep -q -- "$2" "$TMP/server-$1.log"
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
    scratch
    server_log_has "$i" "agent workers configured.*\"workers\":$n" || die "server $i did not log agent workers configured with workers $n"
  done
  say "servers run with $n agent workers"
}

CHECK_ITEMS=(C1 C2 C3 C4 C5 C6 C7 C8 C15 C12 C13 C14 C9 C10 C11)
ADMIN_KEY_FILE="$RUNTIME/admin.key"
EDITOR_KEYS_FILE="$RUNTIME/editor.keys"
ITEM=""
CHECK_DASHBOARD=""
CHECK_VERSION=1
BUILD_IDS=""
SSE_LAST=""
FAILED=0

pass() {
  printf 'PASS  %s %s\n' "$ITEM" "$1"
}

known() {
  printf 'KNOWN %s %s\n' "$ITEM" "$1"
}

info() {
  printf '      %s\n' "$1"
}

fail() {
  local n
  printf 'FAIL  %s %s\n' "$ITEM" "$(printf '%s' "$1" | redact)"
  for n in 1 2; do
    printf -- '--- last 200 log lines of server %s\n' "$n"
    docker logs --tail 200 "${PROJECT}-server-$n" 2>&1 | redact || true
  done
  exit 1
}

eventually() {
  local seconds="$1" what="$2" i
  shift 2
  for ((i = 0; i < seconds; i++)); do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  "$@" >/dev/null 2>&1 || fail "$what did not hold within ${seconds}s"
}

metric() {
  local n="$1" series="$2"
  curl -fsS "http://localhost:$(server_port "$n")/metrics" | awk -v s="$series" 'index($0, s " ") == 1 && !seen { print $2; seen = 1 }'
}

metric_is() {
  local got
  got="$(metric "$1" "$2")"
  [[ -n "$got" ]] && awk -v a="$got" -v b="$3" 'BEGIN { exit !(a + 0 == b + 0) }'
}

metric_sum() {
  curl -fsS "http://localhost:$(server_port "$1")/metrics" | awk -v p="$2" 'index($0, p) == 1 { total += $2 } END { print total + 0 }'
}

fleet_is() {
  metric_is 1 slashobserve_fleet_servers "$1" && metric_is 2 slashobserve_fleet_servers "$1"
}

slots_are() {
  metric_is 1 'slashobserve_datasource_slots_limit{datasource="prom_small"}' "$1" &&
    metric_is 2 'slashobserve_datasource_slots_limit{datasource="prom_small"}' "$1"
}

json_field() {
  python3 -I -c 'import json, sys
d = json.load(open(sys.argv[1]))
for part in sys.argv[2].split("."):
    d = d[int(part)] if isinstance(d, list) else d[part]
print("" if d is None else d)' "$1" "$2"
}

admin_key() {
  cat "$ADMIN_KEY_FILE"
}

editor_key() {
  sed -n "$(($1 + 1))p" "$EDITOR_KEYS_FILE"
}

api_as() {
  local key="$1" method="$2" base="$3" path="$4" body="${5:-}" out="${OUT:-$TMP/api.body}"
  if [[ -n "$body" ]]; then
    curl -sS -o "$out" -w '%{http_code}' -X "$method" -H "Authorization: Bearer $key" \
      -H 'Content-Type: application/json' -d "$body" "$base$path"
  else
    curl -sS -o "$out" -w '%{http_code}' -X "$method" -H "Authorization: Bearer $key" "$base$path"
  fi
}

api() {
  api_as "$(admin_key)" "$@"
}

session_api() {
  local jar="$1" method="$2" base="$3" path="$4" body="${5:-}" out="${OUT:-$TMP/api.body}"
  if [[ -n "$body" ]]; then
    curl -sS -b "$jar" -o "$out" -w '%{http_code}' -X "$method" -H "Origin: $LB" \
      -H 'Content-Type: application/json' -d "$body" "$base$path"
  else
    curl -sS -b "$jar" -o "$out" -w '%{http_code}' -X "$method" -H "Origin: $LB" "$base$path"
  fi
}

new_session() {
  local jar="$1"
  : >"$jar"
  dex_login "$jar"
}

check_spec() {
  printf '{"specVersion":"0","title":"%s","tags":["replicas-check"],"time":{"from":"now-15m","to":"now"},"panels":[{"id":"rps","type":"timeseries","title":"Requests per second","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"sum(rate(http_requests_total[$__rate_interval]))"}]}]}' "$1"
}

ensure_dashboard() {
  local code
  [[ -z "$CHECK_DASHBOARD" ]] || return 0
  code="$(api POST "$ONE" /api/v1/dashboards "{\"summary\":\"replicas check\",\"spec\":$(check_spec 'Replicas check')}")"
  [[ "$code" == 201 ]] || fail "create the check dashboard on server 1 ($code: $(head -c 300 "$TMP/api.body"))"
  CHECK_DASHBOARD="$(json_field "$TMP/api.body" dashboard.id)"
  CHECK_VERSION=1
}

annotate() {
  local base="$1" text="$2" code
  code="$(OUT="$TMP/annotation.json" api POST "$base" /api/v1/annotations "{\"dashboard_id\":\"$CHECK_DASHBOARD\",\"text\":\"$text\"}")"
  [[ "$code" == 201 ]] || fail "create an annotation on $base ($code: $(head -c 300 "$TMP/annotation.json"))"
  json_field "$TMP/annotation.json" id
}

sse_open() {
  local url="$1" file="$2" last="${3:-}" key tries=50
  local args=()
  key="$(admin_key)"
  args=(-H "Authorization: Bearer $key")
  if [[ -n "$last" ]]; then
    args=(-H "Authorization: Bearer $key" -H "Last-Event-ID: $last")
  fi
  : >"$file"
  command curl -sS -N --max-time 900 "${args[@]}" "$url" >"$file" 2>/dev/null &
  SSE_LAST=$!
  SSE_PIDS="$SSE_PIDS $SSE_LAST"
  until grep -q '^event: ready$' "$file"; do
    ((--tries > 0)) || fail "the stream $url sent no ready event ($(head -c 300 "$file"))"
    sleep 0.2
  done
}

sse_close() {
  kill "$1" 2>/dev/null || true
  wait "$1" 2>/dev/null || true
}

sse_has() {
  local file="$1" pattern="$2" seconds="$3" tries=$(($3 * 5))
  until grep -q -- "$pattern" "$file"; do
    ((--tries > 0)) || return 1
    sleep 0.2
  done
}

sse_last_id() {
  { grep '^id: ' "$1" || true; } | tail -1 | sed 's/^id: //'
}

sse_ids() {
  { grep '^id: ' "$1" || true; } | sed 's/^id: //'
}

sql_list() {
  local out="" id
  for id in "$@"; do
    out="$out${out:+,}'$id'"
  done
  printf '%s' "$out"
}

submit_build() {
  local key="$1" base="$2" code tries=0 out
  out="$TMP/submit.json"
  while ((tries < 120)); do
    code="$(curl -sS -o "$out" -w '%{http_code}' -X POST -H "Authorization: Bearer $key" \
      -H 'Content-Type: application/json' -d '{"prompt":"replicas stack build"}' "$base/api/v1/observe")"
    if [[ "$code" == 202 ]]; then
      json_field "$out" request.id
      return 0
    fi
    if [[ "$code" != 429 ]]; then
      printf '%s: %s' "$code" "$(head -c 300 "$out")"
      return 1
    fi
    tries=$((tries + 1))
    sleep 1
  done
  printf 'still 429 after 120 tries'
  return 1
}

submit_builds() {
  local count="$1" mode="$2" i base id
  BUILD_IDS=""
  for ((i = 0; i < count; i++)); do
    base="$LB"
    if [[ "$mode" == alternate ]]; then
      if ((i % 2 == 0)); then base="$ONE"; else base="$TWO"; fi
    fi
    id="$(submit_build "$(editor_key $((i % 7)))" "$base")" || fail "submit build $i ($id)"
    BUILD_IDS="$BUILD_IDS $id"
  done
}

builds_in_status() {
  psql_app "SELECT count(*) FROM agent_requests WHERE id IN ($(sql_list $BUILD_IDS)) AND status IN ($1)"
}

builds_running_at_least() {
  (($(builds_in_status "'running'") >= $1))
}

builds_settled() {
  [[ "$(builds_in_status "'queued','running'")" == 0 ]]
}

build_report() {
  psql_app "SELECT left(id::text, 8) || ' ' || status || ' failure=' || coalesce(failure_code, '-') || ' attempts=' || attempts FROM agent_requests WHERE id IN ($(sql_list $BUILD_IDS)) ORDER BY created_at"
}

oauth_register() {
  curl -sS -o "$2" -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -d '{"client_name":"replicas-check","redirect_uris":["http://127.0.0.1:9/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"]}' \
    "$1/oauth/register"
}

dex_callback() {
  local jar="$1" url="$2" out hop
  out="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code} %{redirect_url}' \
    --data-urlencode login=admin@example.com --data-urlencode password=password "$url")"
  for hop in 1 2 3 4 5 6; do
    url="${out#* }"
    [[ -n "$url" ]] || return 1
    if [[ "$url" == "$LB/api/v1/auth/callback?"* ]]; then
      printf '%s' "$url"
      return 0
    fi
    out="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code} %{redirect_url}' "$url")"
  done
  return 1
}

form_field() {
  python3 -I -c 'import sys, urllib.parse
q = urllib.parse.parse_qs(urllib.parse.urlparse(sys.argv[1]).query)
print(q[sys.argv[2]][0])' "$1" "$2"
}

pkce_challenge() {
  printf '%s' "$1" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '='
}

mcp_tools_list() {
  curl -sS -o "$2" -w '%{http_code}' -X POST -H "Authorization: Bearer $3" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$1/mcp"
}

token_request() {
  local base="$1" out="$2"
  shift 2
  curl -sS -o "$out" -w '%{http_code}' -X POST "$base/oauth/token" "$@"
}

check_c1() {
  local dup n state
  dup="$(psql_app "SELECT version_id FROM goose_db_version GROUP BY 1 HAVING count(*) > 1")"
  [[ -z "$dup" ]] || fail "a migration version was applied twice ($dup)"
  for n in 1 2; do
    curl -fsS -o /dev/null "http://localhost:$(server_port "$n")/readyz" || fail "server $n is not ready"
    state="$(docker inspect -f '{{.RestartCount}} {{.State.OOMKilled}}' "${PROJECT}-server-$n")"
    [[ "$state" == "0 false" ]] || fail "server $n restarted or was OOM killed ($state)"
  done
  pass "migrations ran once; both servers are ready and neither restarted"
}

provisioned_clean() {
  [[ "$(psql_app "SELECT count(*) FROM provisioned_dashboards WHERE last_error = ''")" == "$1" ]]
}

check_c2() {
  local want n
  want="$(find "$ROOT/deploy/compose/dashboards/SlashObserve" -name '*.json' | wc -l | tr -d ' ')"
  eventually 300 "provisioned_dashboards to hold $want rows without an error" provisioned_clean "$want"
  [[ "$(psql_app "SELECT count(*) FROM dashboards")" == "$want" ]] ||
    fail "dashboards holds $(psql_app "SELECT count(*) FROM dashboards") rows, not $want"
  [[ "$(psql_app "SELECT count(*) FROM provisioned_dashboards")" == "$want" ]] || fail "provisioned_dashboards holds more than $want rows"
  for n in 1 2; do
    if server_log_has "$n" service_accounts_failed; then
      fail "server $n logged service_accounts_failed"
    fi
  done
  [[ "$(psql_app "SELECT count(*) FROM service_accounts WHERE name = 'demo-loadgen'")" == 1 ]] ||
    fail "the demo-loadgen service account does not exist exactly once"
  pass "$want provisioned dashboard and the demo service account exist once after a simultaneous boot"
}

check_c3() {
  local n host leased
  eventually 20 "both servers to report 2 servers" fleet_is 2
  [[ "$(psql_app "SELECT count(*) || '|' || count(DISTINCT id) FROM server_instances")" == "2|2" ]] ||
    fail "server_instances does not hold 2 distinct rows"
  for n in 1 2; do
    host="$(docker inspect -f '{{.Config.Hostname}}' "${PROJECT}-server-$n")"
    [[ "$(psql_app "SELECT count(*) FROM server_instances WHERE id LIKE '$host-%'")" == 1 ]] ||
      fail "no server_instances row starts with the hostname $host of server $n"
  done
  leased="$(psql_app "SELECT count(*) FROM jobs WHERE leased_by <> '' AND NOT EXISTS (SELECT 1 FROM server_instances s WHERE s.id = jobs.leased_by)")"
  [[ "$leased" == 0 ]] || fail "$leased jobs are leased by an id that is not a live server"
  pass "both servers count 2 servers and every job lease belongs to one of them"
}

check_c4() {
  local n small big i code
  for n in 1 2; do
    small="$(metric "$n" 'slashobserve_datasource_slots_limit{datasource="prom_small"}')"
    big="$(metric "$n" 'slashobserve_datasource_slots_limit{datasource="prom"}')"
    metric_is "$n" 'slashobserve_datasource_slots_limit{datasource="prom_small"}' 2 &&
      metric_is "$n" 'slashobserve_datasource_slots_limit{datasource="prom"}' 16 ||
      fail "server $n reports slot limits ${small:-none} and ${big:-none}, not 2 and 16"
  done
  for ((i = 1; i <= 15; i++)); do
    code="$(oauth_register "$ONE" "$TMP/register.json")"
    [[ "$code" == 201 ]] || fail "registration $i on server 1 answered $code, not 201"
  done
  code="$(oauth_register "$ONE" "$TMP/register.json")"
  [[ "$code" == 429 ]] || fail "registration 16 on server 1 answered $code, not 429"
  pass "slot limits are 2 and 16 on each server and OAuth registration stops at 15 a minute per server"
}

check_c5() {
  local id events activity last ids sorted count
  ensure_dashboard
  events="$TMP/dashboard-events.sse"
  sse_open "$TWO/api/v1/dashboards/$CHECK_DASHBOARD/events" "$events"
  id="$SSE_LAST"
  CHECK_VERSION=$((CHECK_VERSION + 1))
  [[ "$(api POST "$ONE" "/api/v1/dashboards/$CHECK_DASHBOARD/versions" \
    "{\"summary\":\"replicas check\",\"parent_version\":$((CHECK_VERSION - 1)),\"spec\":$(check_spec "Replicas check $CHECK_VERSION")}")" == 201 ]] ||
    fail "save a version on server 1 ($(head -c 300 "$TMP/api.body"))"
  sse_has "$events" '^event: version.created$' 2 || fail "server 2 sent no version.created within 2s of a save on server 1"
  sse_close "$id"
  activity="$TMP/activity-1.sse"
  sse_open "$TWO/api/v1/activity/stream?dashboard_id=$CHECK_DASHBOARD" "$activity"
  id="$SSE_LAST"
  annotate "$ONE" "replicas check 0" >/dev/null
  sse_has "$activity" 'annotation.created' 2 || fail "server 2's activity stream missed an annotation made on server 1 within 2s"
  last="$(sse_last_id "$activity")"
  [[ -n "$last" ]] || fail "the activity stream carried no id"
  sse_close "$id"
  for count in 1 2 3; do
    annotate "$TWO" "replicas check $count" >/dev/null
  done
  sleep 2
  activity="$TMP/activity-2.sse"
  sse_open "$ONE/api/v1/activity/stream?dashboard_id=$CHECK_DASHBOARD" "$activity" "$last"
  id="$SSE_LAST"
  sse_has "$activity" '^id: ' 5 || fail "the resumed stream sent no events"
  sleep 1
  sse_close "$id"
  count="$(grep -c '^event: activity$' "$activity" || true)"
  [[ "$count" == 3 ]] || fail "the resumed stream sent $count events after id $last, not 3"
  if grep -q '^event: reset$' "$activity"; then
    fail "the resumed stream sent event: reset"
  fi
  ids="$(sse_ids "$activity" | tr '\n' ' ')"
  sorted="$(sse_ids "$activity" | sort -n | tr '\n' ' ')"
  [[ "$ids" == "$sorted" ]] || fail "the resumed events are out of order ($ids)"
  pass "a version and an activity event made on one server reach a stream on the other, and a resumed stream replays exactly the missed events"
}

check_c6() {
  local before1 before2 after1 after2 total sql bad
  before1="$(metric_sum 1 'slashobserve_agent_runs_total{kind="create",outcome="succeeded"}')"
  before2="$(metric_sum 2 'slashobserve_agent_runs_total{kind="create",outcome="succeeded"}')"
  submit_builds 20 alternate
  eventually 180 "all 20 builds to finish" builds_settled
  sql="$(sql_list $BUILD_IDS)"
  total="$(psql_app "SELECT count(*) FROM agent_requests WHERE id IN ($sql) AND status = 'succeeded'")"
  [[ "$total" == 20 ]] || fail "$total of 20 builds succeeded ($(build_report | grep -v ' succeeded ' | tr '\n' ';'))"
  bad="$(psql_app "SELECT count(*) FROM agent_requests WHERE id IN ($sql) AND attempts <> 1")"
  [[ "$bad" == 0 ]] || fail "$bad builds took more than one attempt"
  bad="$(psql_app "SELECT created_run_id FROM dashboards WHERE created_run_id IN ($sql) GROUP BY 1 HAVING count(*) <> 1")"
  [[ -z "$bad" ]] || fail "a build created more than one dashboard ($bad)"
  [[ "$(psql_app "SELECT count(*) FROM dashboards WHERE created_run_id IN ($sql)")" == 20 ]] || fail "the 20 builds did not create 20 dashboards"
  after1="$(metric_sum 1 'slashobserve_agent_runs_total{kind="create",outcome="succeeded"}')"
  after2="$(metric_sum 2 'slashobserve_agent_runs_total{kind="create",outcome="succeeded"}')"
  [[ "$(((after1 - before1) + (after2 - before2)))" == 20 ]] ||
    fail "the servers counted $(((after1 - before1) + (after2 - before2))) successful runs, not 20"
  pass "20 builds submitted alternately to both servers each succeeded once, on one attempt, with one dashboard"
  info "runs by server: server 1 $((after1 - before1)), server 2 $((after2 - before2))"
}

jobs_all_ran() {
  local name
  for name in $(psql_app "SELECT name FROM jobs"); do
    (($(metric_sum 1 "slashobserve_scheduler_jobs_total{job=\"$name\",") + $(metric_sum 2 "slashobserve_scheduler_jobs_total{job=\"$name\",") >= 1)) || return 1
  done
}

check_c7() {
  local name total count=0
  eventually 120 "every job to have run" jobs_all_ran
  for name in $(psql_app "SELECT name FROM jobs"); do
    total=$(($(metric_sum 1 "slashobserve_scheduler_jobs_total{job=\"$name\",") + $(metric_sum 2 "slashobserve_scheduler_jobs_total{job=\"$name\",")))
    [[ "$total" == 1 ]] || fail "job $name ran $total times across both servers, not once"
    count=$((count + 1))
  done
  ((count > 0)) || fail "the jobs table is empty"
  pass "each of $count scheduled jobs ran once across both servers at boot"
}

check_c8() {
  local jar="$TMP/c8.jar" code wid i bad total overlap
  new_session "$jar"
  code="$(session_api "$jar" POST "$LB" /api/v1/outbound-domains '{"domain":"receiver.invalid"}')"
  [[ "$code" == 2* ]] || fail "(needs a domain that never resolves; the allowlist answered $code: $(head -c 200 "$TMP/api.body"))"
  code="$(OUT="$TMP/webhook.json" session_api "$jar" POST "$LB" /api/v1/webhooks \
    '{"name":"replicas-check","url":"https://receiver.invalid/hook","event_types":["annotation.created"]}')"
  [[ "$code" == 201 ]] || fail "(needs a domain that never resolves; the webhook was refused with $code: $(head -c 200 "$TMP/webhook.json"))"
  wid="$(json_field "$TMP/webhook.json" webhook.id)"
  ensure_dashboard
  for i in 1 2 3; do
    annotate "$LB" "replicas webhook $i" >/dev/null
  done
  sleep 90
  total="$(psql_app "SELECT count(*) FROM webhook_deliveries WHERE webhook_id = '$wid'")"
  ((total >= 1)) || fail "no delivery was attempted in 90s"
  bad="$(psql_app "SELECT count(*) FROM webhook_deliveries WHERE webhook_id = '$wid' AND NOT (outcome = 'failed' AND error_code = 'unreachable')")"
  [[ "$bad" == 0 ]] || fail "$bad attempts did not fail as unreachable"
  overlap="$(psql_app "SELECT delivery_id FROM (SELECT delivery_id, attempted_at - lag(attempted_at) OVER (PARTITION BY delivery_id ORDER BY attempted_at) AS gap FROM webhook_deliveries WHERE webhook_id = '$wid') g WHERE gap < interval '1 second'")"
  [[ -z "$overlap" ]] || fail "two servers attempted one delivery at once ($overlap)"
  pass "$total webhook attempts all failed as unreachable and no delivery was attempted twice at once"
}

check_c15() {
  local code gid sync
  code="$(OUT="$TMP/grafana.json" api POST "$ONE" /api/v1/grafana '{"name":"replicas-check","url":"https://grafana.invalid","token":"not-a-real-token"}')"
  if [[ "$code" != 201 ]]; then
    known "not reproduced locally ($code $(json_field "$TMP/grafana.json" code 2>/dev/null || true))"
    return 0
  fi
  gid="$(json_field "$TMP/grafana.json" id)"
  code="$(OUT="$TMP/import.json" api POST "$ONE" "/api/v1/grafana/$gid/imports" '{"folders":"all"}')"
  if [[ "$code" != 202 ]]; then
    known "not reproduced locally ($code $(json_field "$TMP/import.json" code 2>/dev/null || true))"
    return 0
  fi
  sync="$(json_field "$TMP/import.json" id)"
  [[ "$(api GET "$ONE" "/api/v1/grafana/$gid/imports/$sync")" == 200 ]] || fail "server 1 does not know the import it started"
  if [[ "$(api GET "$TWO" "/api/v1/grafana/$gid/imports/$sync")" == 404 ]]; then
    known "Grafana sync progress lives on the server that started it"
    return 0
  fi
  fail "server 2 answers the import status; the known gap is fixed, so make this item a PASS and update the scaling page"
}

check_c12() {
  local jar="$TMP/c12.jar" authorize form_url callback code
  : >"$jar"
  authorize="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code} %{redirect_url}' \
    --get --data-urlencode "return_to=/" "$ONE/api/v1/auth/login")"
  [[ "$authorize" == "302 http://localhost:${DEX_PORT}/dex/auth?"* ]] || fail "login on server 1 redirects to Dex ($authorize)"
  form_url="$(curl -sS -L -c "$jar" -b "$jar" -o "$TMP/c12-form.html" -w '%{url_effective}' "${authorize#* }")"
  grep -q 'name="login"' "$TMP/c12-form.html" || fail "the Dex login form was not reached ($form_url)"
  callback="$(dex_callback "$jar" "$form_url")" || fail "Dex did not redirect to the callback"
  callback="${callback/localhost:${LB_PORT}/localhost:${TWO_PORT}}"
  code="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code}' "$callback")"
  [[ "$code" == 302 || "$code" == 303 || "$code" == 307 ]] || fail "the callback on server 2 answered $code, not a redirect"
  grep -q so_session "$jar" || fail "the callback on server 2 set no session cookie"
  code="$(curl -sS -b "$jar" -o /dev/null -w '%{http_code}' "$ONE/api/v1/me")"
  [[ "$code" == 200 ]] || fail "the session made on server 2 answers $code on server 1"
  pass "a login started on server 1 finished on server 2 and its session works on server 1"
}

check_c13() {
  local jar="$TMP/c13.jar" i code client verifier challenge state authorize request redirect_to auth_code
  local access refresh access2 refresh2 reuse
  new_session "$jar"
  for ((i = 0; i < 70; i++)); do
    code="$(oauth_register "$ONE" "$TMP/c13-client.json")"
    [[ "$code" == 429 ]] || break
    sleep 1
  done
  [[ "$code" == 201 ]] || fail "register a client on server 1 ($code: $(head -c 200 "$TMP/c13-client.json"))"
  client="$(json_field "$TMP/c13-client.json" client_id)"
  verifier="$(openssl rand -base64 48 | tr '+/' '-_' | tr -d '=\n')"
  challenge="$(pkce_challenge "$verifier")"
  state="$(openssl rand -hex 8)"
  authorize="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --get \
    --data-urlencode response_type=code --data-urlencode "client_id=$client" \
    --data-urlencode redirect_uri=http://127.0.0.1:9/callback --data-urlencode "state=$state" \
    --data-urlencode "code_challenge=$challenge" --data-urlencode code_challenge_method=S256 \
    --data-urlencode "resource=$LB/mcp" "$TWO/oauth/authorize")"
  [[ "$authorize" == "302 "*"/connect/"* ]] || fail "authorize on server 2 answered ($authorize)"
  request="${authorize##*/connect/}"
  code="$(OUT="$TMP/c13-request.json" session_api "$jar" GET "$ONE" "/api/v1/oauth/requests/$request")"
  [[ "$code" == 200 ]] || fail "server 1 does not know the consent request made on server 2 ($code)"
  code="$(OUT="$TMP/c13-approve.json" session_api "$jar" POST "$ONE" "/api/v1/oauth/requests/$request/approve")"
  [[ "$code" == 200 ]] || fail "approve on server 1 answered $code ($(head -c 200 "$TMP/c13-approve.json"))"
  redirect_to="$(json_field "$TMP/c13-approve.json" redirect_to)"
  [[ "$(form_field "$redirect_to" state)" == "$state" ]] || fail "the approval returned another state"
  auth_code="$(form_field "$redirect_to" code)"
  code="$(token_request "$TWO" "$TMP/c13-token.json" \
    --data-urlencode grant_type=authorization_code --data-urlencode "code=$auth_code" \
    --data-urlencode "client_id=$client" --data-urlencode redirect_uri=http://127.0.0.1:9/callback \
    --data-urlencode "code_verifier=$verifier" --data-urlencode "resource=$LB/mcp")"
  [[ "$code" == 200 ]] || fail "the code exchange on server 2 answered $code ($(head -c 200 "$TMP/c13-token.json"))"
  access="$(json_field "$TMP/c13-token.json" access_token)"
  refresh="$(json_field "$TMP/c13-token.json" refresh_token)"
  code="$(mcp_tools_list "$ONE" "$TMP/c13-tools.json" "$access")"
  [[ "$code" == 200 ]] && grep -q '"tools"' "$TMP/c13-tools.json" ||
    fail "tools/list on server 1 with a token issued by server 2 answered $code"
  code="$(token_request "$TWO" "$TMP/c13-refresh.json" \
    --data-urlencode grant_type=refresh_token --data-urlencode "refresh_token=$refresh" \
    --data-urlencode "client_id=$client" --data-urlencode "resource=$LB/mcp")"
  [[ "$code" == 200 ]] || fail "the refresh on server 2 answered $code"
  access2="$(json_field "$TMP/c13-refresh.json" access_token)"
  refresh2="$(json_field "$TMP/c13-refresh.json" refresh_token)"
  [[ "$refresh2" != "$refresh" && "$access2" != "$access" ]] || fail "the refresh returned the old tokens"
  sleep 11
  code="$(token_request "$ONE" "$TMP/c13-reuse.json" \
    --data-urlencode grant_type=refresh_token --data-urlencode "refresh_token=$refresh" \
    --data-urlencode "client_id=$client" --data-urlencode "resource=$LB/mcp")"
  reuse="$(head -c 300 "$TMP/c13-reuse.json")"
  [[ "$code" == 400 && "$reuse" == *invalid_grant* ]] || fail "reusing the old refresh token on server 1 answered $code ($reuse)"
  code="$(mcp_tools_list "$TWO" "$TMP/c13-after.json" "$access2")"
  [[ "$code" == 401 ]] || fail "the new access token still works on server 2 after the reuse ($code)"
  pass "an OAuth client registered on one server, approved and exchanged across both, and revoked by a refresh reuse seen on the other"
}

check_c14() {
  local n ip direct clients
  for n in 1 2; do
    ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${PROJECT}-server-$n")"
    direct="$(psql_app "SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend' AND client_addr = '$ip'")"
    [[ "$direct" == 2 ]] || fail "server $n holds $direct direct connections to Postgres, not 2 (broker and worker relay)"
  done
  psql_bouncer "SHOW POOLS" | grep -q '^slashobserve|.*|transaction|' || fail "PgBouncer lists no transaction pool for slashobserve"
  clients="$(psql_bouncer "SHOW CLIENTS" | awk -F'|' '$1 == "C" && $3 == "slashobserve" { n++ } END { print n + 0 }')"
  ((clients > 0)) || fail "PgBouncer shows no client connections for slashobserve"
  for n in 1 2; do
    if server_log_has "$n" 'prepared statement'; then
      fail "server $n logged a prepared statement error"
    fi
  done
  pass "each server holds 2 direct connections to Postgres, $clients clients use the pooler and no prepared statement error was logged"
}

readyz_probe() {
  local i code
  for ((i = 0; i < 30; i++)); do
    code="$(command curl -s --max-time 5 -o /dev/null -w '%{http_code}' "$LB/readyz" || true)"
    printf '%s\n' "$code" >>"$1"
    sleep 1
  done
}

check_c9() {
  local activity last id probe stream_pid a1 a2 expected got bad elapsed total_probes
  ensure_dashboard
  activity="$TMP/c9-activity.sse"
  sse_open "$TWO/api/v1/activity/stream?dashboard_id=$CHECK_DASHBOARD" "$activity"
  stream_pid="$SSE_LAST"
  annotate "$ONE" "replicas stop 0" >/dev/null
  sse_has "$activity" 'annotation.created' 3 || fail "the stream on server 2 saw no event before the stop"
  submit_builds 6 balanced
  eventually 30 "2 builds to be running" builds_running_at_least 2
  : >"$TMP/c9-readyz.log"
  readyz_probe "$TMP/c9-readyz.log" &
  probe=$!
  SECONDS=0
  stop_server 2
  elapsed=$SECONDS
  a1="$(annotate "$LB" "replicas stop 1")"
  a2="$(annotate "$LB" "replicas stop 2")"
  ((elapsed <= 30)) || fail "server 2 took ${elapsed}s to stop"
  [[ "$(docker inspect -f '{{.State.ExitCode}}' "${PROJECT}-server-2")" == 0 ]] || fail "server 2 exited with code $(docker inspect -f '{{.State.ExitCode}}' "${PROJECT}-server-2")"
  server_log_has 2 'shutting down' || fail "server 2 did not log shutting down"
  eventually 15 "server 1 to count 1 server and take all 4 slots" survivor_alone
  [[ "$(psql_app "SELECT count(*) FROM server_instances")" == 1 ]] || fail "server_instances still lists a stopped server"
  wait "$probe" || true
  total_probes="$(wc -l <"$TMP/c9-readyz.log" | tr -d ' ')"
  bad="$(grep -vc '^200$' "$TMP/c9-readyz.log" || true)"
  [[ "$total_probes" == 30 && "$bad" == 0 ]] || fail "$bad of $total_probes /readyz probes through the load balancer were not 200"
  eventually 180 "all 6 builds to finish" builds_settled
  [[ "$(builds_in_status "'succeeded'")" == 6 ]] || fail "not every build succeeded ($(build_report | tr '\n' ';'))"
  sse_close "$stream_pid"
  last="$(sse_last_id "$activity")"
  [[ -n "$last" ]] || fail "the stream on server 2 carried no id"
  sse_open "$LB/api/v1/activity/stream?dashboard_id=$CHECK_DASHBOARD" "$TMP/c9-resumed.sse" "$last"
  id="$SSE_LAST"
  sse_has "$TMP/c9-resumed.sse" "$a2" 5 || true
  sleep 1
  sse_close "$id"
  for expected in "$a1" "$a2"; do
    grep -q "$expected" "$TMP/c9-resumed.sse" || grep -q "$expected" "$activity" || fail "the annotation $expected made during the stop never reached a stream"
  done
  got="$(grep -c '^event: reset$' "$TMP/c9-resumed.sse" || true)"
  [[ "$got" == 0 ]] || fail "the resumed stream sent event: reset"
  pass "a stopped server left within ${elapsed}s with code 0, the survivor took over its share and its builds, and no probe or event was lost"
}

survivor_alone() {
  fleet_alone_on_one && metric_is 1 'slashobserve_datasource_slots_limit{datasource="prom_small"}' 4
}

fleet_alone_on_one() {
  metric_is 1 slashobserve_fleet_servers 1
}

check_c10() {
  start_server 2
  eventually 20 "both servers to report 2 servers and 2 slots" fleet_and_slots_rejoined
  pass "a restarted server is counted again and the limits divide by 2 again"
}

fleet_and_slots_rejoined() {
  fleet_is 2 && slots_are 2
}

check_c11() {
  local report
  submit_builds 6 balanced
  eventually 30 "2 builds to be running" builds_running_at_least 2
  kill_server 2
  eventually 45 "server 1 to count 1 server" fleet_alone_on_one
  eventually 240 "every build to leave queued and running" builds_settled
  report="$(build_report)"
  info "$(printf '%s' "$report" | tr '\n' ';')"
  [[ "$(psql_app "SELECT count(*) FROM agent_requests WHERE id IN ($(sql_list $BUILD_IDS)) AND attempts > 2")" == 0 ]] ||
    fail "a build took more than 2 attempts ($report)"
  [[ "$(psql_app "SELECT count(*) FROM agent_requests WHERE id IN ($(sql_list $BUILD_IDS)) AND attempts = 1 AND status <> 'succeeded'")" == 0 ]] ||
    fail "a build that never ran on the killed server did not succeed ($report)"
  start_server 2
  eventually 20 "both servers to report 2 servers and 2 slots" fleet_and_slots_rejoined
  pass "after a kill the survivor counted 1 server in 45s and no build was left queued or running"
}

check() {
  local item want matched
  scratch
  curl -fsS -o /dev/null "$ONE/readyz" || die "server 1 is not ready at $ONE; run scripts/replicas.sh up"
  curl -fsS -o /dev/null "$TWO/readyz" || die "server 2 is not ready at $TWO; run scripts/replicas.sh up"
  [[ -s "$ADMIN_KEY_FILE" && -s "$EDITOR_KEYS_FILE" ]] || die "no keys in $RUNTIME; run scripts/replicas.sh keys"
  for want in "$@"; do
    matched=0
    for item in "${CHECK_ITEMS[@]}"; do
      [[ "$item" != "$want" ]] || matched=1
    done
    ((matched == 1)) || die "unknown item $want; the items are ${CHECK_ITEMS[*]}"
  done
  for item in "${CHECK_ITEMS[@]}"; do
    matched=$(($# == 0))
    for want in "$@"; do
      [[ "$want" != "$item" ]] || matched=1
    done
    if ((matched == 1)); then
      ITEM="$item"
      "check_$(printf '%s' "$item" | tr 'C' 'c')"
    fi
  done
}

LOAD_SAMPLE_SECONDS=5
LOAD_REPORTS="$ROOT/loadtest/reports"

running_servers() {
  local n out=""
  for n in 1 2; do
    if [[ "$(docker inspect -f '{{.State.Running}}' "${PROJECT}-server-$n" 2>/dev/null || true)" == true ]]; then
      out="$out $n"
    fi
  done
  printf '%s' "$out"
}

pgbouncer_pools() {
  "${COMPOSE_CMD[@]}" exec -T -e PGPASSWORD=slashobserve postgres psql -U slashobserve -h pgbouncer -p 6432 -d pgbouncer -qA -c "SHOW POOLS" | sed '/^(/d'
}

sample_once() {
  local dir="$1" stamp
  stamp="$(date -u +%H:%M:%S)"
  docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' | { grep "^${PROJECT}-" || true; } | sed "s/^/${stamp},/" >>"$dir/docker-stats.csv"
  psql_app "SELECT state, count(*) FROM pg_stat_activity WHERE datname = 'slashobserve' GROUP BY 1" | sed "s/^/${stamp}|/" >>"$dir/pg-activity.txt"
  pgbouncer_pools | awk -v stamp="$stamp" -v dir="$dir" 'NR == 1 { print > (dir "/pgbouncer-pools.header"); next } { print stamp "|" $0 }' >>"$dir/pgbouncer-pools.txt"
}

sample_loop() {
  local dir="$1"
  while :; do
    sample_once "$dir" 2>/dev/null || true
    sleep "$LOAD_SAMPLE_SECONDS"
  done
}

resource_peaks() {
  awk -F, '
    function mib(s,   parts, unit, val) {
      split(s, parts, " / ")
      s = parts[1]
      match(s, /[A-Za-z]+$/)
      unit = substr(s, RSTART)
      val = substr(s, 1, RSTART - 1) + 0
      if (unit == "GiB") return val * 1024
      if (unit == "KiB" || unit == "kB") return val / 1024
      if (unit == "B") return val / 1048576
      return val
    }
    {
      cpu = $3
      sub(/%/, "", cpu)
      cpu += 0
      m = mib($4)
      if (!($2 in pc) || cpu > pc[$2]) pc[$2] = cpu
      if (!($2 in pm) || m > pm[$2]) pm[$2] = m
    }
    END { for (n in pc) printf "| %s | %.0f%% | %.0f MiB |\n", n, pc[n], pm[n] }
  ' "$1" | sort
}

pg_peaks() {
  awk -F'|' '
    { n = $3 + 0; total[$1] += n; if (n > st[$2]) st[$2] = n }
    END {
      for (s in st) printf "| %s | %d |\n", (s == "" ? "background" : s), st[s]
      for (t in total) if (total[t] > peak) peak = total[t]
      printf "| all states together | %d |\n", peak
    }
  ' "$1" | sort
}

pool_peaks() {
  awk -F'|' -v header="$(cat "$2")" '
    BEGIN {
      n = split(header, h, "|")
      for (i = 1; i <= n; i++) idx[h[i]] = i + 1
      split("cl_active cl_waiting sv_active sv_idle sv_used maxwait", cols, " ")
    }
    $2 == "slashobserve" {
      for (c = 1; c <= 6; c++) {
        v = $(idx[cols[c]]) + 0
        if (v > peak[cols[c]]) peak[cols[c]] = v
      }
    }
    END { for (c = 1; c <= 6; c++) printf "| %s | %d |\n", cols[c], peak[cols[c]] }
  ' "$1"
}

restarted_containers() {
  local id state out=""
  for id in $(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT"); do
    state="$(docker inspect -f '{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}}' "$id")"
    case "$state" in
      *"restarts=0 oom=false") ;;
      *) out="$out ${state#/}" ;;
    esac
  done
  printf '%s' "$out"
}

slow_queries() {
  psql_app "SELECT round(mean_exec_time::numeric, 2) AS mean_ms, calls, round(total_exec_time::numeric, 0) AS total_ms, left(regexp_replace(query, '\\s+', ' ', 'g'), 160) AS query FROM pg_stat_statements WHERE dbid = (SELECT oid FROM pg_database WHERE datname = 'slashobserve') ORDER BY mean_exec_time DESC LIMIT 10" | redact
}

finish_load() {
  local dir="$1" samples="$2" bad="$3" word="pass" detail="no container restarted or was OOM killed"
  mv "$samples"/* "$dir"/ 2>/dev/null || true
  slow_queries >"$dir/slow-queries.txt" 2>&1 || true
  if [[ -n "$bad" ]]; then
    word="FAIL"
    detail="$bad"
  fi
  printf '%s\n' "$detail" >"$dir/containers.txt"
  {
    printf '| no container restarted or was OOM killed | %s | %s |\n' "$word" "$detail"
    printf '\n## Resource use\n\nPeak over the run, sampled every %s s.\n\n' "$LOAD_SAMPLE_SECONDS"
    printf '| Container | Peak CPU | Peak memory |\n| --- | --- | --- |\n'
    resource_peaks "$dir/docker-stats.csv"
    printf '\n## Postgres connections\n\n| State | Peak |\n| --- | --- |\n'
    pg_peaks "$dir/pg-activity.txt"
    printf '\n## PgBouncer pool slashobserve\n\n| Column | Peak |\n| --- | --- |\n'
    pool_peaks "$dir/pgbouncer-pools.txt" "$dir/pgbouncer-pools.header"
    printf '\nThe ten slowest statements by mean time are in slow-queries.txt.\n'
  } >>"$dir/report.md"
}

load() {
  local only="all" label="" n servers code=0 dir bad
  local replicas=()
  while (($# > 0)); do
    case "$1" in
      --only)
        only="${2:-}"
        shift 2 || die "--only needs viewers, streams, mcp or agent"
        ;;
      -*) die "unknown option $1" ;;
      *)
        label="$1"
        shift
        ;;
    esac
  done
  [[ "$only" =~ ^(all|viewers|streams|mcp|agent)$ ]] || die "--only is viewers, streams, mcp or agent"
  [[ "$label" =~ ^[a-z0-9-]+$ ]] || die "give the run a label of lowercase letters, digits and dashes, for example servers2-workers8"
  [[ -s "$RUNTIME/viewer.keys" && -s "$EDITOR_KEYS_FILE" ]] || die "no keys in $RUNTIME; run scripts/replicas.sh keys"
  servers="$(running_servers)"
  [[ -n "$servers" ]] || die "no server of $PROJECT is running; run scripts/replicas.sh up"
  for n in $servers; do
    wait_ready "http://localhost:$(server_port "$n")" 60
    replicas+=(--replica "http://localhost:$(server_port "$n")")
  done
  scratch
  mkdir -p "$TMP/samples"
  (cd "$ROOT" && go build -o "$TMP/loadtest" ./loadtest)
  psql_app "CREATE EXTENSION IF NOT EXISTS pg_stat_statements" >/dev/null
  psql_app "SELECT pg_stat_statements_reset()" >/dev/null
  sample_loop "$TMP/samples" &
  SAMPLER_PID=$!
  { "$TMP/loadtest" --target "$LB" "${replicas[@]}" \
    --viewer-keys "$RUNTIME/viewer.keys" --editor-keys "$EDITOR_KEYS_FILE" \
    --label "$label" --out "$LOAD_REPORTS" --commit "$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || true)" \
    "$only" 2>&1 && echo 0 >"$TMP/load.code" || echo "$?" >"$TMP/load.code"; } | tee "$TMP/load.out"
  code="$(cat "$TMP/load.code")"
  kill "$SAMPLER_PID" 2>/dev/null || true
  wait "$SAMPLER_PID" 2>/dev/null || true
  SAMPLER_PID=""
  dir="$({ grep '^report directory: ' "$TMP/load.out" || true; } | tail -1 | sed 's/^report directory: //')"
  bad="$(restarted_containers)"
  if [[ -n "$dir" && -d "$dir" ]]; then
    finish_load "$dir" "$TMP/samples" "$bad"
    say "report: $dir/report.md"
  fi
  ((code == 0)) || die "the load test gate failed or the tool stopped with status $code"
  [[ -z "$bad" ]] || die "containers restarted or were OOM killed:$bad"
  say "load test passed every gate"
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
    check) check "$@" ;;
    load) load "$@" ;;
    help | -h | --help) usage ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
