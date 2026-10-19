#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="slashobserve-e2e"
PORT="${SLASHOBSERVE_E2E_PORT:-18080}"
BASE_URL="http://localhost:${PORT}"
export SLASHOBSERVE_HOST_PORT="$PORT"
DEX_PORT="${SLASHOBSERVE_E2E_DEX_PORT:-15556}"
export SLASHOBSERVE_DEX_HOST_PORT="$DEX_PORT"
export SLASHOBSERVE_PROMETHEUS_HOST_PORT="${SLASHOBSERVE_E2E_PROMETHEUS_PORT:-19090}"
export SLASHOBSERVE_SHOP_HOST_PORT="${SLASHOBSERVE_E2E_SHOP_PORT:-18081}"
export SLASHOBSERVE_DATASOURCES_HOST_FILE=./datasources.e2e.yaml
export SLASHOBSERVE_API_CONTRACT_CHECK=true
export SLASHOBSERVE_LLM_PROVIDER=replay

curl() { command curl --max-time 10 "$@"; }

read -r -d '' REDACT_PERL <<'PERL' || true
BEGIN { $lit = $ENV{REDACT_LITERAL} // ''; }
my $key = qr/(?!\w*tokens["']?\s*[:=]\s*\d+(?![\w.~+\/=-]))\w*(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|access[_-]?key)\w*/i;
s{(://)[^/\s:\@]+:[^/\s\@]+\@}{${1}\[redacted]\@}g;
s/sok_[A-Za-z0-9_-]+/sok_\[redacted]/gi;
s/(Bearer\s+)[A-Za-z0-9._~+\/=-]+/${1}\[redacted]/gi;
s/\b((?:authorization|cookie|set-cookie|x-api-key|so_session)["']?\s*[:=]\s*)(?:(["'])(?:(?:\\.|(?!\2)[^\\])*\2|(?:(?!\2).)*\2)|.*)/defined $2 ? "${1}${2}\[redacted]${2}" : "${1}\[redacted]"/gie;
s/(so_session\s+)\S+/${1}\[redacted]/gi;
s/(--password)[ =]\S+/${1} \[redacted]/gi;
s/($key)(["']?\s*[:=]\s*)(["'])(?:(?:\\.|(?!\3)[^\\])*\3|(?:(?!\3).)*\3)/${1}${2}${3}\[redacted]${3}/g;
s/($key)(["']?\s*[:=]\s*)(?!\[redacted\])[^\s,;&"'}\])]+/${1}${2}\[redacted]/g;
s/([?&]code=)[^&\s#"']+/${1}\[redacted]/gi;
s/\Q$lit\E/\[redacted]/g if length $lit;
PERL

redact() {
  REDACT_LITERAL="${API_TOKEN:-}" perl -pe "$REDACT_PERL"
}

pass() { printf 'PASS  %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n' "$(printf '%s' "$1" | redact)" >&2
  exit 1
}

if [[ -n "${SLASHOBSERVE_IMAGE:-}" ]]; then
  BUILD_FLAGS=(--no-build --pull missing)
else
  export SLASHOBSERVE_IMAGE="slashobserve-e2e:local"
  export SLASHOBSERVE_SHOP_IMAGE="slashobserve-e2e-shop:local"
  BUILD_FLAGS=(--build)
fi

usage() {
  cat <<EOF
Usage: scripts/e2e.sh [run|up|browser|down|logs]

  run      reset the isolated ${PROJECT} compose stack, boot it, and run the
           API smoke suite (default); the stack is left running
  up       reuse the running, healthy ${PROJECT} stack, or reset and boot one;
           reuse does not rebuild, so rerun 'run' after code changes
  browser  'up', then run the Playwright suite in web/e2e against it
  down     tear the stack down, including volumes
  logs     print the stack's container logs

Environment:
  SLASHOBSERVE_E2E_PORT             host port of the e2e server (default 18080)
  SLASHOBSERVE_E2E_DEX_PORT         host port of the e2e Dex (default 15556)
  SLASHOBSERVE_E2E_PROMETHEUS_PORT  host port of the e2e Prometheus (default 19090)
  SLASHOBSERVE_E2E_SHOP_PORT        host port of the e2e shop fault API (default 18081)
  SLASHOBSERVE_IMAGE                run this prebuilt server image instead of building the checkout
  SLASHOBSERVE_SHOP_IMAGE           the prebuilt demo shop image, required with SLASHOBSERVE_IMAGE

Requirements: docker with compose v2.20+, curl, perl; 'browser' also needs bun and web dependencies.
EOF
}

COMPOSE_CMD=(docker compose -f "$ROOT/deploy/compose/compose.yaml" -p "$PROJECT")

read -r -d '' BOUNDED_PERL <<'PERL' || true
my $secs = shift @ARGV;
my $pid = fork();
defined $pid or exit 125;
if ($pid == 0) {
  setpgrp(0, 0);
  exec { $ARGV[0] } @ARGV;
  print STDERR "cannot run $ARGV[0]: $!\n";
  exit 127;
}
setpgrp($pid, $pid);
my ($timed_out, $interrupted, $escalated) = (0, 0, 0);
my $stop = sub {
  kill($escalated++ ? 'KILL' : 'TERM', -$pid);
  alarm 5;
};
$SIG{ALRM} = sub { $timed_out = 1 unless $interrupted; $stop->() };
my %signo = (HUP => 1, INT => 2, TERM => 15);
for my $name (keys %signo) {
  $SIG{$name} = sub { return if $interrupted; $interrupted = $signo{$name}; $stop->() unless $escalated };
}
alarm $secs;
my $reaped;
do { $reaped = waitpid($pid, 0) } while ($reaped == -1 && $!{EINTR});
my $status = $?;
select(undef, undef, undef, 0.1) while $escalated && kill(0, -$pid);
exit(128 + $interrupted) if $interrupted;
exit 124 if $timed_out && $status != 0;
exit($status & 127 ? 128 + ($status & 127) : $status >> 8);
PERL

CHILD_PID=""
foreground() {
  local rc=0
  "$@" 9>&- &
  CHILD_PID=$!
  wait "$CHILD_PID" || rc=$?
  CHILD_PID=""
  return "$rc"
}

bounded() {
  foreground perl -e "$BOUNDED_PERL" "$@"
}

grouped() {
  bounded 0 "$@"
}

EXEC_TIMEOUT=60
PS_TIMEOUT=30
LOGS_TIMEOUT=30
TIMED_OUT=124

compose_exec() {
  bounded "$EXEC_TIMEOUT" "${COMPOSE_CMD[@]}" exec -T "$@" </dev/null
}

compose_ps() {
  bounded "$PS_TIMEOUT" "${COMPOSE_CMD[@]}" ps "$@" </dev/null
}

compose_logs() {
  bounded "$LOGS_TIMEOUT" "${COMPOSE_CMD[@]}" logs --no-color "$@" </dev/null 2>&1
}

dump_logs() {
  compose_logs --tail "$1" >"$OUT_FILE" || true
  redact <"$OUT_FILE" >&2
}

LOGS_ALL_TIMEOUT=120

all_logs() {
  local rc=0
  LOGS_TIMEOUT="$LOGS_ALL_TIMEOUT" compose_logs --timestamps >"$OUT_FILE" || rc=$?
  redact <"$OUT_FILE"
  if ((rc == TIMED_OUT)); then echo "docker compose logs timed out after ${LOGS_ALL_TIMEOUT}s" >&2; fi
  return "$rc"
}

logs_into() {
  local var="$1" rc=0
  shift
  capture "$var" compose_logs "$@" || rc=$?
  ((rc != TIMED_OUT)) || fail "docker compose logs timed out after ${LOGS_TIMEOUT}s"
}

PROM_URL="http://localhost:${SLASHOBSERVE_PROMETHEUS_HOST_PORT}"
SHOP_URL="http://localhost:${SLASHOBSERVE_SHOP_HOST_PORT}"
FAULT_SET=0

prom_query() {
  foreground command curl -fsS --max-time 5 -o "$TMP/prom.body" "$PROM_URL/api/v1/query" --data-urlencode "query=$1"
}

prom_scalar() {
  prom_query "scalar($1)" || return 0
  sed -nE 's/.*"result":\[[^,]*,"([^"]*)"\].*/\1/p' "$TMP/prom.body" || true
}

ch() {
  local rc=0
  compose_exec clickhouse sh -c \
    'exec clickhouse-client --connect_timeout=5 --receive_timeout=10 --user slashobserve_ro --password "$SLASHOBSERVE_RO_PASSWORD" -q "$1"' \
    sh "$1" || rc=$?
  if ((rc == TIMED_OUT)); then echo "docker compose exec clickhouse timed out after ${EXEC_TIMEOUT}s" >&2; fi
  return "$rc"
}

OUT_FILE=""
ERR_FILE=""
LAST=""
LAST_ERR=""

capture() {
  local var="$1" rc=0
  shift
  "$@" >"$OUT_FILE" 2>"$ERR_FILE" || rc=$?
  printf -v "$var" '%s' "$(<"$OUT_FILE")"
  LAST_ERR="$(tail -3 "$ERR_FILE" | redact | cut -c1-300)"
  return "$rc"
}

capture_merged() {
  local var="$1" rc=0
  shift
  "$@" >"$OUT_FILE" 2>&1 || rc=$?
  printf -v "$var" '%s' "$(<"$OUT_FILE")"
  return "$rc"
}

above() { awk -v v="$1" -v t="$2" 'BEGIN { exit !(v + 0 > t + 0) }'; }
positive() { above "$1" 0; }
prom_is() { capture LAST prom_scalar "$1" || true; [[ "$LAST" == "$2" ]]; }
prom_positive() { prom_above "$1" 0; }
prom_above() { capture LAST prom_scalar "$1" || true; above "$LAST" "$2"; }
ch_is() { capture LAST ch "$1" || true; [[ "$LAST" == "$2" ]]; }
ch_positive() { capture LAST ch "$1" || true; positive "$LAST"; }

poll_try() {
  local timeout="$1" interval="$2"
  shift 2
  local deadline=$((SECONDS + timeout))
  LAST=""
  LAST_ERR=""
  until "$@"; do
    ((SECONDS < deadline)) || return 1
    foreground sleep "$interval"
  done
}

poll() {
  local timeout="$1" desc="$2" detail=""
  shift 2
  poll_try "$timeout" 2 "$@" && return 0
  dump_logs 50
  if [[ -n "$LAST_ERR" ]]; then detail=", last error: ${LAST_ERR}"; fi
  fail "$desc (not true after ${timeout}s, last observed: '${LAST}'${detail})"
}

is_number() { [[ "$1" =~ ^[0-9.eE+-]+$ ]]; }
in_band() { awk -v v="$1" -v lo="$2" -v hi="$3" 'BEGIN { exit !(v ~ /^[0-9.eE+-]+$/ && v + 0 >= lo && v + 0 <= hi) }'; }

RATE_BAND_LO=0.85
RATE_BAND_HI=1.15
P95_BAND_LO=0.95
P95_BAND_HI=1.6
BUSY_RPS=1.5

junction_rate() { printf 'sum by (service, route, pod) (rate(http_requests_total{job="shop"}[1m]%s))' "$1"; }
junction_p95() { printf 'histogram_quantile(0.95, sum by (le, service, route, pod) (rate(http_request_duration_seconds_bucket{job="shop"}[1m]%s)))' "$1"; }

junction_inner() {
  local series="$1" before_at="$2" probe_at="$3" agg="$4"
  printf '%s_over_time(((%s / %s) and on (service, route, pod) (%s > %s))[2m:5s] @ %s)' \
    "$agg" "$("$series" "")" "$("$series" " @ $before_at")" "$(junction_rate " @ $before_at")" "$BUSY_RPS" "$probe_at"
}

junction_extreme() {
  printf '%s(%s)' "$4" "$(junction_inner "$@")"
}

prom_worst() {
  prom_query "$1" || { echo "query failed"; return 0; }
  sed -nE 's/.*"metric":\{([^}]*)\}.*"value":\[[^,]*,"([^"]*)"\].*/\1 = \2/p' "$TMP/prom.body" | tr -d '"' || true
}

junction_offenders() {
  local before_at="$1" probe_at="$2" kind series lo hi agg pick worst value out=""
  for kind in rate p95; do
    series="junction_${kind}"
    if [[ "$kind" == rate ]]; then lo="$RATE_BAND_LO"; hi="$RATE_BAND_HI"; else lo="$P95_BAND_LO"; hi="$P95_BAND_HI"; fi
    for agg in min max; do
      if [[ "$agg" == min ]]; then pick=bottomk; else pick=topk; fi
      capture worst prom_worst "${pick}(1, $(junction_inner "$series" "$before_at" "$probe_at" "$agg"))" || true
      value="${worst##* = }"
      if ! in_band "$value" "$lo" "$hi"; then out+="; ${kind} ${agg}: ${worst:-no series}"; fi
    done
  done
  printf '%s' "$out"
}

loadgen_dropped() {
  local logs dropped
  capture_merged logs compose_logs --tail 100 loadgen || true
  dropped="$(printf '%s\n' "$logs" | sed -nE 's/.*"msg":"loadgen summary".*"dropped":([0-9]+).*/\1/p' | tail -1)"
  printf '%s' "${dropped:-unknown}"
}

backfill_joins_live() {
  local boot end before_at probe_at rate_min rate_max p95_min p95_max
  capture boot prom_scalar 'process_start_time_seconds{job="prometheus"}' || true
  [[ "$boot" =~ ^[0-9.]+$ ]] || return 1
  capture end compose_exec prometheus cat /prometheus/backfill-end || true
  [[ "$end" =~ ^[0-9]+$ ]] || return 1
  before_at=$((end - 35))
  probe_at=$((end + 90))
  if (($(date +%s) < probe_at + 10)); then
    LAST="waiting for the window ending $((probe_at + 10 - $(date +%s)))s from now"
    return 1
  fi
  capture rate_min prom_scalar "$(junction_extreme junction_rate "$before_at" "$probe_at" min)" || true
  capture rate_max prom_scalar "$(junction_extreme junction_rate "$before_at" "$probe_at" max)" || true
  capture p95_min prom_scalar "$(junction_extreme junction_p95 "$before_at" "$probe_at" min)" || true
  capture p95_max prom_scalar "$(junction_extreme junction_p95 "$before_at" "$probe_at" max)" || true
  LAST="boot ${boot}, end ${end}: per-series request rate ratio [${rate_min}, ${rate_max}], p95 ratio [${p95_min}, ${p95_max}]"
  if in_band "$rate_min" "$RATE_BAND_LO" "$RATE_BAND_HI" && in_band "$rate_max" "$RATE_BAND_LO" "$RATE_BAND_HI" \
    && in_band "$p95_min" "$P95_BAND_LO" "$P95_BAND_HI" && in_band "$p95_max" "$P95_BAND_LO" "$P95_BAND_HI"; then
    return 0
  fi
  local offenders
  offenders="$(junction_offenders "$before_at" "$probe_at" | cut -c3-)"
  LAST="${LAST}, out of band: ${offenders:-none on re-query}, loadgen dropped $(loadgen_dropped) requests as of its last summary"
  if is_number "$rate_min" && is_number "$rate_max" && is_number "$p95_min" && is_number "$p95_max"; then
    dump_logs 50
    fail "backfilled request rate and p95 latency continue into live data (out of band once the window settled, last observed: '${LAST}')"
  fi
  return 1
}

targets_scraped() {
  prom_is 'count(up{job=~"shop|prometheus|node|cadvisor|otel-collector"} == 1)' 8
}

logs_and_traces_present() {
  ch_is "SELECT (SELECT count() FROM otel.otel_logs) > 0 AND (SELECT count() FROM otel.otel_traces) > 0" 1
}

wait_for_data() {
  poll 180 "Prometheus scraped all targets" targets_scraped
  pass "Prometheus scraped all targets"
  poll 180 "ClickHouse has log and trace rows" logs_and_traces_present
  pass "ClickHouse has log and trace rows"
}

is_ready() { capture LAST foreground command curl -fsS --max-time 3 "$BASE_URL/readyz"; }

stack_reusable() {
  local running rc=0
  capture running compose_ps --status running -q server || rc=$?
  ((rc != TIMED_OUT)) || fail "docker compose ps timed out after ${PS_TIMEOUT}s"
  [[ -n "$running" ]] && is_ready
}

boot() {
  [[ -n "${SLASHOBSERVE_SHOP_IMAGE:-}" ]] || fail "SLASHOBSERVE_SHOP_IMAGE must be set with SLASHOBSERVE_IMAGE"
  grouped "${COMPOSE_CMD[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  if ! grouped "${COMPOSE_CMD[@]}" up -d "${BUILD_FLAGS[@]}" --wait --wait-timeout 300; then
    dump_logs 200
    fail "stack did not become healthy"
  fi
  if poll_try 60 1 is_ready; then
    wait_for_data
    return 0
  fi
  fail "server ready ($BASE_URL/readyz did not return 200)"
}

TMP=""
LOCK_FILE=/tmp/slashobserve-e2e-run.lock
SSE_PID=""
CLAIM_LOCK_APP="slashobserve-e2e-claim-lock"
CLAIM_LOCK_PID=""

lock_owner() {
  local owner="" tries=10
  while ((tries-- > 0)); do
    owner="$(tr -cd '0-9' <"$LOCK_FILE" 2>/dev/null || true)"
    [[ -z "$owner" ]] || break
    sleep 0.2
  done
  if [[ -z "$owner" ]]; then
    printf 'pid unknown'
  elif kill -0 "$owner" 2>/dev/null || [[ -n "$(ps -p "$owner" -o pid= 2>/dev/null)" ]]; then
    printf 'pid %s' "$owner"
  else
    printf 'pid %s exited; a leftover child still holds the lock, retry shortly' "$owner"
  fi
}

read -r -d '' LOCK_PERL <<'PERL' || true
use Fcntl qw(:flock);
my ($path, $pid) = @ARGV;
open(my $f, ">&=", 9) or exit 2;
flock($f, LOCK_EX | LOCK_NB) or exit($!{EWOULDBLOCK} ? 1 : 2);
my @held = stat($f);
my @named = lstat($path);
exit 3 unless @named && !-l _ && $held[0] == $named[0] && $held[1] == $named[1];
truncate($f, 0) or exit 2;
syswrite($f, "$pid\n") or exit 2;
chmod(0666, $f);
PERL

acquire_lock() {
  local rc=0
  [[ ! -L "$LOCK_FILE" ]] || fail "$LOCK_FILE is a symlink; remove it"
  [[ ! -d "$LOCK_FILE" ]] || fail "stale directory from an older e2e.sh at $LOCK_FILE; rmdir it"
  { exec 9>>"$LOCK_FILE"; } 2>/dev/null || fail "cannot open $LOCK_FILE"
  perl -e "$LOCK_PERL" "$LOCK_FILE" "$$" || rc=$?
  case "$rc" in
    0) ;;
    1) fail "another e2e run is active ($(lock_owner))" ;;
    3) fail "$LOCK_FILE was replaced while locking it; retry" ;;
    *) fail "flock failed (exit $rc)" ;;
  esac
}

cleanup() {
  trap '' INT TERM HUP
  sse_close
  if [[ -n "$CHILD_PID" ]]; then
    kill "$CHILD_PID" 2>/dev/null || true
    wait "$CHILD_PID" 2>/dev/null || true
  fi
  release_claim_lock
  if [[ "$FAULT_SET" == 1 ]]; then command curl -s --max-time 5 -X DELETE "$SHOP_URL/fault" >/dev/null 2>&1 || true; fi
  if [[ -n "$TMP" ]]; then rm -rf "$TMP"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

psql_app() {
  compose_exec postgres psql -U slashobserve -d slashobserve -qAt -c "$1"
}

hold_claim_lock() {
  "${COMPOSE_CMD[@]}" exec -T -e PGAPPNAME="$CLAIM_LOCK_APP" postgres psql -U slashobserve -d slashobserve -qAt \
    -c "SELECT pg_advisory_lock(hashtextextended('agent_requests:claim', 0)), pg_sleep(600)" \
    </dev/null >"$TMP/claim-lock.log" 2>&1 9>&- &
  CLAIM_LOCK_PID=$!
}

claim_lock_log() {
  [[ -f "$TMP/claim-lock.log" ]] || return 0
  redact <"$TMP/claim-lock.log" | tr '\n' ' '
}

claim_lock_held() {
  local count
  capture count psql_app "SELECT count(*) FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid WHERE l.locktype = 'advisory' AND l.granted AND a.application_name = '$CLAIM_LOCK_APP'" || true
  LAST="$count (lock session output: $(claim_lock_log))"
  [[ "$count" == 1 ]]
}

release_claim_lock() {
  [[ -n "$CLAIM_LOCK_PID" ]] || return 0
  psql_app "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = '$CLAIM_LOCK_APP'" >/dev/null 2>&1 || true
  kill "$CLAIM_LOCK_PID" 2>/dev/null || true
  wait "$CLAIM_LOCK_PID" 2>/dev/null || true
  CLAIM_LOCK_PID=""
}

dex_login() {
  local jar="$1" return_to="$2" expected="$3" user="${4:-admin@example.com}" authorize form_url final
  authorize="$(curl -sS -c "$jar" -b "$jar" -o /dev/null -w '%{http_code} %{redirect_url}' \
    --get --data-urlencode "return_to=$return_to" "$BASE_URL/api/v1/auth/login")"
  [[ "$authorize" == "302 http://localhost:${DEX_PORT}/dex/auth?"* ]] || fail "login redirects to Dex ($authorize)"
  form_url="$(curl -sS -L -c "$jar" -b "$jar" -o "$TMP/dex-form.html" -w '%{url_effective}' "${authorize#* }")"
  grep -q 'name="login"' "$TMP/dex-form.html" || fail "Dex login form not reached ($form_url)"
  final="$(curl -sS -L -c "$jar" -b "$jar" -o /dev/null -w '%{url_effective}' \
    --data-urlencode "login=$user" --data-urlencode password=password "$form_url")"
  [[ "$final" == "$expected" ]] || fail "code flow lands on $expected ($final)"
}

suite() {
  local out asset llm_line logs field

  out="$(curl -sS -o "$TMP/healthz" -w '%{http_code}' "$BASE_URL/healthz")"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"ok"' "$TMP/healthz"; then
    fail "/healthz ($out: $(cat "$TMP/healthz"))"
  fi
  pass "/healthz returns ok"

  out="$(curl -sS -o "$TMP/readyz" -w '%{http_code}' "$BASE_URL/readyz")"
  if [[ "$out" != 200 ]] || ! grep -q '"database":"ok"' "$TMP/readyz" || ! grep -q '"migrations":"ok"' "$TMP/readyz"; then
    fail "/readyz ($out: $(cat "$TMP/readyz"))"
  fi
  pass "/readyz reports database and migrations ok"

  logs_into logs server
  llm_line="$(grep -F '"msg":"llm provider configured"' <<<"$logs" || true)"
  if [[ "$llm_line" != *'"provider":"replay"'* ]]; then
    logs_into logs --tail 50 server
    fail "server did not boot with the replay LLM provider ($logs)"
  fi
  pass "server boots with the replay LLM provider"

  out="$(curl -sS -o "$TMP/index.html" -w '%{http_code} %{content_type}' "$BASE_URL/")"
  [[ "$out" == "200 text/html; charset=utf-8" ]] || fail "web app index ($out)"
  grep -q '<div id="root">' "$TMP/index.html" || fail "web app index has no #root element"
  pass "web app index served at /"

  asset="$(grep -o '/assets/[^"]*\.js' "$TMP/index.html" | head -1 || true)"
  [[ -n "$asset" ]] || fail "web app index references no /assets/*.js bundle"
  out="$(curl -sS -o /dev/null -D "$TMP/asset.headers" -w '%{http_code}' "$BASE_URL$asset")"
  [[ "$out" == 200 ]] || fail "asset $asset ($out)"
  grep -qi '^cache-control: public, max-age=31536000, immutable' "$TMP/asset.headers" \
    || fail "asset $asset is not cached as immutable"
  pass "hashed assets served with immutable caching"

  out="$(curl -sS -o "$TMP/deep.html" -w '%{http_code} %{content_type}' "$BASE_URL/dashboards/does-not-exist")"
  if [[ "$out" != "200 text/html; charset=utf-8" ]] || ! grep -q '<div id="root">' "$TMP/deep.html"; then
    fail "SPA fallback for deep links ($out)"
  fi
  pass "deep links fall back to the web app"

  out="$(curl -sS -o "$TMP/api404" -w '%{http_code} %{content_type}' "$BASE_URL/api/v1/does-not-exist")"
  [[ "$out" == "404 application/json; charset=utf-8" ]] || fail "unknown API path ($out)"
  pass "unknown /api paths return JSON 404"

  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/assets/does-not-exist.js")"
  [[ "$out" == 404 ]] || fail "missing asset ($out)"
  pass "missing assets return 404"

  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/robots.txt")"
  [[ "$out" == 404 ]] || fail "missing top-level file ($out)"
  pass "missing top-level files return 404"

  out="$(curl -sS -o "$TMP/dotted.html" -w '%{http_code} %{content_type}' "$BASE_URL/dashboards/v1.2")"
  if [[ "$out" != "200 text/html; charset=utf-8" ]] || ! grep -q '<div id="root">' "$TMP/dotted.html"; then
    fail "SPA fallback for nested paths with dots ($out)"
  fi
  pass "nested paths with dots fall back to the web app"

  curl -sS -o /dev/null -D "$TMP/index.headers" "$BASE_URL/"
  grep -qi '^x-frame-options: DENY' "$TMP/index.headers" || fail "X-Frame-Options header missing"
  grep -qi "^content-security-policy: frame-ancestors 'none'" "$TMP/index.headers" || fail "frame-ancestors CSP missing"
  pass "responses forbid framing"

  out="$(curl -sS -o "$TMP/me-anon" -D "$TMP/me-anon.headers" -w '%{http_code}' "$BASE_URL/api/v1/me")"
  if [[ "$out" != 401 ]] || ! grep -q '"code":"unauthenticated"' "$TMP/me-anon" \
    || ! grep -qi '^www-authenticate: Bearer' "$TMP/me-anon.headers"; then
    fail "unauthenticated /api/v1/me ($out: $(cat "$TMP/me-anon"))"
  fi
  pass "request without auth gets 401"

  out="$(curl -sS -o "$TMP/session-anon" -w '%{http_code}' "$BASE_URL/api/v1/auth/session")"
  if [[ "$out" != 200 ]] || ! grep -q '"authenticated":false' "$TMP/session-anon"; then
    fail "anonymous session ($out: $(cat "$TMP/session-anon"))"
  fi
  pass "anonymous session reports signed out"

  dex_login "$TMP/jar" /after-login "$BASE_URL/after-login"
  grep -qE '^#HttpOnly_localhost[[:space:]].*[[:space:]]so_session[[:space:]]' "$TMP/jar" \
    || fail "session cookie is not HttpOnly or missing ($(cat "$TMP/jar"))"
  pass "OIDC code flow through Dex sets an HttpOnly session cookie"

  out="$(curl -sS -b "$TMP/jar" -o "$TMP/me" -D "$TMP/me.headers" -w '%{http_code}' "$BASE_URL/api/v1/me")"
  if [[ "$out" != 200 ]] || ! grep -q '"kind":"user"' "$TMP/me" || ! grep -q '"email":"admin@example.com"' "$TMP/me" \
    || ! grep -qi '^cache-control: no-store' "$TMP/me.headers"; then
    fail "/api/v1/me with a session cookie ($out: $(cat "$TMP/me"))"
  fi
  pass "session cookie authenticates /api/v1/me"

  out="$(curl -sS -b "$TMP/jar" -o "$TMP/instance" -D "$TMP/instance.headers" -w '%{http_code}' "$BASE_URL/api/v1/instance")"
  if [[ "$out" != 200 ]] || ! grep -q "\"public_url\":\"$BASE_URL\"" "$TMP/instance" || ! grep -q "\"mcp_url\":\"$BASE_URL/mcp\"" "$TMP/instance" \
    || ! grep -qi '^cache-control: no-store' "$TMP/instance.headers"; then
    fail "/api/v1/instance with a session cookie ($out: $(cat "$TMP/instance"))"
  fi
  for field in "\"oidc_issuer_url\":\"http://localhost:$DEX_PORT/dex\"" '"oidc_provider_name":"Dex"' '"allowed_email_domains":["example.com"]' \
    '"any_email_domain":false' '"session_ttl_seconds":604800'; do
    grep -qF -- "$field" "$TMP/instance" || fail "/api/v1/instance lacks $field ($(cat "$TMP/instance"))"
  done
  if grep -qiE 'client_secret|internal|password' "$TMP/instance"; then
    fail "/api/v1/instance exposes a secret-looking key ($(cat "$TMP/instance"))"
  fi
  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/instance")"
  [[ "$out" == 401 ]] || fail "/api/v1/instance without credentials ($out)"
  pass "instance URLs and sign-in settings are served to signed-in clients, with no secrets"

  out="$(curl -sS -b "$TMP/jar" -o /dev/null -w '%{http_code} %{redirect_url}' "$BASE_URL/api/v1/auth/callback?code=x&state=bogus")"
  [[ "$out" == "302 "*"/login?error=state_mismatch" ]] || fail "forged callback state ($out)"
  pass "callback with a forged state is rejected"

  dex_login "$TMP/jar2" //evil.example "$BASE_URL/"
  dex_login "$TMP/jar3" $'/\t/evil.example' "$BASE_URL/"
  pass "login redirects only to local paths"

  out="$(curl -sS -b "$TMP/jar" -o "$TMP/csrf-origin" -w '%{http_code}' -X POST -H 'Origin: http://evil.example' "$BASE_URL/api/v1/auth/logout")"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"csrf_rejected"' "$TMP/csrf-origin"; then
    fail "cross-origin logout by Origin ($out: $(cat "$TMP/csrf-origin"))"
  fi
  out="$(curl -sS -b "$TMP/jar" -o /dev/null -w '%{http_code}' -X POST -H 'Sec-Fetch-Site: cross-site' "$BASE_URL/api/v1/auth/logout")"
  [[ "$out" == 403 ]] || fail "cross-site logout by Sec-Fetch-Site ($out)"
  pass "cookie-authenticated cross-origin mutations are rejected"

  out="$(curl -sS -b "$TMP/jar" -o /dev/null -w '%{http_code}' -X POST -H "Origin: $BASE_URL" "$BASE_URL/api/v1/auth/logout")"
  [[ "$out" == 204 ]] || fail "logout ($out)"
  out="$(curl -sS -b "$TMP/jar" -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "/api/v1/me after logout ($out)"
  pass "logout ends the session"

  dex_login "$TMP/jar-outsider" / "$BASE_URL/login?error=email_not_allowed" outsider@example.org
  [[ "$(grep -c so_session "$TMP/jar-outsider" || true)" == 0 ]] || fail "refused account received a session cookie"
  out="$(curl -sS -b "$TMP/jar-outsider" -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "refused account /api/v1/me ($out)"
  curl -sS -o "$TMP/refused.html" "$BASE_URL/login?error=email_not_allowed"
  [[ "$(grep -c 'example.com' "$TMP/refused.html" || true)" == 0 ]] || fail "refusal page leaks the allowed domains"
  pass "accounts outside the allowed email domains are refused"

  local sa_id token key_id json_headers=(-H "Origin: $BASE_URL" -H 'Content-Type: application/json')
  dex_login "$TMP/sa-jar" / "$BASE_URL/"

  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/sa.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"e2e-bot"}' "$BASE_URL/api/v1/service-accounts")"
  if [[ "$out" != 201 ]] || ! grep -q '"name":"e2e-bot"' "$TMP/sa.json"; then
    fail "create service account ($out: $(cat "$TMP/sa.json"))"
  fi
  sa_id="$(grep -o '"id":"[^"]*"' "$TMP/sa.json" | head -1 | cut -d'"' -f4)"
  [[ -n "$sa_id" ]] || fail "service account response has no id ($(cat "$TMP/sa.json"))"
  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/sa-dup.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"e2e-bot"}' "$BASE_URL/api/v1/service-accounts")"
  if [[ "$out" != 409 ]] || ! grep -q '"code":"conflict"' "$TMP/sa-dup.json"; then
    fail "duplicate service account name ($out: $(cat "$TMP/sa-dup.json"))"
  fi
  pass "service accounts are created with unique names"

  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/key.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"ci"}' "$BASE_URL/api/v1/service-accounts/$sa_id/keys")"
  [[ "$out" == 201 ]] || fail "create key ($out: $(cat "$TMP/key.json"))"
  token="$(grep -o '"token":"sok_[^"]*"' "$TMP/key.json" | cut -d'"' -f4)"
  key_id="$(grep -o '"id":"[^"]*"' "$TMP/key.json" | head -1 | cut -d'"' -f4)"
  if [[ "${#token}" != 47 || "$token" != sok_* || -z "$key_id" ]]; then
    fail "issued key has the wrong shape (token length ${#token}, key id '$key_id')"
  fi
  pass "service-account keys are issued once"

  out="$(curl -sS -o "$TMP/me-bearer" -w '%{http_code}' -H "Authorization: Bearer $token" "$BASE_URL/api/v1/me")"
  if [[ "$out" != 200 ]] || ! grep -q '"kind":"service_account"' "$TMP/me-bearer" || ! grep -q '"name":"e2e-bot"' "$TMP/me-bearer"; then
    fail "/api/v1/me with a bearer key ($out: $(cat "$TMP/me-bearer"))"
  fi
  pass "a service-account key works as a bearer token"

  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/keys.json" -w '%{http_code}' "$BASE_URL/api/v1/service-accounts/$sa_id/keys")"
  if [[ "$out" != 200 ]] || ! grep -q "\"display_prefix\":\"${token:0:12}\"" "$TMP/keys.json"; then
    fail "key listing ($out: $(cat "$TMP/keys.json"))"
  fi
  grep -q '"last_used_at":"' "$TMP/keys.json" || fail "key listing has no last_used_at after a bearer call ($(cat "$TMP/keys.json"))"
  [[ "$(grep -c "$token" "$TMP/keys.json" || true)" == 0 ]] || fail "key listing contains the full token"
  [[ "$(grep -c '"token' "$TMP/keys.json" || true)" == 0 ]] || fail "key listing contains a token field"
  pass "key listings never expose secrets"

  out="$(curl -sS -o "$TMP/escalate.json" -w '%{http_code}' -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{"name":"escalate"}' "$BASE_URL/api/v1/service-accounts")"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"forbidden"' "$TMP/escalate.json"; then
    fail "service account creating a service account ($out: $(cat "$TMP/escalate.json"))"
  fi
  pass "service accounts cannot mint keys"

  out="$(curl -sS -b "$TMP/sa-jar" -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer sok_invalid' "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "invalid bearer with a valid session ($out)"
  out="$(curl -sS -b "$TMP/sa-jar" -o /dev/null -w '%{http_code}' -H "Authorization: Bearer sok_$(printf 'A%.0s' {1..43})" "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "well-shaped unknown bearer key ($out)"
  pass "an invalid bearer key is rejected even with a session"

  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/ctl-key.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"ci\u0001"}' "$BASE_URL/api/v1/service-accounts/$sa_id/keys")"
  if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/ctl-key.json"; then
    fail "key name with a control character ($out: $(cat "$TMP/ctl-key.json"))"
  fi
  out="$(curl -sS -b "$TMP/sa-jar" -o "$TMP/ctl-sa.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"ctl-bot","description":"a\u0000b"}' "$BASE_URL/api/v1/service-accounts")"
  if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/ctl-sa.json"; then
    fail "description with a control character ($out: $(cat "$TMP/ctl-sa.json"))"
  fi
  pass "control characters in names and descriptions are rejected"

  for _ in 1 2; do
    out="$(curl -sS -b "$TMP/sa-jar" -o /dev/null -w '%{http_code}' -X DELETE -H "Origin: $BASE_URL" "$BASE_URL/api/v1/service-accounts/$sa_id/keys/$key_id")"
    [[ "$out" == 204 ]] || fail "revoke key ($out)"
  done
  out="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "revoked key /api/v1/me ($out)"
  pass "revoked keys stop working"

  out="$(curl -sS -b "$TMP/sa-jar" -o /dev/null -w '%{http_code}' -X DELETE -H "Origin: $BASE_URL" "$BASE_URL/api/v1/service-accounts/$sa_id/keys/$LOOKALIKE_UUID")"
  [[ "$out" == 400 ]] || fail "revoke a key with misplaced dashes in its id ($out)"
  pass "ids with misplaced dashes are not read as UUIDs"

  check_datasources "$TMP/sa-jar"
  dashboards_suite
  events_suite
  comments_suite
  revisions_suite
  observe_suite
  mcp_suite
  proxy_suite
  cli_suite
  dashboard_list_suite
  evals_smoke_suite
  people_suite
  check_api_contract
  check_no_keys_in_server_log
}

check_api_contract() {
  local logs violations
  logs_into logs server
  grep -q 'api contract check enabled' <<<"$logs" || fail "server did not enable the API contract check"
  pass "server runs with the API contract check enabled"
  violations="$(grep 'api contract violation' <<<"$logs" || true)"
  [[ -z "$violations" ]] || fail "API calls broke the OpenAPI contract:"$'\n'"$violations"
  pass "every API call matched the OpenAPI contract"
}

check_no_keys_in_server_log() {
  local logs
  logs_into logs server
  if grep -qE 'sok_[A-Za-z0-9_-]{43}' <<<"$logs"; then
    fail "the server log contains a service-account key"
  fi
  pass "the server log contains no service-account keys"
}

all_datasources_listed() {
  local uid
  for uid in prom ch prom_tight ch_tight; do
    grep -q "\"uid\":\"$uid\"" <<<"$1" || return 1
  done
}

datasources_ready() {
  capture LAST foreground command curl -sS --max-time 10 -b "$1" "$BASE_URL/api/v1/datasources" || true
  [[ "$(grep -o '"healthy":true' <<<"$LAST" | wc -l | tr -d ' ')" == 4 ]] && all_datasources_listed "$LAST"
}

check_datasources() {
  local jar="$1" out body leak entry keys entries=0

  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/datasources")"
  [[ "$out" == 401 ]] || fail "unauthenticated datasource list ($out)"
  pass "datasource list requires authentication"

  poll_try 60 1 datasources_ready "$jar" || fail "datasources did not report healthy within 60s ($LAST)"
  body="$LAST"
  leak="$(grep -oE 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' <<<"$body" || true)"
  [[ -z "$leak" ]] || fail "datasource list exposes connection details ($leak)"
  pass "datasources report healthy without exposing connection details"

  while IFS= read -r entry; do
    keys="$(grep -oE '"[A-Za-z0-9_]+":' <<<"$entry" | tr -d '":' | sort | paste -sd, -)"
    case "$keys" in
      checked_at,dashboard_count,healthy,name,type,uid | checked_at,dashboard_count,healthy,message,name,type,uid) entries=$((entries + 1)) ;;
      *) fail "datasource entry has unexpected keys ($keys)" ;;
    esac
  done < <(sed -E 's/:"([^"\\]|\\.)*"/:/g' <<<"$body" | grep -oE '\{[^{}]*\}')
  [[ "$entries" == 4 ]] || fail "expected 4 datasource entries, found $entries"
  pass "datasource entries carry exactly uid, type, name, healthy, checked_at, dashboard_count and an optional message"
}

API_TOKEN=""
FIXTURES="$ROOT/e2e/fixtures/api"
LOOKALIKE_UUID=0000000000000x0000x0000x000000000000

api_token() {
  [[ -z "$API_TOKEN" ]] || return 0
  local jar="$TMP/api-jar" out sa_id json_headers=(-H "Origin: $BASE_URL" -H 'Content-Type: application/json')
  dex_login "$jar" / "$BASE_URL/"
  out="$(curl -sS -b "$jar" -o "$TMP/api-sa.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"e2e-api"}' "$BASE_URL/api/v1/service-accounts")"
  [[ "$out" == 201 ]] || fail "create the e2e-api service account ($out: $(cat "$TMP/api-sa.json"))"
  sa_id="$(grep -o '"id":"[^"]*"' "$TMP/api-sa.json" | head -1 | cut -d'"' -f4)"
  out="$(curl -sS -b "$jar" -o "$TMP/api-key.json" -w '%{http_code}' -X POST "${json_headers[@]}" -d '{"name":"suite"}' "$BASE_URL/api/v1/service-accounts/$sa_id/keys")"
  [[ "$out" == 201 ]] || fail "create the e2e-api key ($out: $(cat "$TMP/api-key.json"))"
  API_TOKEN="$(grep -o '"token":"sok_[^"]*"' "$TMP/api-key.json" | cut -d'"' -f4)"
  [[ "$API_TOKEN" == sok_* ]] || fail "the e2e-api key response has no token"
}

api() {
  local method="$1" path="$2"
  shift 2
  command curl -sS --max-time 90 -o "$TMP/api.body" -D "$TMP/api.headers" -w '%{http_code}' -X "$method" \
    -H "Authorization: Bearer $API_TOKEN" -H 'Content-Type: application/json' "$@" "$BASE_URL/api/v1$path"
}

version_body() {
  printf '{"parent_version":%s,%s' "$2" "$(tail -c +2 "$FIXTURES/$1")" >"$TMP/version-body.json"
}

expect_rejected() {
  local fixture="$1" code="$2" path="${3:-}" out
  out="$(api POST /dashboards --data-binary "@$FIXTURES/$fixture")"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"invalid_spec"' "$TMP/api.body" || ! grep -q "\"code\":\"$code\"" "$TMP/api.body"; then
    fail "$fixture should be rejected with $code ($out: $(cat "$TMP/api.body"))"
  fi
  if [[ -n "$path" ]] && ! grep -q "\"path\":\"$path\"" "$TMP/api.body"; then
    fail "$fixture rejection should point at $path ($(cat "$TMP/api.body"))"
  fi
}

dashboards_suite() {
  api_token
  local out id evolved_id nil_id=00000000-0000-0000-0000-000000000000 leak ids cursor first_id found panel_id path origin_id

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  if [[ "$out" != 201 ]] || ! grep -q '"version":1' "$TMP/api.body" || ! grep -q '"parent_version":null' "$TMP/api.body"; then
    fail "create a valid dashboard ($out: $(cat "$TMP/api.body"))"
  fi
  id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  [[ -n "$id" ]] || fail "created dashboard has no id ($(cat "$TMP/api.body"))"
  grep -qi "^location: /api/v1/dashboards/$id" "$TMP/api.headers" || fail "create response has no Location header ($(cat "$TMP/api.headers"))"
  leak="$(grep -oE 'prometheus:9090|clickhouse:9000|slashobserve_ro' "$TMP/api.body" || true)"
  [[ -z "$leak" ]] || fail "create response exposes connection details ($leak)"
  pass "a valid dashboard is saved as version 1 without exposing connection details"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-evolved.json")"
  if [[ "$out" != 201 ]]; then
    fail "create the evolved shape dashboard ($out: $(cat "$TMP/api.body"))"
  fi
  evolved_id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  [[ -n "$evolved_id" ]] || fail "evolved dashboard has no id ($(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$evolved_id/versions/1/panels/p_up/data")"
  if [[ "$out" != 200 ]] || ! grep -q '"frames":\[{' "$TMP/api.body"; then
    fail "a row child returns frames ($out: $(cat "$TMP/api.body"))"
  fi
  for panel_id in p_note r_prom; do
    out="$(api GET "/dashboards/$evolved_id/versions/1/panels/$panel_id/data")"
    if [[ "$out" != 404 ]] || ! grep -q '"code":"panel_not_found"' "$TMP/api.body"; then
      fail "panel $panel_id should have no data ($out: $(cat "$TMP/api.body"))"
    fi
  done
  pass "an evolved Spec with a row, a text panel and a placeholder saves, and only data panels return data"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-blank-summary.json")"
  if [[ "$out" != 201 ]] || ! grep -q '"summary":"Create dashboard"' "$TMP/api.body"; then
    fail "create with a blank summary ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a blank summary falls back to the default"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-nul.json")"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"invalid_spec"' "$TMP/api.body"; then
    fail "a NUL character should be rejected as an invalid Spec ($out: $(cat "$TMP/api.body"))"
  fi
  for path in /summary /title /panels/0/title; do
    grep -q "\"path\":\"$path\",\"code\":\"schema\"" "$TMP/api.body" || fail "NUL rejection should point at $path in one response ($(cat "$TMP/api.body"))"
  done
  pass "NUL characters in the summary and the Spec are rejected together with their paths"

  expect_rejected dashboard-invalid-schema.json schema
  pass "a Spec that breaks the schema is rejected with its issues"

  expect_rejected dashboard-failing-query.json query_invalid /panels/0/queries/0/query
  pass "a panel whose query fails is rejected with the reason"

  expect_rejected dashboard-empty-query.json query_empty
  pass "a panel whose query returns no data is rejected"

  expect_rejected dashboard-ch-insert.json query_forbidden
  expect_rejected dashboard-ch-multi.json query_forbidden
  pass "ClickHouse writes and multi-statement queries are rejected"

  expect_rejected dashboard-unknown-ds.json unknown_datasource
  pass "a panel on an unknown datasource is rejected"

  expect_rejected dashboard-legacy.json schema
  pass "a Spec in the legacy v0 shape is rejected"

  version_body dashboard-v2.json 1
  out="$(api POST "/dashboards/$id/versions" --data-binary "@$TMP/version-body.json")"
  if [[ "$out" != 201 ]] || ! grep -q '"version":2' "$TMP/api.body" || ! grep -q '"parent_version":1' "$TMP/api.body"; then
    fail "save version 2 ($out: $(cat "$TMP/api.body"))"
  fi
  grep -qi "^location: /api/v1/dashboards/$id/versions/2" "$TMP/api.headers" || fail "new version has no Location header ($(cat "$TMP/api.headers"))"
  out="$(api POST "/dashboards/$id/versions" --data-binary "@$TMP/version-body.json")"
  if [[ "$out" != 200 ]] || ! grep -qi '^idempotent-replay: true' "$TMP/api.headers" || ! grep -q '"version":2' "$TMP/api.body"; then
    fail "replay of version 2 ($out: $(cat "$TMP/api.headers") $(cat "$TMP/api.body"))"
  fi
  version_body dashboard-valid.json 1
  out="$(api POST "/dashboards/$id/versions" --data-binary "@$TMP/version-body.json")"
  if [[ "$out" != 409 ]] || ! grep -q '"code":"version_conflict"' "$TMP/api.body" || ! grep -q '"latest_version":2' "$TMP/api.body"; then
    fail "stale parent version ($out: $(cat "$TMP/api.body"))"
  fi
  pass "versions stack on their parent, a repeated save replays, and a stale parent gets 409"

  out="$(api GET /dashboards)"
  if [[ "$out" != 200 ]] || ! grep -q "\"id\":\"$id\"" "$TMP/api.body"; then
    fail "list dashboards ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards?cursor=not-a-cursor")"
  [[ "$out" == 400 ]] || fail "list dashboards with a malformed cursor ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id")"
  if [[ "$out" != 200 ]] || ! grep -q '"latest_version":2' "$TMP/api.body"; then
    fail "get dashboard ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/versions")"
  ids="$(grep -o '"version":[0-9]*' "$TMP/api.body" | paste -sd, -)"
  if [[ "$out" != 200 || "$ids" != '"version":2,"version":1' ]] || grep -q '"spec"' "$TMP/api.body"; then
    fail "list versions newest first without Specs ($out: $ids)"
  fi
  out="$(api GET "/dashboards/$id/versions/1")"
  if [[ "$out" != 200 ]] || ! grep -q '"spec":{"specVersion":"0","title":"E2E dashboard",' "$TMP/api.body"; then
    fail "get version 1 ($out: $(cat "$TMP/api.body"))"
  fi
  pass "dashboards and their versions can be listed and read"

  out="$(api GET "/dashboards/$id/diff?from=1&to=2")"
  if [[ "$out" != 200 ]] || ! grep -q '"path":"/title"' "$TMP/api.body"; then
    fail "diff versions 1 and 2 ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/diff?from=0&to=1")"
  if [[ "$out" != 200 ]] || ! grep -q '"from":0,"to":1,"changes":\[{"op":"add","path":"","to":{"' "$TMP/api.body" || grep -q '"from":null' "$TMP/api.body"; then
    fail "diff version 1 against an empty document ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/diff?from=2&to=1")"
  [[ "$out" == 400 ]] || fail "diff with from after to ($out: $(cat "$TMP/api.body"))"
  pass "versions can be diffed"

  out="$(api POST "/dashboards/$id/versions/1/restore" -d '{"parent_version":2}')"
  if [[ "$out" != 201 ]] || ! grep -q '"version":3' "$TMP/api.body" || ! grep -q '"restored_from":1' "$TMP/api.body"; then
    fail "restore version 1 ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$id/versions/1/restore" -d '{"parent_version":2}')"
  [[ "$out" == 409 ]] || fail "restore on a stale parent ($out: $(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$id/versions/1/restore" -d '{"parent_version":3,"summary":""}')"
  if [[ "$out" != 201 ]] || ! grep -q '"version":4' "$TMP/api.body" || ! grep -q '"summary":"Restore version 1"' "$TMP/api.body"; then
    fail "restore with a blank summary ($out: $(cat "$TMP/api.body"))"
  fi
  pass "an earlier version is restored as a new version"

  out="$(api GET "/dashboards/$nil_id")"
  [[ "$out" == 404 ]] || fail "unknown dashboard ($out)"
  out="$(api GET /dashboards/x)"
  [[ "$out" == 400 ]] || fail "dashboard id that is not a UUID ($out)"
  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/dashboards")"
  [[ "$out" == 401 ]] || fail "dashboard list without credentials ($out)"
  out="$(api GET "/dashboards/$LOOKALIKE_UUID")"
  [[ "$out" == 400 ]] || fail "dashboard id with misplaced dashes ($out: $(cat "$TMP/api.body"))"
  cursor="$(printf '0:%s' "$LOOKALIKE_UUID" | base64 | tr '+/' '-_' | tr -d '=\n')"
  out="$(api GET "/dashboards?cursor=$cursor")"
  [[ "$out" == 400 ]] || fail "list cursor with misplaced dashes in its id ($out: $(cat "$TMP/api.body"))"
  pass "unknown, malformed and unauthenticated dashboard requests are refused"

  head -c "$((3 * 512 * 1024))" /dev/zero | tr '\0' a | { printf '{"summary":"big","spec":{"pad":"'; cat; printf '"}}'; } >"$TMP/oversized.json"
  out="$(api POST /dashboards --data-binary "@$TMP/oversized.json")"
  if [[ "$out" != 413 ]] || ! grep -q '"code":"too_large"' "$TMP/api.body"; then
    fail "oversized dashboard body ($out: $(cat "$TMP/api.body"))"
  fi
  pass "an oversized dashboard body is refused with 413"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-all-variables.json")"
  [[ "$out" == 201 ]] || fail "dashboard with includeAll variables ($out: $(cat "$TMP/api.body"))"
  pass "variables that include all values are saved"

  out="$(api GET "/dashboards?limit=1")"
  cursor="$(grep -o '"next_cursor":"[^"]*"' "$TMP/api.body" | cut -d'"' -f4 || true)"
  first_id="$(grep -o '"dashboards":\[{"id":"[^"]*"' "$TMP/api.body" | cut -d'"' -f6 || true)"
  if [[ "$out" != 200 || -z "$cursor" || -z "$first_id" ]] || ! grep -q '"title":"All variables"' "$TMP/api.body"; then
    fail "the first page lists the most recently updated dashboard and a cursor ($out: $(cat "$TMP/api.body"))"
  fi
  found=0
  for _ in $(seq 1 50); do
    out="$(api GET "/dashboards?limit=1&cursor=$cursor")"
    [[ "$out" == 200 ]] || fail "following the page cursor ($out: $(cat "$TMP/api.body"))"
    ! grep -q "\"id\":\"$first_id\"" "$TMP/api.body" || fail "a later page repeats the first page's dashboard"
    if grep -q "\"dashboards\":\[{\"id\":\"$id\"" "$TMP/api.body"; then
      found=1
      break
    fi
    cursor="$(grep -o '"next_cursor":"[^"]*"' "$TMP/api.body" | cut -d'"' -f4 || true)"
    [[ -n "$cursor" ]] || break
  done
  [[ "$found" == 1 ]] || fail "paging with the cursor never reached the older dashboard $id"
  pass "dashboards are paged newest first with an opaque cursor"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-broken-variables.json")"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"invalid_spec"' "$TMP/api.body" \
    || ! grep -q '"path":"/variables/0","code":"variable_empty"' "$TMP/api.body" \
    || ! grep -q '"path":"/variables/1","code":"variable_failed"' "$TMP/api.body" \
    || ! grep -q '"path":"/panels/0/queries/0/query","code":"variable_unresolved"' "$TMP/api.body" \
    || grep -q '"path":"/panels/1' "$TMP/api.body"; then
    fail "broken variables ($out: $(cat "$TMP/api.body"))"
  fi
  pass "broken variables are reported with the panels that depend on them"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create a dashboard for the api origin ($out: $(cat "$TMP/api.body"))"
  origin_id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  out="$(api GET "/dashboards/$origin_id/versions/1")"
  if [[ "$out" != 200 ]] || ! grep -q '"origin":"api"' "$TMP/api.body" || ! grep -q '"origin_client":null' "$TMP/api.body" \
    || ! grep -q '"agent_request_id":null' "$TMP/api.body"; then
    fail "a version saved with a bearer key should record the api origin ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(command curl -sS -o "$TMP/web-create.json" -w '%{http_code}' -b "$TMP/api-jar" -H "Origin: $BASE_URL" -H 'Content-Type: application/json' -X POST --data-binary "@$FIXTURES/dashboard-valid.json" "$BASE_URL/api/v1/dashboards")"
  [[ "$out" == 201 ]] || fail "create a dashboard through the browser session ($out: $(cat "$TMP/web-create.json"))"
  origin_id="$(grep -o '"id":"[^"]*"' "$TMP/web-create.json" | head -1 | cut -d'"' -f4)"
  out="$(api GET "/dashboards/$origin_id/versions/1")"
  if [[ "$out" != 200 ]] || ! grep -q '"origin":"web"' "$TMP/api.body"; then
    fail "a version saved through the browser session should record the web origin ($out: $(cat "$TMP/api.body"))"
  fi
  pass "versions record the web and api origins"
}

sse_open() {
  local id="$1" file="$2" tries=50
  sse_close
  : >"$file"
  command curl -sS -N --max-time 60 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/dashboards/$id/events" >"$file" 9>&- &
  SSE_PID=$!
  disown "$SSE_PID"
  until grep -q '^event: ready$' "$file"; do
    ((--tries > 0)) || fail "SSE stream for $id sent no ready event ($(cat "$file"))"
    sleep 0.2
  done
}

sse_wait() {
  local file="$1" pattern="$2" tries=$(($3 * 5))
  until grep -q -- "$pattern" "$file"; do
    ((--tries > 0)) || fail "SSE stream never matched '$pattern' ($(cat "$file"))"
    sleep 0.2
  done
}

sse_close() {
  if [[ -n "$SSE_PID" ]]; then kill "$SSE_PID" 2>/dev/null || true; fi
  SSE_PID=""
}

events_suite() {
  api_token
  local out id nil_id=00000000-0000-0000-0000-000000000000 stream="$TMP/sse-events.txt"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the SSE dashboard ($out: $(cat "$TMP/api.body"))"
  id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"

  sse_open "$id" "$stream"
  sse_wait "$stream" "^data: {\"dashboard_id\":\"$id\",\"latest_version\":1}$" 5
  pass "SSE stream starts with a ready snapshot"

  version_body dashboard-v2.json 1
  out="$(api POST "/dashboards/$id/versions" --data-binary "@$TMP/version-body.json")"
  [[ "$out" == 201 ]] || fail "add a version to the SSE dashboard ($out: $(cat "$TMP/api.body"))"
  sse_wait "$stream" '^event: version.created$' 10
  sse_wait "$stream" "^data: {\"type\":\"version.created\",\"dashboard_id\":\"$id\",\"version\":2,\"parent_version\":1," 10
  pass "SSE emits version.created for a new version"
  sse_close

  command curl -sS -o /dev/null -D "$TMP/sse.headers" --max-time 2 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/dashboards/$id/events" 2>/dev/null || true
  grep -qi '^content-type: text/event-stream' "$TMP/sse.headers" || fail "SSE response is not text/event-stream ($(cat "$TMP/sse.headers"))"
  grep -qi '^cache-control: no-store' "$TMP/sse.headers" || fail "SSE response is cacheable ($(cat "$TMP/sse.headers"))"
  grep -qi '^x-accel-buffering: no' "$TMP/sse.headers" || fail "SSE response may be buffered by a proxy ($(cat "$TMP/sse.headers"))"
  pass "SSE responses are uncacheable, unbuffered text/event-stream"

  out="$(api GET "/dashboards/$nil_id/events")"
  if [[ "$out" != 404 ]] || ! grep -q '"code":"not_found"' "$TMP/api.body"; then
    fail "SSE for an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/not-a-uuid/events")"
  [[ "$out" == 400 ]] || fail "SSE for a malformed dashboard id ($out: $(cat "$TMP/api.body"))"
  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/dashboards/$id/events")"
  [[ "$out" == 401 ]] || fail "SSE without a token ($out)"
  pass "SSE refuses unknown dashboards, malformed ids and unauthenticated callers"
}

comment_body() {
  printf '{"version":%s,"panel_id":"%s","body":"%s","request_revision":false}' "$1" "$2" "$3"
}

comment_ids() {
  { grep -o '{"id":"[^"]*","dashboard_id"' "$1" || true; } | cut -d'"' -f4
}

comments_suite() {
  api_token
  local out id panel second_panel cid cid2 cid3 cursor stream="$TMP/sse-comments.txt" updated page_ids
  local nil_id=00000000-0000-0000-0000-000000000000 other_id cid_other

  panel="$(sed -n 's/^ *"id": *"\([^"]*\)".*/\1/p' "$FIXTURES/dashboard-valid.json" | sed -n 1p)"
  second_panel="$(sed -n 's/^ *"id": *"\([^"]*\)".*/\1/p' "$FIXTURES/dashboard-valid.json" | sed -n 2p)"
  [[ -n "$panel" && -n "$second_panel" ]] || fail "dashboard-valid.json has no two panel ids ($panel, $second_panel)"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the comments dashboard ($out: $(cat "$TMP/api.body"))"
  id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"

  sse_open "$id" "$stream"
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 "$panel" 'The rate looks high after the deploy.')")"
  if [[ "$out" != 201 ]] || ! grep -q '"status":"open"' "$TMP/api.body" || ! grep -q '"revision":null' "$TMP/api.body" \
    || ! grep -q '"panel_present":true' "$TMP/api.body" || ! grep -q '"resolved_at":null' "$TMP/api.body" \
    || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body"; then
    fail "comment on panel $panel ($out: $(cat "$TMP/api.body"))"
  fi
  cid="$(comment_ids "$TMP/api.body")"
  cid_other="$cid"
  [[ -n "$cid" ]] || fail "created comment has no id ($(cat "$TMP/api.body"))"
  grep -qi "^location: /api/v1/dashboards/$id/comments/$cid" "$TMP/api.headers" || fail "comment response has no Location header ($(cat "$TMP/api.headers"))"
  sse_wait "$stream" '^event: comment.created$' 10
  sse_wait "$stream" "^data: {\"type\":\"comment.created\",\"dashboard_id\":\"$id\",.*\"comment_id\":\"$cid\",\"panel_id\":\"$panel\",\"status\":\"open\"}$" 10
  pass "a comment on a panel is saved open with no revision and announced on the event stream"

  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 no_such_panel 'x')")"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"unknown_panel"' "$TMP/api.body"; then
    fail "comment on an unknown panel ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 99 "$panel" 'x')")"
  if [[ "$out" != 404 ]] || ! grep -q '"code":"not_found"' "$TMP/api.body"; then
    fail "comment on an unknown version ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$nil_id/comments" -d "$(comment_body 1 "$panel" 'x')")"
  [[ "$out" == 404 ]] || fail "comment on an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api POST /dashboards/not-a-uuid/comments -d "$(comment_body 1 "$panel" 'x')")"
  [[ "$out" == 400 ]] || fail "comment on a malformed dashboard id ($out: $(cat "$TMP/api.body"))"
  for bad in "$(comment_body 1 "$panel" '')" "$(comment_body 1 "$panel" '   ')" "$(comment_body 1 "$panel" 'a\u0000b')" \
    "$(comment_body 1 "$panel" 'a\u202eb')" \
    "$(comment_body 1 "$panel" "$(printf 'a%.0s' $(seq 1 4001))")" "$(comment_body 1 'p\u0000' 'x')" "$(comment_body 1 '' 'x')" \
    "$(comment_body 1 "$(printf 'p%.0s' $(seq 1 65))" 'x')" "$(comment_body 0 "$panel" 'x')" '{"version":1,"panel_id":"p"}' \
    "$(comment_body 1 "$panel" 'x' | sed 's/}$/,"extra":1}/')" '{"version":"1","panel_id":"p","body":"x"}'; do
    out="$(api POST "/dashboards/$id/comments" -d "$bad")"
    if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
      fail "invalid comment ${bad:0:80} should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
    fi
  done
  pass "comments on unknown panels, versions and dashboards, and invalid comment bodies, are rejected"

  out="$(api PATCH "/dashboards/$id/comments/$cid" -d '{"status":"resolved"}')"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"resolved"' "$TMP/api.body" || ! grep -q '"resolved_at":"20' "$TMP/api.body" \
    || ! grep -q '"resolved_by":{"kind":"service_account"' "$TMP/api.body"; then
    fail "resolve the comment ($out: $(cat "$TMP/api.body"))"
  fi
  sse_wait "$stream" '^event: comment.updated$' 10
  sse_wait "$stream" "^data: {\"type\":\"comment.updated\",\"dashboard_id\":\"$id\",.*\"comment_id\":\"$cid\",\"panel_id\":\"$panel\",\"status\":\"resolved\"}$" 10
  out="$(api PATCH "/dashboards/$id/comments/$cid" -d '{"status":"resolved"}')"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"resolved"' "$TMP/api.body"; then
    fail "resolve the comment again ($out: $(cat "$TMP/api.body"))"
  fi
  sleep 2
  updated="$(grep -c '^event: comment.updated$' "$stream" || true)"
  [[ "$updated" == 1 ]] || fail "resolving twice sent $updated comment.updated events"
  pass "resolving a comment is idempotent and sends one comment.updated event"

  out="$(api PATCH "/dashboards/$id/comments/$cid" -d '{"status":"open"}')"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"open"' "$TMP/api.body" || ! grep -q '"resolved_at":null' "$TMP/api.body" \
    || ! grep -q '"resolved_by":null' "$TMP/api.body"; then
    fail "reopen the comment ($out: $(cat "$TMP/api.body"))"
  fi
  sse_wait "$stream" "^data: {\"type\":\"comment.updated\",\"dashboard_id\":\"$id\",.*\"comment_id\":\"$cid\",\"panel_id\":\"$panel\",\"status\":\"open\"}$" 10
  pass "reopening a comment clears its resolution"

  out="$(api PATCH "/dashboards/$id/comments/$cid" -d '{"status":"archived"}')"
  [[ "$out" == 400 ]] || fail "an unknown comment status ($out: $(cat "$TMP/api.body"))"
  out="$(api PATCH "/dashboards/$id/comments/$cid" -d '{}')"
  [[ "$out" == 400 ]] || fail "a status change with no status ($out: $(cat "$TMP/api.body"))"
  out="$(api PATCH "/dashboards/$id/comments/$nil_id" -d '{"status":"resolved"}')"
  if [[ "$out" != 404 ]] || ! grep -q '"code":"not_found"' "$TMP/api.body"; then
    fail "resolve an unknown comment ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api PATCH "/dashboards/$nil_id/comments/$cid" -d '{"status":"resolved"}')"
  [[ "$out" == 404 ]] || fail "resolve a comment under an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the second comments dashboard ($out: $(cat "$TMP/api.body"))"
  other_id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  out="$(api PATCH "/dashboards/$other_id/comments/$cid" -d '{"status":"resolved"}')"
  [[ "$out" == 404 ]] || fail "resolve a comment through another dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id/comments")"
  [[ "$out" == 200 ]] && grep -q '"status":"open"' "$TMP/api.body" || fail "the comment changed through another dashboard ($out: $(cat "$TMP/api.body"))"
  pass "status changes refuse unknown statuses, unknown comments and comments of another dashboard"
  sse_close

  out="$(api GET "/dashboards/$id/comments?status=open")"
  if [[ "$out" != 200 ]] || ! grep -q "\"id\":\"$cid\"" "$TMP/api.body"; then
    fail "list open comments ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/comments?status=resolved")"
  if [[ "$out" != 200 ]] || grep -q "\"id\":\"$cid\"" "$TMP/api.body" || ! grep -q '"comments":\[\]' "$TMP/api.body"; then
    fail "list resolved comments ($out: $(cat "$TMP/api.body"))"
  fi
  pass "comments are filtered by status"

  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 "$second_panel" 'The table is empty.')")"
  [[ "$out" == 201 ]] || fail "second comment ($out: $(cat "$TMP/api.body"))"
  cid2="$(comment_ids "$TMP/api.body")"
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 "$panel" 'Any update?')")"
  [[ "$out" == 201 ]] || fail "third comment ($out: $(cat "$TMP/api.body"))"
  cid3="$(comment_ids "$TMP/api.body")"
  out="$(api GET "/dashboards/$id/comments?limit=2")"
  cursor="$(grep -o '"next_cursor":"[^"]*"' "$TMP/api.body" | cut -d'"' -f4)"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  if [[ "$out" != 200 || -z "$cursor" || "$page_ids" != "$cid $cid2 " ]]; then
    fail "first page of comments ($out, cursor '$cursor', ids '$page_ids': $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/comments?limit=2&cursor=$cursor")"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  if [[ "$out" != 200 || "$page_ids" != "$cid3 " ]] || ! grep -q '"next_cursor":null' "$TMP/api.body"; then
    fail "second page of comments (ids '$page_ids': $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/comments?panel_id=$second_panel")"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  [[ "$out" == 200 && "$page_ids" == "$cid2 " ]] || fail "comments filtered by panel (ids '$page_ids': $(cat "$TMP/api.body"))"
  for bad in 'status=archived' 'limit=0' 'limit=501' 'limit=x' 'cursor=!!!' cursor=MTox 'panel_id=%00' 'panel_id=' 'status=' 'panel_id='"$(printf 'p%.0s' $(seq 1 65))"; do
    out="$(api GET "/dashboards/$id/comments?$bad")"
    [[ "$out" == 400 ]] || fail "list comments with $bad should answer 400 ($out: $(cat "$TMP/api.body"))"
  done
  out="$(api GET "/dashboards/$nil_id/comments")"
  [[ "$out" == 404 ]] || fail "list comments of an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  pass "comments list oldest first with cursors, panel filters and validated parameters"

  printf '{"parent_version":1,"summary":"Drop the first panel","spec":{"specVersion":"0","title":"E2E dashboard","time":{"from":"now-15m","to":"now"},"panels":[{"id":"%s","type":"table","title":"Log rows by service","gridPos":{"x":0,"y":0,"w":24,"h":8},"datasource":"ch","queries":[{"query":"SELECT ServiceName AS service, count() AS n FROM otel_logs WHERE $__timeFilter(Timestamp) GROUP BY service ORDER BY n DESC LIMIT 10"}]}]}}' "$second_panel" >"$TMP/comments-v2.json"
  out="$(api POST "/dashboards/$id/versions" --data-binary "@$TMP/comments-v2.json")"
  [[ "$out" == 201 ]] || fail "add a version without the first panel ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id/comments?panel_id=$panel")"
  if [[ "$out" != 200 ]] || ! grep -q "\"id\":\"$cid\"" "$TMP/api.body" || grep -q '"panel_present":true' "$TMP/api.body" || ! grep -q '"panel_present":false' "$TMP/api.body"; then
    fail "comments on a removed panel ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/comments?panel_id=$second_panel")"
  if [[ "$out" != 200 ]] || ! grep -q '"panel_present":true' "$TMP/api.body" || grep -q '"panel_present":false' "$TMP/api.body"; then
    fail "comments on a kept panel ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 "$panel" 'Still anchored to version 1.')")"
  if [[ "$out" != 201 ]] || ! grep -q '"panel_present":false' "$TMP/api.body"; then
    fail "comment on a panel of an older version ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 2 "$panel" 'x')")"
  [[ "$out" == 422 ]] || fail "comment on a panel the newest version dropped ($out: $(cat "$TMP/api.body"))"
  pass "comments stay on the version they were made on and report whether their panel is still present"

  local tid wid rid1 rid2 stream_threads="$TMP/sse-threads.txt"
  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the threads dashboard ($out: $(cat "$TMP/api.body"))"
  tid="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  sse_open "$tid" "$stream_threads"

  out="$(api POST "/dashboards/$tid/comments" -d '{"version":1,"body":"Whole dashboard note"}')"
  if [[ "$out" != 201 ]] || ! grep -q '"panel_id":null' "$TMP/api.body" || ! grep -q '"parent_id":null' "$TMP/api.body" \
    || ! grep -q '"panel_present":true' "$TMP/api.body" || ! grep -q '"revision":null' "$TMP/api.body"; then
    fail "whole-dashboard comment ($out: $(cat "$TMP/api.body"))"
  fi
  wid="$(comment_ids "$TMP/api.body")"
  [[ -n "$wid" ]] || fail "whole-dashboard comment has no id ($(cat "$TMP/api.body"))"
  sse_wait "$stream_threads" "^data: {\"type\":\"comment.created\",\"dashboard_id\":\"$tid\",.*\"comment_id\":\"$wid\"" 10
  if grep "\"comment_id\":\"$wid\"" "$stream_threads" | grep -q '"panel_id"'; then
    fail "the whole-dashboard comment event carries a panel_id ($(cat "$stream_threads"))"
  fi
  pass "a whole-dashboard comment needs no panel and queues no revision by default"

  out="$(api POST "/dashboards/$tid/comments" -d '{"version":1,"panel_id":"'"$panel"'","body":"Thread root"}')"
  [[ "$out" == 201 ]] || fail "thread root ($out: $(cat "$TMP/api.body"))"
  cid="$(comment_ids "$TMP/api.body")"
  out="$(api POST "/dashboards/$tid/comments" -d '{"parent_id":"'"$cid"'","body":"Agreed"}')"
  if [[ "$out" != 201 ]] || ! grep -q "\"parent_id\":\"$cid\"" "$TMP/api.body" || ! grep -q '"version":1,' "$TMP/api.body" \
    || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body" || ! grep -q '"status":"open"' "$TMP/api.body"; then
    fail "reply to the thread root ($out: $(cat "$TMP/api.body"))"
  fi
  rid1="$(comment_ids "$TMP/api.body")"
  sse_wait "$stream_threads" "^data: {\"type\":\"comment.created\",\"dashboard_id\":\"$tid\",.*\"comment_id\":\"$rid1\",.*\"parent_id\":\"$cid\"" 10
  out="$(api POST "/dashboards/$tid/comments" -d '{"parent_id":"'"$rid1"'","body":"Me too"}')"
  if [[ "$out" != 201 ]] || ! grep -q "\"parent_id\":\"$cid\"" "$TMP/api.body" || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body"; then
    fail "reply to a reply ($out: $(cat "$TMP/api.body"))"
  fi
  rid2="$(comment_ids "$TMP/api.body")"
  for bad in '{"parent_id":"'"$cid"'","version":1,"body":"x"}' '{"parent_id":"'"$cid"'","panel_id":"'"$panel"'","body":"x"}' \
    '{"parent_id":"'"$cid"'","body":"x","request_revision":true}' '{"parent_id":"x","body":"x"}'; do
    out="$(api POST "/dashboards/$tid/comments" -d "$bad")"
    if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
      fail "invalid reply ${bad:0:80} should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
    fi
  done
  out="$(api POST "/dashboards/$tid/comments" -d '{"parent_id":"'"$nil_id"'","body":"x"}')"
  [[ "$out" == 404 ]] || fail "reply to an unknown comment ($out: $(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$tid/comments" -d '{"parent_id":"'"$cid_other"'","body":"x"}')"
  [[ "$out" == 404 ]] || fail "reply to a comment of another dashboard ($out: $(cat "$TMP/api.body"))"
  pass "replies attach to their thread, one level deep, and inherit its panel"

  out="$(api PATCH "/dashboards/$tid/comments/$rid2" -d '{"status":"resolved","resolved_by_version":1}')"
  if [[ "$out" != 200 ]] || ! grep -q "\"id\":\"$cid\"" "$TMP/api.body" || ! grep -q '"parent_id":null' "$TMP/api.body" \
    || ! grep -q '"status":"resolved"' "$TMP/api.body" || ! grep -q '"resolved_by_version":1' "$TMP/api.body"; then
    fail "resolve the thread through a reply ($out: $(cat "$TMP/api.body"))"
  fi
  sse_wait "$stream_threads" "^data: {\"type\":\"comment.updated\",\"dashboard_id\":\"$tid\",.*\"comment_id\":\"$cid\"" 10
  out="$(api GET "/dashboards/$tid/comments?status=resolved")"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  if [[ "$out" != 200 || "$page_ids" != "$cid $rid1 $rid2 " ]] || [[ "$(grep -o '"status":"resolved"' "$TMP/api.body" | wc -l)" -ne 3 ]] \
    || [[ "$(grep -o '"resolved_by_version":null' "$TMP/api.body" | wc -l)" -ne 2 ]] || ! grep -q '"resolved_by_version":1' "$TMP/api.body"; then
    fail "resolved comments of the thread (ids '$page_ids': $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$tid/comments?status=open")"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  [[ "$out" == 200 && "$page_ids" == "$wid " ]] || fail "open comments after the thread was resolved (ids '$page_ids': $(cat "$TMP/api.body"))"
  out="$(api PATCH "/dashboards/$tid/comments/$rid2" -d '{"status":"resolved","resolved_by_version":1}')"
  if [[ "$out" != 200 ]] || ! grep -q "\"id\":\"$cid\"" "$TMP/api.body" || ! grep -q '"resolved_by_version":1' "$TMP/api.body"; then
    fail "resolve the thread again ($out: $(cat "$TMP/api.body"))"
  fi
  sleep 2
  updated="$(grep -c '^event: comment.updated$' "$stream_threads" || true)"
  [[ "$updated" == 1 ]] || fail "resolving a thread twice sent $updated comment.updated events"
  out="$(api PATCH "/dashboards/$tid/comments/$wid" -d '{"status":"resolved","resolved_by_version":99}')"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"unknown_version"' "$TMP/api.body"; then
    fail "resolve by an unknown version ($out: $(cat "$TMP/api.body"))"
  fi
  for bad in '{"status":"open","resolved_by_version":1}' '{"status":"resolved","resolved_by_version":0}'; do
    out="$(api PATCH "/dashboards/$tid/comments/$wid" -d "$bad")"
    [[ "$out" == 400 ]] || fail "$bad should answer 400 ($out: $(cat "$TMP/api.body"))"
  done
  out="$(api PATCH "/dashboards/$tid/comments/$cid" -d '{"status":"open"}')"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"open"' "$TMP/api.body" || ! grep -q '"resolved_by_version":null' "$TMP/api.body"; then
    fail "reopen the thread ($out: $(cat "$TMP/api.body"))"
  fi
  sse_close
  pass "resolve applies to the whole thread and records the version that fixed it"
}

revision_id_of() {
  grep -o '"revision":{"id":"[^"]*"' "$1" | head -1 | cut -d'"' -f6
}

wait_revision() {
  local dashboard="$1" revision="$2" status="$3" tries=$(($4 * 5))
  until [[ "$(api GET "/dashboards/$dashboard/revisions/$revision")" == 200 ]] && grep -q "\"status\":\"$status\"" "$TMP/api.body"; do
    ((--tries > 0)) || fail "revision $revision never became $status ($(cat "$TMP/api.body"))"
    sleep 0.2
  done
}

revisions_settled() {
  capture LAST api GET "/dashboards/$1/revisions?limit=200" || true
  [[ "$LAST" == 200 ]] && ! grep -Eq '"status":"(queued|running)"' "$TMP/api.body"
}

expect_version_data() {
  local dashboard="$1" version="$2" base out panels variables panel name value selections="" window now_ms tries
  base="/dashboards/$dashboard/versions/$version"
  out="$(api GET "$base")"
  [[ "$out" == 200 ]] || fail "read $base ($out: $(cat "$TMP/api.body"))"
  panels="$(grep -o '{"id":"[^"]*","type":"[a-z_]*","title"' "$TMP/api.body" | cut -d'"' -f4)"
  variables="$(grep -o '{"name":"[^"]*"' "$TMP/api.body" | cut -d'"' -f4 || true)"
  [[ -n "$panels" ]] || fail "$base lists no panels ($(cat "$TMP/api.body"))"
  now_ms=$(($(date +%s) * 1000))
  window="from=$((now_ms - 930000))&to=$((now_ms - 30000))"
  for name in $variables; do
    out="$(api GET "$base/variables/$name/values?$window$selections")"
    value="$(first_value)"
    [[ "$out" == 200 && -n "$value" ]] || fail "values of variable $name in $base ($out: $(cat "$TMP/api.body"))"
    selections+="&var-$name=$(urlencode "$value")"
  done
  for panel in $panels; do
    tries=30
    while :; do
      now_ms=$(($(date +%s) * 1000))
      window="from=$((now_ms - 900000))&to=$now_ms"
      if [[ "$(api GET "$base/panels/$panel/data?$window$selections")" == 200 ]] && grep -q '"frames":\[{' "$TMP/api.body"; then break; fi
      ((--tries > 0)) || fail "panel $panel of $base returned no data ($(cat "$TMP/api.body"))"
      sleep 2
    done
  done
}

revisions_suite() {
  api_token
  local out id id2 panel rid rid2 rid3 cid page_ids stream="$TMP/sse-revisions.txt" nil_id=00000000-0000-0000-0000-000000000000
  local accepted=0 limited=0 attempt succeeded
  local pid prid tdid troot treply trid tstream="$TMP/sse-thread-revision.txt"

  panel="$(sed -n 's/^ *"id": *"\([^"]*\)".*/\1/p' "$FIXTURES/dashboard-valid.json" | sed -n 1p)"
  [[ -n "$panel" ]] || fail "dashboard-valid.json has no panel id"
  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the revisions dashboard ($out: $(cat "$TMP/api.body"))"
  id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"

  sse_open "$id" "$stream"
  out="$(api POST "/dashboards/$id/comments" -d '{"version":1,"panel_id":"'"$panel"'","body":"Show this per route","request_revision":true}')"
  if [[ "$out" != 201 ]] || ! grep -q '"revision":{"id":"[^"]*","status":"queued"}' "$TMP/api.body"; then
    fail "a comment queues a revision ($out: $(cat "$TMP/api.body"))"
  fi
  rid="$(revision_id_of "$TMP/api.body")"
  cid="$(comment_ids "$TMP/api.body")"
  [[ -n "$rid" && -n "$cid" ]] || fail "queued revision or comment has no id ($(cat "$TMP/api.body"))"
  sse_wait "$stream" "^data: {\"type\":\"revision.updated\",\"dashboard_id\":\"$id\",\"comment_id\":\"$cid\",\"revision_id\":\"$rid\",\"status\":\"queued\"}$" 10
  sse_wait "$stream" "^data: {\"type\":\"revision.updated\",\"dashboard_id\":\"$id\",.*\"revision_id\":\"$rid\",\"status\":\"succeeded\"}$" 90
  sse_wait "$stream" "^data: {\"type\":\"version.created\",\"dashboard_id\":\"$id\",\"version\":2,\"parent_version\":1," 10
  sse_wait "$stream" "^data: {\"type\":\"revision.updated\",\"dashboard_id\":\"$id\",\"version\":2,\"comment_id\":\"$cid\",\"revision_id\":\"$rid\",\"status\":\"succeeded\"}$" 10
  pass "a comment queues a revision that saves version 2 and announces it on the event stream"

  out="$(api GET "/dashboards/$id/revisions/$rid")"
  if [[ "$out" != 200 ]] || ! grep -q '"status":"succeeded"' "$TMP/api.body" || ! grep -q '"result_version":2' "$TMP/api.body" \
    || ! grep -q '"source":"comment"' "$TMP/api.body" || ! grep -q "\"comment_id\":\"$cid\"" "$TMP/api.body" \
    || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body" || ! grep -q '"attempts":1' "$TMP/api.body" \
    || ! grep -q '"error":null' "$TMP/api.body" || ! grep -q '"base_version":1' "$TMP/api.body" \
    || ! grep -q '"prompt":"Show this per route"' "$TMP/api.body" || ! grep -q '"requested_by":{"kind":"service_account"' "$TMP/api.body"; then
    fail "the finished comment revision ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/versions/2")"
  if [[ "$out" != 200 ]] || ! grep -q '"summary":"Show requests per second by route"' "$TMP/api.body" \
    || ! grep -q '"author":{"kind":"service_account"' "$TMP/api.body" || ! grep -q 'sum by (route)' "$TMP/api.body"; then
    fail "the revised version 2 ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/comments")"
  if [[ "$out" != 200 ]] || ! grep -q "\"revision\":{\"id\":\"$rid\",\"status\":\"succeeded\"}" "$TMP/api.body" || ! grep -q '"status":"resolved"' "$TMP/api.body" || ! grep -q '"resolved_by_version":2' "$TMP/api.body" \
    || grep -q '"status":"open"' "$TMP/api.body"; then
    fail "the comment keeps its revision and is resolved by version 2 ($out: $(cat "$TMP/api.body"))"
  fi
  pass "the revision records its result, author and comment, and the comment is resolved by version 2"

  out="$(api POST "/dashboards/$id/revisions" -d '{"prompt":"  Add a p99 latency panel ","base_version":2}')"
  if [[ "$out" != 202 ]] || ! grep -q '"status":"queued"' "$TMP/api.body" || ! grep -q '"source":"prompt"' "$TMP/api.body" \
    || ! grep -q '"comment_id":null' "$TMP/api.body" || ! grep -q '"panel_id":null' "$TMP/api.body"; then
    fail "a follow-up prompt is queued ($out: $(cat "$TMP/api.body"))"
  fi
  rid2="$(grep -o '"revision":{"id":"[^"]*"' "$TMP/api.body" | cut -d'"' -f6)"
  grep -qi "^location: /api/v1/dashboards/$id/revisions/$rid2" "$TMP/api.headers" || fail "the revision response has no Location header ($(cat "$TMP/api.headers"))"
  sse_wait "$stream" "^data: {\"type\":\"version.created\",\"dashboard_id\":\"$id\",\"version\":3,\"parent_version\":2," 90
  wait_revision "$id" "$rid2" succeeded 10
  grep -q '"result_version":3' "$TMP/api.body" || fail "the follow-up revision should save version 3 ($(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id")"
  if [[ "$out" != 200 ]] || ! grep -q '"latest_version":3' "$TMP/api.body" || ! grep -q '"id":"p99_latency"' "$TMP/api.body"; then
    fail "the dashboard after the follow-up ($out: $(cat "$TMP/api.body"))"
  fi
  sse_close
  pass "a follow-up prompt queues a revision that saves version 3 with the new panel"

  expect_version_data "$id" 2
  expect_version_data "$id" 3
  pass "every panel of the revised versions returns data through the query proxy"

  out="$(api GET "/dashboards/$id/revisions")"
  page_ids="$(comment_ids "$TMP/api.body" | tr '\n' ' ')"
  if [[ "$out" != 200 || "$page_ids" != "$rid2 $rid " ]] || ! grep -q '"next_cursor":null' "$TMP/api.body"; then
    fail "list the revisions, newest first (ids '$page_ids': $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id/revisions?limit=1")"
  if [[ "$out" != 200 ]] || ! grep -q '"next_cursor":"[^"]' "$TMP/api.body"; then
    fail "page the revisions ($out: $(cat "$TMP/api.body"))"
  fi
  pass "revisions list newest first with cursors"

  out="$(api POST "/dashboards/$id/revisions" -d '{"prompt":"No fixture covers this prompt","base_version":3}')"
  [[ "$out" == 202 ]] || fail "queue a prompt the replay provider cannot answer ($out: $(cat "$TMP/api.body"))"
  rid3="$(grep -o '"revision":{"id":"[^"]*"' "$TMP/api.body" | cut -d'"' -f6)"
  wait_revision "$id" "$rid3" failed 30
  if ! grep -q '"error":"model call failed: no replay fixture"' "$TMP/api.body" || ! grep -q '"result_version":null' "$TMP/api.body" \
    || ! grep -q '"attempts":1' "$TMP/api.body"; then
    fail "a failed revision records a safe error ($(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$id")"
  if [[ "$out" != 200 ]] || ! grep -q '"latest_version":3' "$TMP/api.body"; then
    fail "a failed revision must not save a version ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a revision the agent cannot complete fails with a safe error and saves nothing"

  for bad in '{"base_version":1}' '{"prompt":"x"}' '{"prompt":"   ","base_version":1}' '{"prompt":"a\u0000b","base_version":1}' \
    '{"prompt":"a\u202eb","base_version":1}' '{"prompt":"x","base_version":0}' '{"prompt":"x","base_version":"1"}' \
    '{"prompt":"x","base_version":1,"extra":1}' "{\"prompt\":\"$(printf 'a%.0s' $(seq 1 4001))\",\"base_version\":1}"; do
    out="$(api POST "/dashboards/$id/revisions" -d "$bad")"
    if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
      fail "invalid revision ${bad:0:60} should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
    fi
  done
  out="$(api POST "/dashboards/$id/revisions" -d '{"prompt":"x","base_version":99}')"
  [[ "$out" == 404 ]] || fail "revision against an unknown version ($out: $(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$nil_id/revisions" -d '{"prompt":"x","base_version":1}')"
  [[ "$out" == 404 ]] || fail "revision of an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api POST /dashboards/not-a-uuid/revisions -d '{"prompt":"x","base_version":1}')"
  [[ "$out" == 400 ]] || fail "revision of a malformed dashboard id ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$nil_id/revisions")"
  [[ "$out" == 404 ]] || fail "list revisions of an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id/revisions/$nil_id")"
  [[ "$out" == 404 ]] || fail "read an unknown revision ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id/revisions/not-a-uuid")"
  [[ "$out" == 400 ]] || fail "read a malformed revision id ($out: $(cat "$TMP/api.body"))"
  for bad in limit=0 limit=201 limit=x 'cursor=!!!'; do
    out="$(api GET "/dashboards/$id/revisions?$bad")"
    [[ "$out" == 400 ]] || fail "list revisions with $bad should answer 400 ($out: $(cat "$TMP/api.body"))"
  done
  out="$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"prompt":"x","base_version":1}' "$BASE_URL/api/v1/dashboards/$id/revisions")"
  [[ "$out" == 401 ]] || fail "revision without a token ($out)"
  pass "revision requests refuse invalid bodies, unknown versions and dashboards, bad paging and anonymous callers"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the thread revision dashboard ($out: $(cat "$TMP/api.body"))"
  tdid="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  out="$(api POST "/dashboards/$tdid/comments" -d '{"version":1,"panel_id":"'"$panel"'","body":"Split this by route"}')"
  troot="$(comment_ids "$TMP/api.body")"
  [[ "$out" == 201 && -n "$troot" ]] || fail "create the thread root ($out: $(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$tdid/comments" -d '{"parent_id":"'"$troot"'","body":"Agreed"}')"
  treply="$(comment_ids "$TMP/api.body")"
  [[ "$out" == 201 && -n "$treply" ]] || fail "create the thread reply ($out: $(cat "$TMP/api.body"))"
  sse_open "$tdid" "$tstream"
  out="$(api POST "/dashboards/$tdid/revisions" -d '{"prompt":"Show this per route","base_version":1,"comment_id":"'"$treply"'"}')"
  if [[ "$out" != 202 ]] || ! grep -q '"source":"comment"' "$TMP/api.body" || ! grep -q "\"comment_id\":\"$troot\"" "$TMP/api.body" \
    || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body"; then
    fail "a request sent from a reply is queued against the thread's root ($out: $(cat "$TMP/api.body"))"
  fi
  trid="$(grep -o '"revision":{"id":"[^"]*"' "$TMP/api.body" | cut -d'"' -f6)"
  [[ -n "$trid" ]] || fail "the thread request has no id ($(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$tdid/revisions" -d '{"prompt":"x","base_version":1,"comment_id":"'"$treply"'","panel_id":"'"$panel"'"}')"
  if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
    fail "comment_id with panel_id should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$tdid/revisions" -d '{"prompt":"x","base_version":1,"comment_id":"nope"}')"
  if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
    fail "a malformed comment_id should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$tdid/revisions" -d '{"prompt":"x","base_version":1,"comment_id":"'"$nil_id"'"}')"
  if [[ "$out" != 404 ]] || ! grep -q '"code":"not_found"' "$TMP/api.body"; then
    fail "an unknown comment_id should answer 404 ($out: $(cat "$TMP/api.body"))"
  fi
  wait_revision "$tdid" "$trid" succeeded 90
  sse_wait "$tstream" "^data: {\"type\":\"comment.updated\",\"dashboard_id\":\"$tdid\",.*\"comment_id\":\"$troot\",\"panel_id\":\"$panel\",\"status\":\"resolved\"}$" 10
  sse_close
  out="$(api GET "/dashboards/$tdid/comments?status=resolved")"
  if [[ "$out" != 200 ]] || ! python3 -c 'import json,sys
b=json.load(open(sys.argv[1]))
c=[x for x in b["comments"] if x["id"]==sys.argv[2]]
sys.exit(0 if len(c)==1 and c[0]["status"]=="resolved" and c[0]["resolved_by_version"]==2 else 1)' "$TMP/api.body" "$troot"; then
    fail "the thread should be resolved by version 2 ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a request sent from a comment thread resolves the thread with the version it saved"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the revision cap dashboard ($out: $(cat "$TMP/api.body"))"
  id2="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  for attempt in $(seq 1 12); do
    out="$(api POST "/dashboards/$id2/revisions" -d '{"prompt":"Add a latency panel after checking the data","base_version":1}')"
    if [[ "$out" == 202 ]]; then
      accepted=$((accepted + 1))
    elif [[ "$out" == 429 ]] && grep -q '"code":"too_many_revisions"' "$TMP/api.body"; then
      limited=1
      break
    else
      fail "rapid revision $attempt ($out: $(cat "$TMP/api.body"))"
    fi
  done
  ((limited == 1 && accepted >= 5 && accepted <= 11)) || fail "rapid revisions: $accepted accepted, limited=$limited"
  pass "at most five revisions are queued or running per dashboard"

  poll_try 120 1 revisions_settled "$id2" || fail "queued revisions did not finish ($(cat "$TMP/api.body"))"
  succeeded="$(grep -o '"status":"succeeded"' "$TMP/api.body" | wc -l | tr -d ' ')"
  [[ "$succeeded" == "$accepted" ]] || fail "$succeeded revisions succeeded out of $accepted accepted ($(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$id2/versions")"
  if [[ "$out" != 200 ]] || [[ "$(grep -o '"version":[0-9]*,"parent_version"' "$TMP/api.body" | wc -l | tr -d ' ')" != "$((succeeded + 1))" ]]; then
    fail "each revision should save exactly one version ($succeeded revisions: $(cat "$TMP/api.body"))"
  fi
  pass "every queued revision saves exactly one version, one at a time per dashboard"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the panel revision dashboard ($out: $(cat "$TMP/api.body"))"
  pid="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  out="$(api POST "/dashboards/$pid/revisions" -d '{"prompt":"Focus on this panel","base_version":1,"panel_id":"'"$panel"'"}')"
  if [[ "$out" != 202 ]] || ! grep -q "\"panel_id\":\"$panel\"" "$TMP/api.body" || ! grep -q '"source":"prompt"' "$TMP/api.body"; then
    fail "a revision aimed at one panel is queued ($out: $(cat "$TMP/api.body"))"
  fi
  prid="$(revision_id_of "$TMP/api.body")"
  out="$(api POST "/dashboards/$pid/revisions" -d '{"prompt":"Focus on this panel","base_version":1,"panel_id":"no_such_panel"}')"
  if [[ "$out" != 422 ]] || ! grep -q '"code":"unknown_panel"' "$TMP/api.body"; then
    fail "a revision aimed at an unknown panel is refused ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api POST "/dashboards/$pid/revisions" -d '{"prompt":"Focus on this panel","base_version":1,"panel_id":""}')"
  [[ "$out" == 400 ]] || fail "a revision with an empty panel_id should answer 400 ($out: $(cat "$TMP/api.body"))"
  wait_revision "$pid" "$prid" failed 90
  pass "a revision can target one panel, and an unknown panel is refused"
}

observe_id_of() {
  grep -o '"request":{"id":"[^"]*"' "$1" | head -1 | cut -d'"' -f6
}

observe_wait() {
  local id="$1" tries="$2"
  until [[ "$(api GET "/observe/$id")" == 200 ]] && grep -Eq '"status":"(succeeded|failed)"' "$TMP/api.body"; do
    ((--tries > 0)) || fail "agent request $id never finished ($(cat "$TMP/api.body"))"
    sleep 1
  done
  cat "$TMP/api.body" >>"$TMP/observe-bodies.txt"
}

observe_expect() {
  local what="$1" pattern
  shift
  for pattern in "$@"; do
    grep -qF -- "$pattern" "$TMP/api.body" || fail "$what lacks $pattern ($(cat "$TMP/api.body"))"
  done
}

observe_suite() {
  api_token
  local out id did fid id2 did2 nil_id=00000000-0000-0000-0000-000000000000 bad attempt accepted_ids=() long_prompt
  local fpos ipos cursor listed_first listed_second page_id
  : >"$TMP/observe-bodies.txt"

  out="$(api GET /agent)"
  [[ "$out" == 200 ]] || fail "read the agent configuration ($out: $(cat "$TMP/api.body"))"
  observe_expect "the agent configuration" '"available":true,"provider":"replay","model":"replay","effort":null'
  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/agent")"
  [[ "$out" == 401 ]] || fail "read the agent configuration without a token ($out)"
  pass "the agent configuration names the replay provider with no effort and needs a token"

  out="$(api POST /observe -d '{"prompt":"demo shop RED metrics by endpoint"}')"
  [[ "$out" == 202 ]] || fail "queue a new dashboard request ($out: $(cat "$TMP/api.body"))"
  observe_expect "the queued observe request" '"source":"observe"' '"status":"queued"' '"dashboard_id":null' '"base_version":null' '"steps":[]' '"usage":null' '"url":null' '"origin":"api"'
  id="$(observe_id_of "$TMP/api.body")"
  [[ -n "$id" ]] || fail "the queued observe request has no id ($(cat "$TMP/api.body"))"
  grep -qi "^location: /api/v1/observe/$id" "$TMP/api.headers" || fail "the observe response has no Location header ($(cat "$TMP/api.headers"))"
  observe_wait "$id" 90
  did="$(grep -o '"dashboard_id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  [[ -n "$did" ]] || fail "the finished observe request has no dashboard ($(cat "$TMP/api.body"))"
  observe_expect "the finished observe request" '"status":"succeeded"' '"result_version":1' '"attempts":1' '"error":null' "\"url\":\"$BASE_URL/dashboards/$did\"" \
    '"phase":"save","tool":"create_dashboard","status":"error"' '"phase":"repair","tool":"create_dashboard","status":"ok"' \
    '"provider":"replay"' '"model_calls":4' '"repair_rounds":1'
  pass "a new dashboard request repairs its broken query and finishes with steps, usage and the dashboard address"

  out="$(api GET "/dashboards/$did")"
  if [[ "$out" != 200 ]] || ! grep -q '"latest_version":1' "$TMP/api.body"; then
    fail "the observed dashboard ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards?limit=50")"
  [[ "$out" == 200 && "$(grep -o '"title":"Demo shop RED metrics"' "$TMP/api.body" | wc -l | tr -d ' ')" == 1 ]] \
    || fail "exactly one dashboard per observe request ($out: $(cat "$TMP/api.body"))"
  expect_version_data "$did" 1
  pass "the observed dashboard has version 1, exists once, and every panel returns data"

  out="$(api POST /observe -d '{"prompt":"add a requests by pod panel","dashboard_id":"'"$did"'"}')"
  [[ "$out" == 202 ]] || fail "queue a follow-up prompt ($out: $(cat "$TMP/api.body"))"
  observe_expect "the queued follow-up" '"source":"prompt"' '"base_version":1' "\"dashboard_id\":\"$did\""
  fid="$(observe_id_of "$TMP/api.body")"
  observe_wait "$fid" 90
  observe_expect "the finished follow-up" '"status":"succeeded"' '"result_version":2'
  out="$(api GET "/dashboards/$did/revisions/$fid")"
  [[ "$out" == 200 ]] || fail "the follow-up is a revision of the dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/dashboards/$did/revisions")"
  if [[ "$out" != 200 ]] || ! grep -q "$id" "$TMP/api.body" || ! grep -q "$fid" "$TMP/api.body"; then
    fail "the revisions of the dashboard hold the follow-up and the observe request ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/dashboards/$did/revisions/$id")"
  if [[ "$out" != 200 ]] || ! grep -q '"source":"observe"' "$TMP/api.body" || ! grep -q '"base_version":null' "$TMP/api.body"; then
    fail "the observe request is a revision of its dashboard with no base version ($out: $(cat "$TMP/api.body"))"
  fi
  expect_version_data "$did" 2
  pass "a follow-up prompt saves version 2 and the dashboard's revisions start with the chat request that created it"

  out="$(api GET "/observe?limit=100")"
  if [[ "$out" != 200 ]] || ! grep -qF "\"id\":\"$fid\"" "$TMP/api.body" || ! grep -qF "\"id\":\"$id\"" "$TMP/api.body" \
    || ! grep -qF '"dashboard_title":"Demo shop RED metrics"' "$TMP/api.body"; then
    fail "the observe list holds the follow-up and the chat request with the dashboard title ($out: $(cat "$TMP/api.body"))"
  fi
  fpos="$(tr ',' '\n' <"$TMP/api.body" | grep -nF "\"id\":\"$fid\"" | head -1 | cut -d: -f1)"
  ipos="$(tr ',' '\n' <"$TMP/api.body" | grep -nF "\"id\":\"$id\"" | head -1 | cut -d: -f1)"
  ((fpos > 0 && fpos < ipos)) || fail "the observe list should show the follow-up before the chat request (positions '$fpos' and '$ipos')"
  listed_first="$(sed -n 's/.*"requests":\[{"id":"\([^"]*\)".*/\1/p' "$TMP/api.body")"
  listed_second="$(sed -n 's/^[^}]*},{"id":"\([^"]*\)".*/\1/p' "$TMP/api.body")"
  [[ -n "$listed_first" && -n "$listed_second" ]] || fail "the observe list should hold at least two requests ($(cat "$TMP/api.body"))"
  out="$(api GET "/observe?source=observe&limit=100")"
  if [[ "$out" != 200 ]] || ! grep -qF "\"id\":\"$id\"" "$TMP/api.body" || grep -qF "\"id\":\"$fid\"" "$TMP/api.body"; then
    fail "the observe list filtered by source=observe ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/observe?limit=1")"
  page_id="$(sed -n 's/.*"requests":\[{"id":"\([^"]*\)".*/\1/p' "$TMP/api.body")"
  [[ "$page_id" == "$listed_first" ]] || fail "the first observe page should hold the newest request ('$page_id' instead of '$listed_first')"
  cursor="$(sed -n 's/.*"next_cursor":"\([^"]*\)".*/\1/p' "$TMP/api.body")"
  if [[ "$out" != 200 || -z "$cursor" ]]; then
    fail "the first page of the observe list has a next_cursor ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(api GET "/observe?limit=1&cursor=$(urlencode "$cursor")")"
  [[ "$out" == 200 ]] || fail "follow the observe list cursor ($out: $(cat "$TMP/api.body"))"
  page_id="$(sed -n 's/.*"requests":\[{"id":"\([^"]*\)".*/\1/p' "$TMP/api.body")"
  if [[ "$page_id" == "$listed_first" || "$page_id" != "$listed_second" ]]; then
    fail "the second observe page should hold the second request ('$page_id', expected '$listed_second' and not '$listed_first')"
  fi
  for bad in source=bogus source= limit=0 limit=101 cursor=%40%40; do
    out="$(api GET "/observe?$bad")"
    [[ "$out" == 400 ]] || fail "the observe list with $bad should answer 400 ($out: $(cat "$TMP/api.body"))"
  done
  out="$(command curl -sS -o "$TMP/api.body" -w '%{http_code}' -b "$TMP/api-jar" "$BASE_URL/api/v1/observe?limit=100")"
  if [[ "$out" != 200 ]] || grep -qF "$id" "$TMP/api.body"; then
    fail "the browser session must not see another principal's requests ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(command curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/observe")"
  [[ "$out" == 401 ]] || fail "the observe list without credentials ($out)"
  pass "the observe list shows only the caller's requests, newest first, with filters and cursors"

  out="$(api POST /observe -d '{"prompt":"log volume by service"}')"
  [[ "$out" == 202 ]] || fail "queue the log volume request ($out: $(cat "$TMP/api.body"))"
  id2="$(observe_id_of "$TMP/api.body")"
  observe_wait "$id2" 90
  observe_expect "the finished log volume request" '"status":"succeeded"' '"result_version":1'
  did2="$(grep -o '"dashboard_id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  [[ -n "$did2" && "$did2" != "$did" ]] || fail "the log volume request needs its own dashboard ($(cat "$TMP/api.body"))"
  expect_version_data "$did2" 1
  pass "a ClickHouse dashboard request saves version 1 with data in every panel"

  out="$(api POST /observe -d '{"prompt":"e2e: a slow run that never saves"}')"
  [[ "$out" == 202 ]] || fail "queue the run that never saves ($out: $(cat "$TMP/api.body"))"
  id2="$(observe_id_of "$TMP/api.body")"
  observe_wait "$id2" 90
  observe_expect "the failed request" '"status":"failed"' '"error":"the model finished without saving a dashboard"' '"dashboard_id":null' '"url":null' \
    '"result_version":null' '"phase":"query"' '"model_calls":6'
  pass "a run that never saves fails with a fixed error, no dashboard and its usage"

  out="$(api POST /observe -d '{"prompt":"No fixture covers this observe prompt"}')"
  [[ "$out" == 202 ]] || fail "queue a prompt the replay provider cannot answer ($out: $(cat "$TMP/api.body"))"
  id2="$(observe_id_of "$TMP/api.body")"
  observe_wait "$id2" 90
  observe_expect "the unanswerable request" '"status":"failed"' '"error":"model call failed: no replay fixture"' '"usage":null'
  pass "a prompt with no fixture fails with a safe error"

  long_prompt="$(printf 'a%.0s' $(seq 1 4001))"
  for bad in '{}' '{"prompt":"   "}' '{"prompt":"a\u0000b"}' '{"prompt":"a\u202eb"}' '{"prompt":"x","base_version":1}' \
    '{"prompt":"x","dashboard_id":"not-a-uuid"}' "{\"prompt\":\"x\",\"dashboard_id\":\"$LOOKALIKE_UUID\"}" \
    "{\"prompt\":\"x\",\"dashboard_id\":\"$did\",\"base_version\":0}" '{"prompt":"x","extra":1}' "{\"prompt\":\"$long_prompt\"}"; do
    out="$(api POST /observe -d "$bad")"
    if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
      fail "invalid observe request ${bad:0:60} should be rejected with 400 ($out: $(cat "$TMP/api.body"))"
    fi
  done
  out="$(api POST /observe -d '{"prompt":"x","dashboard_id":"'"$nil_id"'"}')"
  [[ "$out" == 404 ]] || fail "observe request for an unknown dashboard ($out: $(cat "$TMP/api.body"))"
  out="$(api POST /observe -d '{"prompt":"x","dashboard_id":"'"$did"'","base_version":99}')"
  [[ "$out" == 404 ]] || fail "observe request against an unknown version ($out: $(cat "$TMP/api.body"))"
  out="$(api GET "/observe/$nil_id")"
  [[ "$out" == 404 ]] || fail "read an unknown agent request ($out: $(cat "$TMP/api.body"))"
  for bad in not-a-uuid "$LOOKALIKE_UUID"; do
    out="$(api GET "/observe/$bad")"
    [[ "$out" == 400 ]] || fail "read agent request $bad should answer 400 ($out: $(cat "$TMP/api.body"))"
  done
  out="$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"prompt":"x"}' "$BASE_URL/api/v1/observe")"
  [[ "$out" == 401 ]] || fail "observe request without a token ($out)"
  pass "observe requests refuse invalid bodies, unknown dashboards, versions and ids, and anonymous callers"

  hold_claim_lock
  poll 30 "the test holds the agent request claim lock" claim_lock_held
  for attempt in 1 2 3; do
    out="$(api POST /observe -d '{"prompt":"e2e: a slow run that never saves"}')"
    [[ "$out" == 202 ]] || fail "observe request $attempt of 3 should be accepted ($out: $(cat "$TMP/api.body"))"
    accepted_ids+=("$(observe_id_of "$TMP/api.body")")
  done
  out="$(api POST /observe -d '{"prompt":"e2e: a slow run that never saves"}')"
  if [[ "$out" != 429 ]] || ! grep -q '"code":"too_many_requests"' "$TMP/api.body"; then
    fail "the fourth queued observe request should be refused with too_many_requests ($out: $(cat "$TMP/api.body"))"
  fi
  release_claim_lock
  for id2 in "${accepted_ids[@]}"; do
    observe_wait "$id2" 90
  done
  pass "at most three new dashboard requests are queued or running per caller"

  if grep -Eq 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' "$TMP/observe-bodies.txt"; then
    fail "an agent request leaks datasource addresses or credentials ($(grep -E 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' "$TMP/observe-bodies.txt" | head -1))"
  fi
  pass "agent requests expose no datasource address or credential"
}

mcp_call() {
  local id="$1" method="$2" params="$3" out_file="$4"
  shift 4
  printf '{"jsonrpc":"2.0","id":%s,"method":"%s","params":%s}' "$id" "$method" "$params" >"$TMP/mcp-request.json"
  command curl -sS --max-time 60 -o "$out_file" -D "$TMP/mcp.headers" -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $API_TOKEN" -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' -H 'MCP-Protocol-Version: 2025-11-25' \
    "$@" --data-binary "@$TMP/mcp-request.json" "$BASE_URL/mcp"
}

mcp_tool() {
  local name="$1" args="$2"
  MCP_STATUS="$(mcp_call 1 tools/call "{\"name\":\"$name\",\"arguments\":$args}" "$TMP/mcp.body")"
  [[ "$MCP_STATUS" == 200 ]] || fail "MCP tools/call $name ($MCP_STATUS: $(cat "$TMP/mcp.body"))"
  cat "$TMP/mcp.body" >>"$TMP/mcp-all.body"
  if grep -q '"isError":true' "$TMP/mcp.body"; then MCP_ERROR=true; else MCP_ERROR=false; fi
}

mcp_expect() {
  local desc="$1" pattern="$2"
  grep -q -- "$pattern" "$TMP/mcp.body" || fail "$desc: expected $pattern ($(head -c 1500 "$TMP/mcp.body"))"
  [[ "$MCP_ERROR" == false ]] || fail "$desc returned a tool error ($(head -c 1500 "$TMP/mcp.body"))"
}

mcp_suite() {
  api_token
  local out id name leak label args panel cid wid vid
  local tool_names=(create_dashboard get_dashboard get_schema get_spec_guide lint_query list_comments list_dashboards list_datasources
    list_label_values list_labels list_metrics query_instant query_range reply_to_comment resolve_comment sample_rows update_dashboard)
  : >"$TMP/mcp-all.body"

  out="$(command curl -sS --max-time 10 -o "$TMP/mcp.body" -D "$TMP/mcp.headers" -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$BASE_URL/mcp")"
  if [[ "$out" != 401 ]] || ! grep -qi '^www-authenticate: Bearer' "$TMP/mcp.headers"; then
    fail "MCP without a key ($out: $(cat "$TMP/mcp.body"))"
  fi
  out="$(command curl -sS --max-time 10 -o "$TMP/mcp.body" -w '%{http_code}' -b "$TMP/api-jar" "$BASE_URL/api/v1/dashboards")"
  [[ "$out" == 200 ]] || fail "the session cookie is not live on the API ($out: $(cat "$TMP/mcp.body"))"
  out="$(command curl -sS --max-time 10 -o "$TMP/mcp.body" -w '%{http_code}' -X POST -b "$TMP/api-jar" \
    -H "Origin: $BASE_URL" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$BASE_URL/mcp")"
  [[ "$out" == 401 ]] || fail "MCP with a session cookie and no key ($out: $(cat "$TMP/mcp.body"))"
  pass "MCP refuses callers without a bearer key, session cookies included"

  out="$(mcp_call 1 tools/list '{}' "$TMP/mcp.body" -H 'Origin: http://evil.example')"
  [[ "$out" == 403 ]] || fail "MCP with a foreign origin ($out: $(cat "$TMP/mcp.body"))"
  out="$(mcp_call 1 tools/list '{}' "$TMP/mcp.body" -H "Origin: $BASE_URL")"
  [[ "$out" == 200 ]] || fail "MCP with the public origin ($out: $(cat "$TMP/mcp.body"))"
  pass "MCP refuses a foreign origin and accepts the public one"

  out="$(mcp_call 1 initialize '{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}' "$TMP/mcp.body")"
  if [[ "$out" != 200 ]] || ! grep -q '"name":"slashobserve"' "$TMP/mcp.body"; then
    fail "MCP initialize ($out: $(cat "$TMP/mcp.body"))"
  fi
  grep -Ei '^mcp-session-id: so1\.[0-9a-f]{32}\.ZTJl' "$TMP/mcp.headers" >/dev/null || fail "MCP initialize did not label the client name ($(cat "$TMP/mcp.headers"))"
  label="$(grep -i '^mcp-session-id:' "$TMP/mcp.headers" | head -1 | cut -d' ' -f2 | tr -d '\r')"
  out="$(mcp_call 1 initialize '{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"","version":"0"}}' "$TMP/mcp.body")"
  if [[ "$out" != 200 ]] || grep -qi '^mcp-session-id:' "$TMP/mcp.headers"; then
    fail "MCP initialize without a client name should answer without a session id ($out: $(cat "$TMP/mcp.headers"))"
  fi
  pass "MCP initialize labels the client name in the session id and keeps no session state"

  out="$(mcp_call 2 tools/list '{}' "$TMP/mcp.body")"
  [[ "$out" == 200 ]] || fail "MCP tools/list ($out: $(cat "$TMP/mcp.body"))"
  for name in "${tool_names[@]}"; do
    grep -q "\"name\":\"$name\"" "$TMP/mcp.body" || fail "MCP tools/list lacks $name ($(cat "$TMP/mcp.body"))"
  done
  cat "$TMP/mcp.body" >>"$TMP/mcp-all.body"
  pass "MCP tools/list returns all ${#tool_names[@]} tools"

  mcp_tool get_spec_guide '{}'
  mcp_expect "get_spec_guide" '"guide"'
  pass "MCP get_spec_guide returns the authoring guide"

  mcp_tool list_datasources '{}'
  mcp_expect "list_datasources" '"uid":"prom"'
  mcp_expect "list_datasources" '"uid":"ch"'
  pass "MCP list_datasources returns both datasources"

  mcp_tool list_metrics '{"datasource":"prom","search":"http_requests"}'
  mcp_expect "list_metrics" 'http_requests_total'
  mcp_tool list_labels '{"datasource":"prom","metric":"http_requests_total"}'
  mcp_expect "list_labels" '"pod"'
  mcp_tool list_label_values '{"datasource":"prom","label":"service","metric":"http_requests_total"}'
  mcp_expect "list_label_values" '"checkout"'
  pass "MCP discovers Prometheus metrics, labels and label values"

  mcp_tool get_schema '{"datasource":"ch","table":"otel_logs"}'
  mcp_expect "get_schema" 'ServiceName'
  mcp_tool sample_rows '{"datasource":"ch","table":"otel_logs","limit":2}'
  mcp_expect "sample_rows" '"rows"'
  pass "MCP discovers ClickHouse schemas and samples rows"

  mcp_tool lint_query '{"datasource":"prom","query":"sum(rate("}'
  mcp_expect "lint_query" '"ok":false'
  pass "MCP lint_query reports an unparseable query"

  mcp_tool query_range '{"datasource":"prom","query":"sum(rate(http_requests_total[$__rate_interval]))","from":"now-15m"}'
  mcp_expect "query_range" '"empty":false'
  mcp_expect "query_range" '"series"'
  mcp_tool query_instant '{"datasource":"prom","query":"sum(http_requests_total)"}'
  mcp_expect "query_instant" '"empty":false'
  pass "MCP runs range and instant queries and returns a summary"

  mcp_tool create_dashboard "$(cat "$FIXTURES/dashboard-valid.json")"
  mcp_expect "create_dashboard" '"dashboard_id":"[0-9a-f-]\{36\}"'
  id="$(grep -o '"dashboard_id":"[^"]*"' "$TMP/mcp.body" | head -1 | cut -d'"' -f4)"
  out="$(api GET "/dashboards/$id")"
  if [[ "$out" != 200 ]] || ! grep -q '"updated_by":{[^}]*"kind":"service_account"' "$TMP/api.body"; then
    fail "dashboard created over MCP ($out: $(cat "$TMP/api.body"))"
  fi
  pass "MCP create_dashboard saves a dashboard attributed to the service account"

  out="$(api GET "/dashboards/$id/versions/1")"
  if [[ "$out" != 200 ]] || ! grep -q '"origin":"mcp"' "$TMP/api.body" || ! grep -q '"origin_client":null' "$TMP/api.body"; then
    fail "a version saved over MCP should record the mcp origin ($out: $(cat "$TMP/api.body"))"
  fi

  mcp_tool get_dashboard "{\"dashboard_id\":\"$id\"}"
  mcp_expect "get_dashboard" '"version":1'
  mcp_tool list_dashboards '{}'
  mcp_expect "list_dashboards" "\"id\":\"$id\""
  pass "MCP reads back the dashboard it created"

  args="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d["dashboard_id"]=sys.argv[2]; d["parent_version"]=int(sys.argv[3]); print(json.dumps(d))' "$FIXTURES/dashboard-valid.json" "$id" 1)"
  out="$(mcp_call 5 tools/call "{\"name\":\"update_dashboard\",\"arguments\":$args,\"_meta\":{\"io.modelcontextprotocol/clientInfo\":{\"name\":\"claude-code\",\"version\":\"9\"}}}" "$TMP/mcp.body")"
  if [[ "$out" != 200 ]] || grep -q '"isError":true' "$TMP/mcp.body"; then
    fail "MCP update_dashboard with clientInfo in _meta ($out: $(head -c 1500 "$TMP/mcp.body"))"
  fi
  out="$(api GET "/dashboards/$id/versions/2")"
  if [[ "$out" != 200 ]] || ! grep -q '"origin":"mcp"' "$TMP/api.body" || ! grep -q '"origin_client":"claude-code"' "$TMP/api.body"; then
    fail "version 2 should record the client name from _meta ($out: $(cat "$TMP/api.body"))"
  fi
  args="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d["dashboard_id"]=sys.argv[2]; d["parent_version"]=int(sys.argv[3]); print(json.dumps(d))' "$FIXTURES/dashboard-valid.json" "$id" 2)"
  out="$(mcp_call 6 tools/call "{\"name\":\"update_dashboard\",\"arguments\":$args}" "$TMP/mcp.body" -H "Mcp-Session-Id: $label")"
  if [[ "$out" != 200 ]] || grep -q '"isError":true' "$TMP/mcp.body"; then
    fail "MCP update_dashboard with a session label ($out: $(head -c 1500 "$TMP/mcp.body"))"
  fi
  out="$(api GET "/dashboards/$id/versions/3")"
  if [[ "$out" != 200 ]] || ! grep -q '"origin_client":"e2e"' "$TMP/api.body"; then
    fail "version 3 should record the client name from the session label ($out: $(cat "$TMP/api.body"))"
  fi
  pass "MCP saves record the client name from _meta or from the session label"

  mcp_tool create_dashboard "$(cat "$FIXTURES/dashboard-failing-query.json")"
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'query_invalid' "$TMP/mcp.body"; then
    fail "create_dashboard with a failing query ($(cat "$TMP/mcp.body"))"
  fi
  pass "MCP create_dashboard returns a tool error for a panel that fails"

  mcp_tool update_dashboard '{"spec":{"specVersion":"0","title":"x","panels":[]}}'
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'dashboard_id' "$TMP/mcp.body"; then
    fail "update_dashboard without a dashboard_id ($(cat "$TMP/mcp.body"))"
  fi
  mcp_tool update_dashboard "{\"dashboard_id\":\"$id\",\"spec\":{\"specVersion\":\"0\",\"title\":\"x\",\"panels\":[]}}"
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'parent_version' "$TMP/mcp.body"; then
    fail "update_dashboard without a parent_version ($(cat "$TMP/mcp.body"))"
  fi
  pass "MCP update_dashboard demands dashboard_id and parent_version"

  panel="$(sed -n 's/^ *"id": *"\([^"]*\)".*/\1/p' "$FIXTURES/dashboard-valid.json" | sed -n 1p)"
  [[ -n "$panel" ]] || fail "dashboard-valid.json has no panel id"
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 1 "$panel" 'Please add p99')")"
  [[ "$out" == 201 ]] || fail "create a panel comment for MCP ($out: $(cat "$TMP/api.body"))"
  cid="$(comment_ids "$TMP/api.body")"
  [[ -n "$cid" ]] || fail "no comment id in the panel comment response ($(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$id/comments" -d '{"version":1,"body":"Overall layout feedback","request_revision":false}')"
  [[ "$out" == 201 ]] || fail "create a whole-dashboard comment for MCP ($out: $(cat "$TMP/api.body"))"
  wid="$(comment_ids "$TMP/api.body")"
  [[ -n "$wid" ]] || fail "no comment id in the whole-dashboard comment response ($(cat "$TMP/api.body"))"
  mcp_tool list_comments "{\"dashboard_id\":\"$id\"}"
  mcp_expect "list_comments" "\"id\":\"$cid\""
  mcp_expect "list_comments" "\"id\":\"$wid\""
  mcp_expect "list_comments" '"panel_id":null'
  mcp_tool reply_to_comment "{\"comment_id\":\"$cid\",\"body\":\"Added p99 in the next version\"}"
  mcp_expect "reply_to_comment" "\"parent_id\":\"$cid\""
  out="$(api GET "/dashboards/$id/comments")"
  [[ "$out" == 200 ]] || fail "list comments after an MCP reply ($out: $(cat "$TMP/api.body"))"
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); r=[c for c in d["comments"] if c["parent_id"]==sys.argv[2]]; sys.exit(0 if len(r)==1 and r[0]["author"]["kind"]=="service_account" else 1)' "$TMP/api.body" "$cid" \
    || fail "the MCP reply should be a service-account reply to $cid ($(cat "$TMP/api.body"))"
  mcp_tool resolve_comment "{\"comment_id\":\"$cid\",\"version\":1}"
  mcp_expect "resolve_comment" '"changed":true'
  mcp_expect "resolve_comment" '"resolved_by_version":1'
  mcp_tool resolve_comment "{\"comment_id\":\"$cid\",\"version\":1}"
  mcp_expect "resolve_comment again" '"changed":false'
  mcp_tool resolve_comment "{\"comment_id\":\"$wid\",\"version\":99}"
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'unknown_version' "$TMP/mcp.body"; then
    fail "resolve_comment with an unknown version ($(cat "$TMP/mcp.body"))"
  fi
  out="$(api POST "/dashboards/$id/comments" -d "$(comment_body 2 "$panel" 'Please check p99 on version 2')")"
  [[ "$out" == 201 ]] || fail "create a comment on version 2 for MCP ($out: $(cat "$TMP/api.body"))"
  vid="$(comment_ids "$TMP/api.body")"
  [[ -n "$vid" ]] || fail "no comment id in the version 2 comment response ($(cat "$TMP/api.body"))"
  mcp_tool resolve_comment "{\"comment_id\":\"$vid\",\"version\":1}"
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'invalid_request' "$TMP/mcp.body" || ! grep -q 'version must be at least 2' "$TMP/mcp.body"; then
    fail "resolve_comment with a version before the thread's ($(cat "$TMP/mcp.body"))"
  fi
  mcp_tool reply_to_comment '{"comment_id":"x","body":"hello"}'
  if [[ "$MCP_ERROR" != true ]] || ! grep -q 'invalid_request' "$TMP/mcp.body"; then
    fail "reply_to_comment with a bad comment_id ($(cat "$TMP/mcp.body"))"
  fi
  mcp_tool list_comments "{\"dashboard_id\":\"$id\"}"
  mcp_expect "list_comments after resolving" "\"id\":\"$wid\""
  if grep -q "\"id\":\"$cid\"" "$TMP/mcp.body"; then
    fail "a resolved thread should not be listed by default ($(cat "$TMP/mcp.body"))"
  fi
  pass "MCP clients list, answer and resolve comments with the version that fixed them"

  leak="$(grep -oE 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' "$TMP/mcp-all.body" || true)"
  [[ -z "$leak" ]] || fail "MCP responses expose connection details ($leak)"
  pass "MCP responses expose no connection details"
}

proxy_call() {
  local out
  out="$(api "$@")"
  cat "$TMP/api.body" >>"$TMP/proxy-bodies.txt"
  printf '%s' "$out"
}

expect_proxy_error() {
  local path="$1" status="$2" code="$3" label="$4" out
  out="$(proxy_call GET "$path")"
  if [[ "$out" != "$status" ]] || ! grep -q "\"code\":\"$code\"" "$TMP/api.body"; then
    fail "$label ($out: $(cat "$TMP/api.body"))"
  fi
}

urlencode() {
  local text="$1" i char out=""
  local LC_ALL=C
  for ((i = 0; i < ${#text}; i++)); do
    char="${text:i:1}"
    case "$char" in
      [a-zA-Z0-9.~_-]) out+="$char" ;;
      *) out+="$(printf '%%%02X' $(($(printf '%d' "'$char") & 255)))" ;;
    esac
  done
  printf '%s' "$out"
}

first_value() {
  grep -o '"values":\["[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4 || true
}

proxy_suite() {
  api_token
  local out id base window stable values_window first_executed service job leak rows_one rows_all now_ms to_ms
  : >"$TMP/proxy-bodies.txt"

  out="$(proxy_call POST /dashboards --data-binary "@$FIXTURES/proxy-dashboard.json")"
  [[ "$out" == 201 ]] || fail "create the query proxy dashboard ($out: $(cat "$TMP/api.body"))"
  id="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  base="/dashboards/$id/versions/1"

  for _ in $(seq 1 45); do
    now_ms=$(($(date +%s) * 1000))
    window="from=$((now_ms - 900000))&to=$now_ms&max_data_points=60"
    out="$(proxy_call GET "$base/panels/p_up/data?$window")"
    if [[ "$out" == 200 ]] && grep -Eq '"type":"time","values":\[[0-9]{13}' "$TMP/api.body"; then break; fi
    sleep 2
  done
  if [[ "$out" != 200 ]] || ! grep -q '"kind":"range"' "$TMP/api.body" || ! grep -q '"step_ms":15000' "$TMP/api.body" \
    || ! grep -q '"cached":false' "$TMP/api.body" || ! grep -Eq '"type":"time","values":\[[0-9]{13}' "$TMP/api.body" \
    || ! grep -q '"ref":"A"' "$TMP/api.body"; then
    fail "panel data for a range panel ($out: $(cat "$TMP/api.body"))"
  fi
  first_executed="$(grep -o '"executed_at":"[^"]*"' "$TMP/api.body")"
  pass "a saved range panel returns frames with epoch millisecond times and an aligned step"

  out="$(proxy_call GET "$base/panels/p_up/data?$window")"
  if [[ "$out" != 200 ]] || ! grep -q '"cached":true' "$TMP/api.body" || ! grep -q "$first_executed" "$TMP/api.body"; then
    fail "the repeated panel data request is served from the cache ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a repeated panel data request is cached and keeps its original execution time"

  out="$(proxy_call GET "$base/panels/p_multi/data?$window")"
  if [[ "$out" != 200 ]] || ! grep -q '"ref":"A"' "$TMP/api.body" || ! grep -q '"ref":"B"' "$TMP/api.body" \
    || grep -q '"ref":"C"' "$TMP/api.body" || ! grep -q '"queries":\[{"ref":"A","kind":"range"' "$TMP/api.body"; then
    fail "a multi-query panel returns frames tagged by query ref ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a multi-query panel returns frames tagged by query ref and skips its hidden query"

  # Instant panels floor their range to 5 s, so an end off that grid lets the variable list a service whose only rows lie past the panel's end.
  for _ in $(seq 1 45); do
    to_ms=$((($(date +%s) * 1000 - 30000) / 5000 * 5000))
    stable="from=$((to_ms - 900000))&to=$to_ms"
    out="$(proxy_call GET "$base/variables/service/values?$stable")"
    service="$(first_value)"
    if [[ "$out" == 200 && -n "$service" ]]; then break; fi
    sleep 2
  done
  if [[ "$out" != 200 || -z "$service" ]] || ! grep -q '"truncated":false' "$TMP/api.body"; then
    fail "values of the service variable ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call GET "$base/panels/p_logs/data?$stable&var-service=$(urlencode "$service")")"
  if [[ "$out" != 200 ]] || ! grep -Eq '"name":"n","type":"number","values":\[[1-9][0-9]*' "$TMP/api.body"; then
    fail "panel data for an explicit service ($out: $(cat "$TMP/api.body"))"
  fi
  pass "a ClickHouse panel runs with an explicitly selected variable"

  out="$(proxy_call GET "$base/panels/p_globals/data?$stable")"
  if [[ "$out" != 200 ]] || ! grep -q 'Query proxy dashboard' "$TMP/api.body" || ! grep -q 'e2e-api' "$TMP/api.body"; then
    fail "global macros expand on the server ($out: $(cat "$TMP/api.body"))"
  fi
  pass "global macros expand on the server, including the dashboard title and the caller"

  out="$(proxy_call GET "$base/panels/p_up/data?query=up")"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"query_not_in_spec"' "$TMP/api.body"; then
    fail "query text on the data endpoint ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call GET "$base/panels/p_up/data?expr=up&sql=select&q=up")"
  [[ "$out" == 403 ]] || fail "other query text parameters on the data endpoint ($out)"
  expect_proxy_error "$base/panels/nope/data" 404 panel_not_found "unknown panel"
  expect_proxy_error "/dashboards/$id/versions/99/panels/p_up/data" 404 not_found "unknown version"
  expect_proxy_error "$base/panels/p_up/data?var-nope=1" 400 invalid_request "undeclared variable"
  expect_proxy_error "$base/panels/p_up/data?max_data_points=5" 400 invalid_request "max_data_points below the minimum"
  expect_proxy_error "$base/panels/p_up/data?from=yesterday" 400 invalid_request "unparseable from"
  expect_proxy_error "$base/panels/p_up/data?from=now-401d" 400 invalid_request "range over 400 days"
  expect_proxy_error "$base/panels/p_up/data?whatever=1" 400 invalid_request "unknown parameter"
  pass "query text is refused and unknown panels, versions, variables and parameters are rejected"

  expect_proxy_error "$base/panels/p_series/data?var-n=50" 422 limit_exceeded "series limit"
  pass "a query over the series limit is refused"

  expect_proxy_error "$base/panels/p_multi_tight/data?var-n=50" 422 limit_exceeded "series limit"
  grep -q '"ref":"B"' "$TMP/api.body" || fail "the failing query is named in the error ($(cat "$TMP/api.body"))"
  pass "a failing query fails its panel request and names its ref"

  expect_proxy_error "$base/panels/p_points/data?from=now-1h&max_data_points=2000" 422 limit_exceeded "points limit"
  pass "a query over the point limit is refused"

  expect_proxy_error "$base/panels/p_sleep/data?var-delay=2" 504 query_timeout "query timeout"
  pass "a query over the timeout is cut off with 504"

  now_ms=$(($(date +%s) * 1000))
  values_window="from=$((now_ms - 900000))&to=$now_ms"
  out="$(proxy_call GET "$base/variables/job/values?$values_window")"
  job="$(first_value)"
  if [[ "$out" != 200 || -z "$job" ]]; then
    fail "values of the job variable ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call GET "$base/variables/job/values?$values_window")"
  if [[ "$out" != 200 ]] || ! grep -q "\"$job\"" "$TMP/api.body" || ! grep -q '"cached":true' "$TMP/api.body"; then
    fail "the job variable values are listed and cached ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call GET "$base/panels/p_up/data?$window&var-job=$(urlencode ".*")")"
  if [[ "$out" != 200 ]] || ! grep -q '"frames":\[\]' "$TMP/api.body"; then
    fail "a regex value in a Prometheus variable matches nothing ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call GET "$base/panels/p_up/data?$window&var-job=$(urlencode "$job")")"
  if [[ "$out" != 200 ]] || ! grep -q '"frames":\[{' "$TMP/api.body"; then
    fail "a real job returns frames ($out: $(cat "$TMP/api.body"))"
  fi
  pass "Prometheus variable values are escaped, so a regex value matches nothing"

  out="$(proxy_call GET "$base/panels/p_logs/data?$stable&var-service=x%27%20OR%201%3D1%20--")"
  if [[ "$out" != 200 ]] || ! grep -Eq '"name":"n","type":"number","values":\[0\]' "$TMP/api.body"; then
    fail "an injection attempt in a ClickHouse variable ($out: $(cat "$TMP/api.body"))"
  fi
  pass "ClickHouse variable values are escaped, so an injection attempt matches nothing"

  out="$(proxy_call GET "$base/variables/n/values")"
  if [[ "$out" != 200 ]] || ! grep -q '"values":\["1","50"\]' "$TMP/api.body" || ! grep -q '"cached":false' "$TMP/api.body"; then
    fail "the custom variable lists its options ($out: $(cat "$TMP/api.body"))"
  fi
  pass "variable values come from the datasource (cached) or from a custom variable's options"

  out="$(proxy_call POST /datasources/prom/lint -d '{"query":"sum(rate("}')"
  if [[ "$out" != 200 ]] || ! grep -q '"severity":"error"' "$TMP/api.body"; then
    fail "lint a broken PromQL query ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call POST /datasources/prom/lint -d '{"query":"up"}')"
  if [[ "$out" != 200 ]] || ! grep -q '"diagnostics":\[\]' "$TMP/api.body"; then
    fail "lint a valid PromQL query ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(proxy_call POST /datasources/nope/lint -d '{"query":"up"}')"
  [[ "$out" == 404 ]] || fail "lint on an unknown datasource ($out)"
  out="$(proxy_call POST /datasources/prom/lint -d '{"query":""}')"
  [[ "$out" == 400 ]] || fail "lint with an empty query ($out)"
  pass "lint returns diagnostics without data and rejects unknown datasources and empty queries"

  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1$base/panels/p_up/data")"
  [[ "$out" == 401 ]] || fail "panel data without credentials ($out)"
  pass "panel data requires authentication"

  out="$(proxy_call GET "$base/panels/p_all/data?$stable")"
  if [[ "$out" != 200 ]] || ! grep -Eq '"name":"n","type":"number","values":\[[1-9][0-9]*' "$TMP/api.body"; then
    fail "an All variable defaults to every value ($out: $(cat "$TMP/api.body"))"
  fi
  rows_all="$(grep -Eo '"name":"n","type":"number","values":\[[0-9]+' "$TMP/api.body")"
  out="$(proxy_call GET "$base/panels/p_all/data?$stable&var-services=%24__all")"
  rows_one="$(grep -Eo '"name":"n","type":"number","values":\[[0-9]+' "$TMP/api.body")"
  if [[ "$out" != 200 || "$rows_one" != "$rows_all" ]]; then
    fail "an explicit All selection matches the default ($out: $rows_one vs $rows_all)"
  fi
  pass "All expands to every value, by default and when selected explicitly"

  expect_proxy_error "$base/panels/p_series/data?var-n=7" 400 invalid_request "custom value outside its options"
  expect_proxy_error "$base/panels/p_series/data?var-n=1&var-n=50" 400 invalid_request "several values for a single-value variable"
  expect_proxy_error "$base/panels/p_all/data?var-services=%24__all&var-services=x" 400 invalid_request "All combined with a value"
  pass "variable selections that break the variable's rules are rejected"

  expect_proxy_error "$base/variables/job/values?var-n=1" 400 invalid_request "selection for a later variable"
  pass "a selection for a variable declared after the listed one is rejected"

  leak="$(grep -oE 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' "$TMP/proxy-bodies.txt" || true)"
  [[ -z "$leak" ]] || fail "query proxy responses expose connection details ($leak)"
  pass "query proxy responses expose no connection details"
}

cli_run() {
  CLI_CODE=0
  env -u OBSERVE_SERVER -u OBSERVE_TOKEN XDG_CONFIG_HOME="$TMP/xdg" "$TMP/observe" "$@" <"${CLI_STDIN:-/dev/null}" >"$TMP/cli.out" 2>"$TMP/cli.err" || CLI_CODE=$?
}

cli_expect() {
  local what="$1" code="$2" pattern="$3" stream="${4:-err}"
  [[ "$CLI_CODE" == "$code" ]] || fail "$what should exit $code, got $CLI_CODE (stdout: $(cat "$TMP/cli.out"); stderr: $(cat "$TMP/cli.err"))"
  grep -qF -- "$pattern" "$TMP/cli.$stream" || fail "$what lacks $pattern on std$stream (stdout: $(cat "$TMP/cli.out"); stderr: $(cat "$TMP/cli.err"))"
}

strip_final_newline() {
  perl -0pe 's/\n\z//' "$1"
}

cli_comments_checks() {
  local did="$1" out cid nil_id=00000000-0000-0000-0000-000000000000

  out="$(api POST "/dashboards/$did/comments" -d '{"version":1,"body":"CLI thread"}')"
  [[ "$out" == 201 ]] || fail "create a comment for the CLI ($out: $(cat "$TMP/api.body"))"
  cid="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  [[ -n "$cid" ]] || fail "the new comment has no id ($(cat "$TMP/api.body"))"

  cli_run comments list "$did" --json
  [[ "$CLI_CODE" == 0 ]] || fail "observe comments list --json exits $CLI_CODE ($(cat "$TMP/cli.err"))"
  python3 -m json.tool <"$TMP/cli.out" >/dev/null || fail "observe comments list --json is not JSON"
  grep -qF "$cid" "$TMP/cli.out" || fail "observe comments list --json lacks the comment $cid"

  cli_run comments list "$did"
  cli_expect "observe comments list" 0 "$cid" out
  grep -F "$cid" "$TMP/cli.out" | grep -qw dashboard || fail "observe comments list should show a whole-dashboard comment as dashboard ($(cat "$TMP/cli.out"))"

  cli_run comments reply "$did" "$cid" Looks good now
  cli_expect "observe comments reply" 0 "Replied to thread $cid" out
  out="$(api GET "/dashboards/$did/comments")"
  if [[ "$out" != 200 ]] || ! grep -q "\"parent_id\":\"$cid\"" "$TMP/api.body" || ! grep -q '"body":"Looks good now"' "$TMP/api.body"; then
    fail "the reply from observe comments reply is missing ($out: $(cat "$TMP/api.body"))"
  fi

  cli_run comments resolve "$did" "$cid" --version 1
  cli_expect "observe comments resolve" 0 "Resolved thread $cid by version 1" out
  out="$(api GET "/dashboards/$did/comments")"
  if [[ "$out" != 200 ]] || ! grep -q '"resolved_by_version":1' "$TMP/api.body"; then
    fail "the comment resolved by the CLI lacks resolved_by_version 1 ($out: $(cat "$TMP/api.body"))"
  fi

  cli_run comments list "$did"
  [[ "$CLI_CODE" == 0 ]] || fail "observe comments list after resolve exits $CLI_CODE ($(cat "$TMP/cli.err"))"
  if grep -qF "$cid" "$TMP/cli.out"; then
    fail "observe comments list should hide a resolved thread by default ($(cat "$TMP/cli.out"))"
  fi

  cli_run comments resolve "$did" not-an-id
  [[ "$CLI_CODE" == 2 ]] || fail "observe comments resolve with a bad id should exit 2, got $CLI_CODE"
  cli_run comments resolve "$did" "$cid" --version 0
  [[ "$CLI_CODE" == 2 ]] || fail "observe comments resolve --version 0 should exit 2, got $CLI_CODE"
  cli_run comments resolve "$did" "$nil_id"
  cli_expect "observe comments resolve for an unknown comment" 1 "not_found"
  pass "observe comments lists, answers and resolves comments, with --json"
}

cli_saved_key_stays_home() {
  local sink_pid sink_port="" tries
  : >"$TMP/sink.log"
  python3 - "$TMP/sink.port" "$TMP/sink.log" <<'PY' &
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

port_file, log_file = sys.argv[1], sys.argv[2]


class Sink(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        with open(log_file, "a") as log:
            log.write("%s %s\n" % (self.path, self.headers.get("Authorization", "")))
        body = b'{"error":"unauthorized","code":"unauthorized"}'
        self.send_response(401)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


server = HTTPServer(("127.0.0.1", 0), Sink)
with open(port_file, "w") as handle:
    handle.write(str(server.server_address[1]))
server.serve_forever()
PY
  sink_pid=$!
  for tries in $(seq 1 50); do
    sink_port="$(cat "$TMP/sink.port" 2>/dev/null || true)"
    [[ -n "$sink_port" ]] && break
    sleep 0.1
  done
  if [[ -z "$sink_port" ]]; then
    kill "$sink_pid" 2>/dev/null || true
    fail "the key sink did not start"
  fi

  cli_run --server "http://127.0.0.1:$sink_port" dashboards list
  cli_expect "observe with the saved key and another server" 1 "the saved key belongs to"
  if [[ -s "$TMP/sink.log" ]]; then
    kill "$sink_pid" 2>/dev/null || true
    fail "observe contacted another server with the saved key ($(cat "$TMP/sink.log"))"
  fi

  cli_run --server "http://127.0.0.1:$sink_port" --token sok_probe dashboards list
  kill "$sink_pid" 2>/dev/null || true
  wait "$sink_pid" 2>/dev/null || true
  grep -qF "Bearer sok_probe" "$TMP/sink.log" || fail "an explicit --token should reach the other server (sink saw: $(cat "$TMP/sink.log"))"
  if grep -qF "$API_TOKEN" "$TMP/sink.log"; then
    fail "the saved key reached another server"
  fi
}

cli_suite() {
  api_token
  local config="$TMP/xdg/slashobserve/cli.json" url did title perms request_id

  (cd "$ROOT" && go build -o "$TMP/observe" ./cmd/observe) || fail "build the observe CLI"

  printf '%s\n' "$API_TOKEN" >"$TMP/cli.key"
  CLI_STDIN="$TMP/cli.key" cli_run login --server "$BASE_URL" --with-token
  cli_expect "observe login" 0 "Signed in" out
  perms="$(stat -c %a "$config" 2>/dev/null || stat -f %Lp "$config")"
  [[ "$perms" == 600 ]] || fail "the CLI config file mode is $perms, expected 600"
  perms="$(stat -c %a "$(dirname "$config")" 2>/dev/null || stat -f %Lp "$(dirname "$config")")"
  [[ "$perms" == 700 ]] || fail "the CLI config directory mode is $perms, expected 700"
  if grep -qF "$API_TOKEN" "$TMP/cli.out" "$TMP/cli.err"; then
    fail "observe login printed the key"
  fi
  pass "observe login validates the key, saves it with mode 600 and does not print it"

  cli_run --timeout 120s "log volume by service"
  url="$(cat "$TMP/cli.out")"
  if [[ "$CLI_CODE" != 0 || "$(wc -l <"$TMP/cli.out" | tr -d ' ')" != 1 || ! "$url" =~ ^http://localhost:${PORT}/dashboards/[0-9a-f-]{36}$ ]]; then
    fail "observe with a prompt should print one dashboard address ($CLI_CODE; stdout: $url; stderr: $(cat "$TMP/cli.err"))"
  fi
  grep -qF "save" "$TMP/cli.err" || fail "observe with a prompt printed no progress steps (stderr: $(cat "$TMP/cli.err"))"
  did="${url##*/}"
  pass "observe with a prompt prints only the dashboard address and reports progress on stderr"

  cli_comments_checks "$did"

  command curl -sS --max-time 10 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/dashboards/$did/versions/1" >"$TMP/cli.api"
  grep -q '"origin":"cli"' "$TMP/cli.api" || fail "the version the agent saved for a CLI prompt should record the cli origin ($(cat "$TMP/cli.api"))"
  request_id="$(grep -o '"agent_request_id":"[^"]*"' "$TMP/cli.api" | head -1 | cut -d'"' -f4)"
  [[ -n "$request_id" ]] || fail "the version the agent saved has no agent_request_id ($(cat "$TMP/cli.api"))"
  command curl -sS --max-time 10 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/observe/$request_id" >"$TMP/cli.api"
  grep -q '"origin":"cli"' "$TMP/cli.api" || fail "a CLI prompt should record the cli origin on its request ($(cat "$TMP/cli.api"))"
  pass "a CLI prompt records the cli origin on the request and on the version the agent saved"

  cli_run dashboards list --json
  [[ "$CLI_CODE" == 0 ]] || fail "observe dashboards list --json exits $CLI_CODE ($(cat "$TMP/cli.err"))"
  python3 -m json.tool <"$TMP/cli.out" >/dev/null || fail "observe dashboards list --json is not JSON"
  grep -qF "$did" "$TMP/cli.out" || fail "observe dashboards list --json lacks the new dashboard $did"
  pass "observe dashboards list --json parses and lists the new dashboard"

  cli_run dashboards show "$did" --json
  [[ "$CLI_CODE" == 0 ]] || fail "observe dashboards show --json exits $CLI_CODE ($(cat "$TMP/cli.err"))"
  command curl -sS --max-time 10 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/dashboards/$did" >"$TMP/cli.api"
  diff <(strip_final_newline "$TMP/cli.out") <(strip_final_newline "$TMP/cli.api") >/dev/null \
    || fail "observe dashboards show --json differs from the API response"
  pass "observe dashboards show --json matches the API response byte for byte"

  title="$(grep -o '"title":"[^"]*"' "$TMP/cli.api" | head -1 | cut -d'"' -f4)"
  [[ -n "$title" ]] || fail "the dashboard has no title"
  cli_run dashboards show "$did"
  cli_expect "observe dashboards show" 0 "$title" out
  cli_run dashboards versions "$did"
  if [[ "$CLI_CODE" != 0 ]] || ! grep -Eq '^1[[:space:]]' "$TMP/cli.out"; then
    fail "observe dashboards versions should list version 1 ($CLI_CODE: $(cat "$TMP/cli.out"))"
  fi
  pass "observe dashboards show prints the title and versions lists version 1"

  cli_run --no-wait "log volume by service"
  request_id="$(cat "$TMP/cli.out")"
  if [[ "$CLI_CODE" != 0 || ! "$request_id" =~ ^[0-9a-f-]{36}$ ]]; then
    fail "observe --no-wait should print the request id ($CLI_CODE: $request_id; $(cat "$TMP/cli.err"))"
  fi
  cli_run status "$request_id" --wait --timeout 120s
  url="$(cat "$TMP/cli.out")"
  if [[ "$CLI_CODE" != 0 || ! "$url" =~ ^http://localhost:${PORT}/dashboards/[0-9a-f-]{36}$ ]]; then
    fail "observe status --wait should print the dashboard address ($CLI_CODE: $url; $(cat "$TMP/cli.err"))"
  fi
  cli_run status "$request_id"
  cli_expect "observe status" 0 "succeeded" out
  pass "observe --no-wait prints a request id that status --wait and status resolve"

  cli_run --timeout 120s "e2e: a slow run that never saves"
  cli_expect "a run that never saves" 1 "without saving"
  pass "observe reports a failed agent request and exits 1"

  cli_run
  [[ "$CLI_CODE" == 2 ]] || fail "observe with no arguments should exit 2, got $CLI_CODE"
  cli_run --bogus
  [[ "$CLI_CODE" == 2 ]] || fail "observe with an unknown flag should exit 2, got $CLI_CODE"
  cli_run login --token sok_ignored
  cli_expect "observe login --token" 2 "--with-token"
  pass "observe usage errors exit 2"

  cli_run --server http://example.com dashboards list
  cli_expect "observe over plain http to another host" 1 "https"
  pass "observe refuses plain http except to loopback addresses"

  cli_saved_key_stays_home
  pass "observe sends the saved key only to the server it was saved for"

  cli_run logout
  cli_expect "observe logout" 0 "Signed out" out
  if ! grep -q '"server"' "$config" || grep -q '"token"' "$config"; then
    fail "observe logout should keep the server and remove the key ($(cat "$config"))"
  fi
  cli_run dashboards list
  cli_expect "observe after logout" 1 "observe login"
  pass "observe logout removes the key and later commands ask to sign in"

  CLI_CODE=0
  env -u OBSERVE_SERVER XDG_CONFIG_HOME="$TMP/xdg" OBSERVE_TOKEN=sok_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    "$TMP/observe" --server "$BASE_URL" dashboards list </dev/null >"$TMP/cli.out" 2>"$TMP/cli.err" || CLI_CODE=$?
  cli_expect "observe with an invalid key" 1 "not signed in, or the key is invalid; run observe login"
  pass "observe reports an invalid key from the environment"
}

evals_smoke_suite() {
  api_token
  local rc=0 flat

  OBSERVE_SERVER="$BASE_URL" OBSERVE_TOKEN="$API_TOKEN" \
    bounded 600 go -C "$ROOT" run ./evals/runner --tag replay-smoke --min-pass 1 --out "$TMP/evals.json" --markdown "$TMP/evals.md" \
    >"$TMP/evals.out" 2>"$TMP/evals.err" || rc=$?
  [[ "$rc" == 0 ]] || fail "the eval runner exited $rc ($(cat "$TMP/evals.err") $(cat "$TMP/evals.out"))"
  flat="$(tr -d ' \n' <"$TMP/evals.json")"
  [[ "$flat" == *'"scenarios":{"passed":2,"total":2,"rate":1}'* ]] || fail "the eval report should show 2 of 2 scenarios passed ($(cat "$TMP/evals.out"))"
  [[ "$flat" != *'"passed":false'* ]] || fail "the eval report has a failed check ($(cat "$TMP/evals.out"))"
  [[ "$flat" == *'"model":"replay"'* ]] || fail "the eval report does not name the replay model"
  grep -q '^| red-metrics-by-endpoint |' "$TMP/evals.md" || fail "the eval Markdown report has no row for red-metrics-by-endpoint"
  pass "eval runner scores replay scenarios"
}

people_admin_field() {
  python3 -c '
import json, sys
people = json.load(open(sys.argv[1]))["users"]
admin = [p for p in people if p["email"] == "admin@example.com"]
if len(admin) != 1:
    sys.exit("expected one admin entry, found %d" % len(admin))
print(admin[0][sys.argv[2]])
' "$1" "$2"
}

dashboard_item_of() {
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps(next(x for x in d["dashboards"] if x["id"]==sys.argv[2])))' "$1" "$2"
}

first_json_string() {
  { grep -o "\"$2\":\"[^\"]*\"" "$1" || true; } | head -1 | cut -d'"' -f4
}

dashboard_list_suite() {
  api_token
  local out d_api d_web d_mcp root resolved_root admin_id item cursor bad want long_q

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the api dashboard for the list ($out: $(cat "$TMP/api.body"))"
  d_api="$(first_json_string "$TMP/api.body" id)"
  out="$(command curl -sS -o "$TMP/list-web.json" -w '%{http_code}' -b "$TMP/api-jar" -H "Origin: $BASE_URL" -H 'Content-Type: application/json' -X POST --data-binary "@$FIXTURES/dashboard-valid.json" "$BASE_URL/api/v1/dashboards")"
  [[ "$out" == 201 ]] || fail "create the web dashboard for the list ($out: $(cat "$TMP/list-web.json"))"
  d_web="$(first_json_string "$TMP/list-web.json" id)"
  out="$(mcp_call 21 tools/call "{\"name\":\"create_dashboard\",\"arguments\":$(cat "$FIXTURES/dashboard-valid.json"),\"_meta\":{\"io.modelcontextprotocol/clientInfo\":{\"name\":\"claude-code\",\"version\":\"9\"}}}" "$TMP/mcp.body")"
  if [[ "$out" != 200 ]] || grep -q '"isError":true' "$TMP/mcp.body"; then
    fail "create the MCP dashboard for the list ($out: $(head -c 1500 "$TMP/mcp.body"))"
  fi
  d_mcp="$(first_json_string "$TMP/mcp.body" dashboard_id)"
  [[ -n "$d_api" && -n "$d_web" && -n "$d_mcp" ]] || fail "the list dashboards have no ids (api '$d_api', web '$d_web', mcp '$d_mcp')"

  out="$(api POST "/dashboards/$d_api/comments" -d '{"version":1,"body":"root"}')"
  [[ "$out" == 201 ]] || fail "open a thread on the api dashboard ($out: $(cat "$TMP/api.body"))"
  root="$(first_json_string "$TMP/api.body" id)"
  [[ -n "$root" ]] || fail "the opened thread has no id ($(head -c 500 "$TMP/api.body"))"
  out="$(api POST "/dashboards/$d_api/comments" -d '{"parent_id":"'"$root"'","body":"reply"}')"
  [[ "$out" == 201 ]] || fail "reply in the thread ($out: $(cat "$TMP/api.body"))"
  out="$(api POST "/dashboards/$d_api/comments" -d '{"version":1,"body":"resolved root"}')"
  [[ "$out" == 201 ]] || fail "open a second thread on the api dashboard ($out: $(cat "$TMP/api.body"))"
  resolved_root="$(first_json_string "$TMP/api.body" id)"
  [[ -n "$resolved_root" ]] || fail "the second thread has no id ($(head -c 500 "$TMP/api.body"))"
  out="$(api PATCH "/dashboards/$d_api/comments/$resolved_root" -d '{"status":"resolved"}')"
  [[ "$out" == 200 ]] || fail "resolve the second thread ($out: $(cat "$TMP/api.body"))"

  out="$(api GET "/dashboards?made_by=me&limit=200")"
  [[ "$out" == 200 ]] || fail "list dashboards made by me ($out: $(cat "$TMP/api.body"))"
  grep -q "\"id\":\"$d_api\"" "$TMP/api.body" || fail "made_by=me should list the api dashboard ($(head -c 1500 "$TMP/api.body"))"
  ! grep -q "\"id\":\"$d_web\"" "$TMP/api.body" || fail "made_by=me should not list a dashboard a person made"
  item="$(dashboard_item_of "$TMP/api.body" "$d_api")"
  for want in '"datasource_uids": ["ch", "prom"]' '"open_comment_count": 1' '"via": "direct"' '"origin": "api"'; do
    grep -qF -- "$want" <<<"$item" || fail "the api dashboard's list item should contain $want ($item)"
  done
  python3 -c 'import json,sys; n=json.load(open(sys.argv[1]))["total_count"]; sys.exit(0 if isinstance(n, int) and n >= 2 else 1)' "$TMP/api.body" \
    || fail "total_count should be a number of at least 2 ($(head -c 300 "$TMP/api.body"))"

  out="$(command curl -sS -o "$TMP/list-me.json" -w '%{http_code}' -b "$TMP/api-jar" "$BASE_URL/api/v1/me")"
  [[ "$out" == 200 ]] || fail "/api/v1/me for the list ($out: $(cat "$TMP/list-me.json"))"
  admin_id="$(first_json_string "$TMP/list-me.json" id)"
  [[ -n "$admin_id" ]] || fail "/api/v1/me has no id ($(head -c 500 "$TMP/list-me.json"))"
  out="$(api GET "/dashboards?made_by=user:$admin_id&limit=200")"
  [[ "$out" == 200 ]] || fail "list dashboards made by the admin user ($out: $(cat "$TMP/api.body"))"
  grep -q "\"id\":\"$d_web\"" "$TMP/api.body" || fail "made_by=user:<id> should list the web dashboard"
  ! grep -q "\"id\":\"$d_api\"" "$TMP/api.body" || fail "made_by=user:<id> should not list the api dashboard"

  out="$(api GET "/dashboards?made_by=mcp:claude-code&limit=200")"
  [[ "$out" == 200 ]] || fail "list dashboards made by claude-code ($out: $(cat "$TMP/api.body"))"
  grep -q "\"id\":\"$d_mcp\"" "$TMP/api.body" || fail "made_by=mcp:claude-code should list the MCP dashboard"
  ! grep -q "\"id\":\"$d_api\"" "$TMP/api.body" || fail "made_by=mcp:claude-code should not list the api dashboard"
  out="$(api GET "/dashboards?made_by=mcp&limit=200")"
  [[ "$out" == 200 ]] && grep -q "\"id\":\"$d_mcp\"" "$TMP/api.body" || fail "made_by=mcp should list the MCP dashboard ($out)"
  out="$(api GET "/dashboards/$d_mcp")"
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1]))["dashboard"]["creator"]; sys.exit(0 if (c["origin"], c["origin_client"], c["via"]) == ("mcp", "claude-code", "direct") else 1)' "$TMP/api.body" \
    || fail "the MCP dashboard's creator should carry the origin and client ($out: $(head -c 1500 "$TMP/api.body"))"

  out="$(api GET "/dashboards?made_by=agent&limit=200")"
  [[ "$out" == 200 ]] || fail "list dashboards made by the agent ($out: $(cat "$TMP/api.body"))"
  grep -qF '"title":"Demo shop RED metrics"' "$TMP/api.body" || fail "made_by=agent should list the observe dashboard ($(head -c 1500 "$TMP/api.body"))"
  ! grep -qF '"via":"direct"' "$TMP/api.body" || fail "made_by=agent should list no direct dashboard"

  out="$(api GET "/dashboards?datasource=ch&limit=200")"
  [[ "$out" == 200 ]] && grep -q "\"id\":\"$d_api\"" "$TMP/api.body" || fail "datasource=ch should list the api dashboard ($out: $(head -c 1500 "$TMP/api.body"))"
  out="$(api GET "/dashboards?datasource=nope-uid")"
  if [[ "$out" != 200 ]] || ! grep -qF '"dashboards":[]' "$TMP/api.body" || ! grep -qF '"total_count":0' "$TMP/api.body"; then
    fail "an unused datasource should match nothing ($out: $(cat "$TMP/api.body"))"
  fi

  out="$(api GET "/dashboards?q=demo%20SHOP%20red&limit=200")"
  [[ "$out" == 200 ]] && grep -qF '"title":"Demo shop RED metrics"' "$TMP/api.body" || fail "q should match a title case-insensitively ($out: $(head -c 1500 "$TMP/api.body"))"
  out="$(api GET "/dashboards?q=zzzz-no-match")"
  if [[ "$out" != 200 ]] || ! grep -qF '"total_count":0' "$TMP/api.body"; then
    fail "q should match nothing ($out: $(cat "$TMP/api.body"))"
  fi

  out="$(api GET "/dashboards?made_by=me&limit=1")"
  python3 -c 'import json,sys; sys.exit(0 if isinstance(json.load(open(sys.argv[1]))["next_cursor"], str) else 1)' "$TMP/api.body" \
    || fail "a page of one should have a next_cursor ($out: $(cat "$TMP/api.body"))"
  cursor="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["next_cursor"])' "$TMP/api.body")"
  out="$(api GET "/dashboards?made_by=me&limit=1&cursor=$cursor")"
  [[ "$out" == 200 ]] || fail "the second page of the list ($out: $(cat "$TMP/api.body"))"

  long_q="%20$(printf 'a%.0s' {1..200})"
  for bad in 'made_by=bogus' 'made_by=user:x' 'made_by=mcp:' 'made_by=' 'datasource=' 'q=' 'q=%20' 'q=%FF' 'datasource=%00' "q=$long_q"; do
    out="$(api GET "/dashboards?$bad")"
    if [[ "$out" != 400 ]] || ! grep -q '"code":"invalid_request"' "$TMP/api.body"; then
      fail "list dashboards with $bad should be a 400 ($out: $(cat "$TMP/api.body"))"
    fi
  done
  pass "the dashboard list reports creator, datasources and open threads, and filters by maker, datasource and text"

  out="$(api GET "/dashboards?datasource=prom&limit=1")"
  [[ "$out" == 200 ]] || fail "list the dashboards that use prom ($out: $(cat "$TMP/api.body"))"
  want="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["total_count"])' "$TMP/api.body")"
  [[ -n "$want" ]] || fail "the prom dashboard list has no total_count ($(head -c 300 "$TMP/api.body"))"
  out="$(api GET /datasources)"
  [[ "$out" == 200 ]] || fail "list datasources for their usage ($out: $(cat "$TMP/api.body"))"
  python3 -c 'import json,sys; e={d["uid"]: d["dashboard_count"] for d in json.load(open(sys.argv[1]))["datasources"]}; n=int(sys.argv[2]); sys.exit(0 if n >= 1 and e.get("prom") == n else 1)' "$TMP/api.body" "$want" \
    || fail "the dashboard_count of prom should equal the prom list's total_count $want and be at least 1 ($(head -c 1500 "$TMP/api.body"))"
  pass "datasources report how many dashboards use them, matching the dashboard list filter"
}

people_suite() {
  api_token
  local out admin_id sessions keys stream_dashboard stream_pid tries nil_id=00000000-0000-0000-0000-000000000000

  out="$(curl -sS -o "$TMP/people.body" -w '%{http_code}' "$BASE_URL/api/v1/users")"
  [[ "$out" == 401 ]] || fail "list people without credentials ($out: $(cat "$TMP/people.body"))"
  out="$(api GET /users)"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"forbidden"' "$TMP/api.body"; then
    fail "a service account lists people ($out: $(cat "$TMP/api.body"))"
  fi
  pass "listing people needs a signed-in user, not a key"

  out="$(curl -sS -b "$TMP/api-jar" -o "$TMP/people.body" -w '%{http_code}' "$BASE_URL/api/v1/users")"
  if [[ "$out" != 200 ]] || ! grep -q '"email":"admin@example.com"' "$TMP/people.body" || ! grep -q '"last_seen_at":"' "$TMP/people.body"; then
    fail "list people ($out: $(cat "$TMP/people.body"))"
  fi
  sessions="$(people_admin_field "$TMP/people.body" session_count)"
  keys="$(people_admin_field "$TMP/people.body" key_count)"
  [[ "$sessions" -ge 1 && "$keys" -ge 1 ]] || fail "the admin entry should count its sessions and keys (sessions $sessions, keys $keys)"
  pass "people are listed with activity, sessions and keys"

  out="$(curl -sS -b "$TMP/api-jar" -o "$TMP/people.body" -w '%{http_code}' "$BASE_URL/api/v1/users?limit=0")"
  [[ "$out" == 400 ]] || fail "list people with limit 0 ($out: $(cat "$TMP/people.body"))"
  out="$(curl -sS -b "$TMP/api-jar" -o "$TMP/people.body" -w '%{http_code}' "$BASE_URL/api/v1/users?cursor=%40%40")"
  [[ "$out" == 400 ]] || fail "list people with a bad cursor ($out: $(cat "$TMP/people.body"))"
  pass "a bad limit or cursor is rejected"

  dex_login "$TMP/people-jar" / "$BASE_URL/"
  out="$(curl -sS -b "$TMP/people-jar" -o "$TMP/people.me" -w '%{http_code}' "$BASE_URL/api/v1/me")"
  [[ "$out" == 200 ]] || fail "/api/v1/me for the second admin session ($out: $(cat "$TMP/people.me"))"
  admin_id="$(grep -o '"id":"[^"]*"' "$TMP/people.me" | head -1 | cut -d'"' -f4)"
  [[ -n "$admin_id" ]] || fail "/api/v1/me has no id ($(cat "$TMP/people.me"))"

  out="$(curl -sS -b "$TMP/people-jar" -o "$TMP/people.body" -w '%{http_code}' -X DELETE -H 'Origin: http://evil.example' "$BASE_URL/api/v1/users/$admin_id/sessions")"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"csrf_rejected"' "$TMP/people.body"; then
    fail "a cross-origin sign out ($out: $(cat "$TMP/people.body"))"
  fi
  out="$(api DELETE "/users/$admin_id/sessions")"
  if [[ "$out" != 403 ]] || ! grep -q '"code":"forbidden"' "$TMP/api.body"; then
    fail "a service account signs a person out ($out: $(cat "$TMP/api.body"))"
  fi
  out="$(curl -sS -b "$TMP/people-jar" -o "$TMP/people.body" -w '%{http_code}' -X DELETE -H "Origin: $BASE_URL" "$BASE_URL/api/v1/users/$nil_id/sessions")"
  if [[ "$out" != 404 ]] || ! grep -q '"code":"not_found"' "$TMP/people.body"; then
    fail "sign out an unknown person ($out: $(cat "$TMP/people.body"))"
  fi
  out="$(curl -sS -b "$TMP/people-jar" -o "$TMP/people.body" -w '%{http_code}' -X DELETE -H "Origin: $BASE_URL" "$BASE_URL/api/v1/users/not-a-uuid/sessions")"
  [[ "$out" == 400 ]] || fail "sign out with a bad id ($out: $(cat "$TMP/people.body"))"
  pass "signing out is refused cross-origin and for keys, and unknown or malformed ids are rejected"

  out="$(api POST /dashboards --data-binary "@$FIXTURES/dashboard-valid.json")"
  [[ "$out" == 201 ]] || fail "create the dashboard for the open stream ($out: $(cat "$TMP/api.body"))"
  stream_dashboard="$(grep -o '"id":"[^"]*"' "$TMP/api.body" | head -1 | cut -d'"' -f4)"
  : >"$TMP/people.sse"
  command curl -sS -N --max-time 90 -b "$TMP/people-jar" "$BASE_URL/api/v1/dashboards/$stream_dashboard/events" >"$TMP/people.sse" 9>&- &
  stream_pid=$!
  disown "$stream_pid"
  tries=50
  until grep -q '^event: ready$' "$TMP/people.sse"; do
    ((--tries > 0)) || { kill "$stream_pid" 2>/dev/null || true; fail "the session stream sent no ready event ($(cat "$TMP/people.sse"))"; }
    sleep 0.2
  done

  out="$(curl -sS -b "$TMP/people-jar" -o "$TMP/people.body" -D "$TMP/people.headers" -w '%{http_code}' -X DELETE -H "Origin: $BASE_URL" "$BASE_URL/api/v1/users/$admin_id/sessions")"
  [[ "$out" == 204 ]] || fail "sign a person out everywhere ($out: $(cat "$TMP/people.body"))"
  if ! grep -i '^set-cookie: so_session=;' "$TMP/people.headers" | grep -qi 'max-age=0'; then
    kill "$stream_pid" 2>/dev/null || true
    fail "signing yourself out everywhere did not expire the session cookie ($(cat "$TMP/people.headers"))"
  fi
  tries=125
  while kill -0 "$stream_pid" 2>/dev/null; do
    ((--tries > 0)) || { kill "$stream_pid" 2>/dev/null || true; fail "an open event stream survived the sign out for 25 seconds"; }
    sleep 0.2
  done
  out="$(curl -sS -b "$TMP/people-jar" -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "the calling session survived its own sign out ($out)"
  out="$(curl -sS -b "$TMP/api-jar" -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/me")"
  [[ "$out" == 401 ]] || fail "another session of the same person survived the sign out ($out)"

  dex_login "$TMP/people-jar2" / "$BASE_URL/"
  out="$(curl -sS -b "$TMP/people-jar2" -o "$TMP/people.body" -w '%{http_code}' "$BASE_URL/api/v1/users")"
  [[ "$out" == 200 ]] || fail "list people after signing in again ($out: $(cat "$TMP/people.body"))"
  sessions="$(people_admin_field "$TMP/people.body" session_count)"
  [[ "$sessions" == 1 ]] || fail "the admin should have one session after signing in again (found $sessions)"
  pass "people are listed with activity, and anyone can sign a person out everywhere"
}

stack_suite() {
  local out ro_out

  prom_is 'count(count by (pod) (http_requests_total{service="checkout"}))' 2 || fail "checkout metrics should carry 2 pods (got: $LAST)"
  pass "shop metrics vary by pod"
  prom_is 'count(count by (service) (http_requests_total))' 3 || fail "http_requests_total should carry 3 services (got: $LAST)"
  pass "shop metrics vary by service"
  prom_positive 'count(http_request_duration_seconds_bucket{service="checkout"})' || fail "checkout latency histogram missing (got: $LAST)"
  pass "latency histogram exported"
  prom_is 'count(count by (pod) (container_memory_working_set_bytes{pod=~"frontend-0|checkout-0|checkout-1|payments-0"}))' 4 \
    || fail "cAdvisor should label the 4 shop containers by pod (got: $LAST)"
  pass "cAdvisor labels containers by pod"
  prom_positive 'count(container_cpu_usage_seconds_total{service="checkout"})' || fail "cAdvisor CPU series should carry the service label (got: $LAST)"
  pass "cAdvisor labels containers by service"
  prom_is 'count(count by (compose_project) (container_memory_working_set_bytes))' 1 || fail "cAdvisor should see exactly one compose project (got: $LAST)"
  pass "cAdvisor metrics limited to this stack"
  prom_positive 'count(machine_cpu_cores) and count(machine_memory_bytes)' || fail "cAdvisor machine metrics missing (got: $LAST)"
  pass "cAdvisor machine metrics ingested"
  { prom_positive 'count(node_filesystem_avail_bytes{instance="demo-node"})' \
    && prom_positive 'count(node_filesystem_files_free{instance="demo-node"})'; } \
    || fail "node disk and inode metrics missing for demo-node (last: $LAST)"
  pass "node disk and inode metrics present"
  prom_positive 'sum(rate(http_requests_total{service="checkout"}[5m] offset 1d))' || fail "no checkout request rate one day ago (got: $LAST)"
  pass "yesterday's request rate is backfilled"
  prom_positive 'histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{service="checkout"}[5m] offset 1d)))' || fail "no checkout p95 latency one day ago (got: $LAST)"
  pass "yesterday's latency histogram is backfilled"
  poll 240 "backfilled request rate and p95 latency continue into live data" backfill_joins_live
  pass "backfill joins live data without a level jump"

  ch_is "SELECT getSetting('readonly')" 2 \
    || fail "ClickHouse readonly setting is not 2 (got: '$LAST', client: $LAST_ERR)"
  if capture_merged ro_out ch "SELECT 1 SETTINGS max_result_rows = 0"; then
    fail "ClickHouse user lifted max_result_rows"
  fi
  [[ "$ro_out" == *SETTING_CONSTRAINT_VIOLATION* || "$ro_out" == *"Code: 452"* ]] \
    || fail "ClickHouse max_result_rows refusal is not a constraint violation ($ro_out)"
  if capture_merged ro_out ch "INSERT INTO otel.otel_logs (Body) VALUES ('x')"; then
    fail "ClickHouse user inserted a row"
  fi
  [[ "$ro_out" == *READONLY* || "$ro_out" == *"Code: 164"* || "$ro_out" == *ACCESS_DENIED* || "$ro_out" == *"Code: 497"* ]] || fail "ClickHouse insert refusal is not READONLY ($ro_out)"
  pass "ClickHouse user is read-only"
  poll 60 "traces span frontend, checkout and payments" ch_positive \
    "SELECT count() FROM (SELECT TraceId FROM otel.otel_traces GROUP BY TraceId HAVING uniqExact(ServiceName) = 3)"
  pass "traces span frontend, checkout and payments"
  poll 60 "checkout logs carry 2 pods" ch_is \
    "SELECT uniqExact(ResourceAttributes['k8s.pod.name']) FROM otel.otel_logs WHERE ServiceName = 'checkout'" 2
  pass "logs vary by pod"
  poll 60 "checkout error logs present" ch_positive \
    "SELECT count() FROM otel.otel_logs WHERE ServiceName = 'checkout' AND SeverityText = 'ERROR'"
  pass "checkout error logs present"
  poll 60 "logs carry trace ids" ch_is "SELECT countIf(TraceId != '') > 0 FROM otel.otel_logs" 1
  pass "logs carry trace ids"

  FAULT_SET=1
  out="$(curl -sS -o "$TMP/fault" -w '%{http_code}' -X POST "$SHOP_URL/fault?type=errors&pct=100" || true)"
  if [[ "$out" != 200 ]] || ! grep -q '"errors_pct":100' "$TMP/fault"; then
    fail "set error fault ($out: $(cat "$TMP/fault"))"
  fi
  pass "error fault accepted"
  poll 90 "checkout-0 serves mostly 500s" prom_above \
    'sum(increase(http_requests_total{pod="checkout-0",status="500"}[30s])) / sum(increase(http_requests_total{pod="checkout-0"}[30s]))' 0.9
  pass "error fault shows in metrics"
  poll 60 "error fault shows in logs" ch_positive \
    "SELECT count() FROM otel.otel_logs WHERE ResourceAttributes['k8s.pod.name'] = 'checkout-0' AND Body = 'database connection refused'"
  pass "error fault shows in logs"
  out="$(curl -sS -o "$TMP/fault" -w '%{http_code}' -X DELETE "$SHOP_URL/fault" || true)"
  if [[ "$out" != 200 ]] || ! grep -q '"errors_pct":0' "$TMP/fault"; then
    fail "reset fault ($out: $(cat "$TMP/fault"))"
  fi
  FAULT_SET=0
  pass "fault resets"
  out="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$SHOP_URL/fault?type=bogus" || true)"
  [[ "$out" == 400 ]] || fail "invalid fault type ($out)"
  pass "invalid fault rejected"
}

data_ready() { targets_scraped && logs_and_traces_present; }

ensure_stack() {
  if stack_reusable; then
    if poll_try 30 2 data_ready; then
      echo "Reusing the healthy $PROJECT stack at $BASE_URL"
      return 0
    fi
    echo "Stack $PROJECT not serving data within 30s (last: $LAST); rebooting"
  fi
  boot
}

browser() {
  ensure_stack
  cd "$ROOT/web"
  grouped bun run e2e:install
  grouped env SLASHOBSERVE_E2E_BASE_URL="$BASE_URL" bun run e2e
  check_api_contract
  check_no_keys_in_server_log
}

case "${1:-run}" in
  run | up | browser | down | logs)
    [[ "${1:-run}" == logs ]] || acquire_lock
    TMP="$(mktemp -d)"
    OUT_FILE="$TMP/capture.out"
    ERR_FILE="$TMP/capture.err"
    ;;
esac

case "${1:-run}" in
  run)
    boot
    suite
    stack_suite
    ;;
  up) ensure_stack ;;
  browser) browser ;;
  down) grouped "${COMPOSE_CMD[@]}" down -v --remove-orphans ;;
  logs) all_logs ;;
  -h | --help | help) usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
