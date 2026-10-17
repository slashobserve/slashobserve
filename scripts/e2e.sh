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
export SLASHOBSERVE_API_CONTRACT_CHECK=true
export SLASHOBSERVE_LLM_PROVIDER=replay

curl() { command curl --max-time 10 "$@"; }

pass() { printf 'PASS  %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n' "$1" >&2
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

Requirements: docker with compose v2.20+, curl; 'browser' also needs bun and web dependencies.
EOF
}

compose() {
  docker compose -f "$ROOT/deploy/compose/compose.yaml" -p "$PROJECT" "$@"
}

PROM_URL="http://localhost:${SLASHOBSERVE_PROMETHEUS_HOST_PORT}"
SHOP_URL="http://localhost:${SLASHOBSERVE_SHOP_HOST_PORT}"
FAULT_SET=0

prom_scalar() {
  command curl -fsS --max-time 5 "$PROM_URL/api/v1/query" --data-urlencode "query=scalar($1)" 2>/dev/null \
    | sed -nE 's/.*"result":\[[^,]*,"([^"]*)"\].*/\1/p' || true
}

ch() {
  compose exec -T clickhouse sh -c \
    'exec clickhouse-client --connect_timeout=5 --receive_timeout=10 --user slashobserve_ro --password "$SLASHOBSERVE_RO_PASSWORD" -q "$1"' \
    sh "$1"
}

LAST=""
above() { awk -v v="$1" -v t="$2" 'BEGIN { exit !(v + 0 > t + 0) }'; }
positive() { above "$1" 0; }
prom_is() { LAST="$(prom_scalar "$1")"; [[ "$LAST" == "$2" ]]; }
prom_positive() { prom_above "$1" 0; }
prom_above() { LAST="$(prom_scalar "$1")"; above "$LAST" "$2"; }
ch_is() { LAST="$(ch "$1" 2>/dev/null || true)"; [[ "$LAST" == "$2" ]]; }
ch_positive() { LAST="$(ch "$1" 2>/dev/null || true)"; positive "$LAST"; }

poll() {
  local timeout="$1" desc="$2"
  shift 2
  local deadline=$((SECONDS + timeout))
  LAST=""
  until "$@"; do
    ((SECONDS < deadline)) || {
      compose logs --no-color --tail 50 >&2 || true
      fail "$desc (not true after ${timeout}s, last observed: '${LAST}')"
    }
    sleep 2
  done
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

is_ready() { command curl -fsS --max-time 3 "$BASE_URL/readyz" >/dev/null 2>&1; }

stack_reusable() {
  [[ -n "$(compose ps --status running -q server 2>/dev/null)" ]] && is_ready
}

boot() {
  [[ -n "${SLASHOBSERVE_SHOP_IMAGE:-}" ]] || fail "SLASHOBSERVE_SHOP_IMAGE must be set with SLASHOBSERVE_IMAGE"
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  if ! compose up -d "${BUILD_FLAGS[@]}" --wait --wait-timeout 300; then
    compose logs --no-color --tail 200 >&2 || true
    fail "stack did not become healthy"
  fi
  for _ in $(seq 1 60); do
    if is_ready; then
      wait_for_data
      return 0
    fi
    sleep 1
  done
  fail "server ready ($BASE_URL/readyz did not return 200)"
}

TMP=""
LOCK_DIR="${TMPDIR:-/tmp}/slashobserve-e2e-run.lock"
LOCK_HELD=0
SSE_PID=""

acquire_lock() {
  mkdir "$LOCK_DIR" 2>/dev/null || fail "another e2e run is active (lock: $LOCK_DIR)"
  LOCK_HELD=1
}

cleanup() {
  sse_close
  if [[ "$LOCK_HELD" == 1 ]]; then rmdir "$LOCK_DIR" 2>/dev/null || true; fi
  if [[ "$FAULT_SET" == 1 ]]; then command curl -s --max-time 5 -X DELETE "$SHOP_URL/fault" >/dev/null 2>&1 || true; fi
  if [[ -n "$TMP" ]]; then rm -rf "$TMP"; fi
}
trap cleanup EXIT
trap 'exit 1' INT TERM HUP

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
  TMP="$(mktemp -d)"
  local out asset llm_line

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

  llm_line="$(compose logs --no-color server | grep -F '"msg":"llm provider configured"' || true)"
  if [[ "$llm_line" != *'"provider":"replay"'* ]]; then
    fail "server did not boot with the replay LLM provider ($(compose logs --no-color --tail 50 server 2>&1))"
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
  check_api_contract
}

check_api_contract() {
  local logs violations
  logs="$(compose logs --no-color server 2>&1)"
  grep -q 'api contract check enabled' <<<"$logs" || fail "server did not enable the API contract check"
  pass "server runs with the API contract check enabled"
  violations="$(grep 'api contract violation' <<<"$logs" || true)"
  [[ -z "$violations" ]] || fail "API calls broke the OpenAPI contract:"$'\n'"$violations"
  pass "every API call matched the OpenAPI contract"
}

check_datasources() {
  local jar="$1" out body="" healthy=0 leak entry keys entries=0

  out="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/api/v1/datasources")"
  [[ "$out" == 401 ]] || fail "unauthenticated datasource list ($out)"
  pass "datasource list requires authentication"

  for _ in $(seq 1 60); do
    body="$(curl -sS -b "$jar" "$BASE_URL/api/v1/datasources" || true)"
    healthy="$(grep -o '"healthy":true' <<<"$body" | wc -l | tr -d ' ')"
    if [[ "$healthy" == 2 ]] && grep -q '"uid":"prom"' <<<"$body" && grep -q '"uid":"ch"' <<<"$body"; then
      break
    fi
    sleep 1
  done
  if [[ "$healthy" != 2 ]] || ! grep -q '"uid":"prom"' <<<"$body" || ! grep -q '"uid":"ch"' <<<"$body"; then
    fail "datasources did not report healthy within 60s ($body)"
  fi
  leak="$(grep -oE 'prometheus:9090|clickhouse:9000|slashobserve_ro|password' <<<"$body" || true)"
  [[ -z "$leak" ]] || fail "datasource list exposes connection details ($leak)"
  pass "datasources report healthy without exposing connection details"

  while IFS= read -r entry; do
    keys="$(grep -oE '"[A-Za-z0-9_]+":' <<<"$entry" | tr -d '":' | sort | paste -sd, -)"
    case "$keys" in
      checked_at,healthy,name,type,uid | checked_at,healthy,message,name,type,uid) entries=$((entries + 1)) ;;
      *) fail "datasource entry has unexpected keys ($keys)" ;;
    esac
  done < <(sed -E 's/:"([^"\\]|\\.)*"/:/g' <<<"$body" | grep -oE '\{[^{}]*\}')
  [[ "$entries" == 2 ]] || fail "expected 2 datasource entries, found $entries"
  pass "datasource entries carry exactly uid, type, name, healthy, checked_at and an optional message"
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
  local out id nil_id=00000000-0000-0000-0000-000000000000 leak ids cursor first_id found

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

  expect_rejected dashboard-failing-query.json query_invalid /panels/0/query
  pass "a panel whose query fails is rejected with the reason"

  expect_rejected dashboard-empty-query.json query_empty
  pass "a panel whose query returns no data is rejected"

  expect_rejected dashboard-ch-insert.json query_forbidden
  expect_rejected dashboard-ch-multi.json query_forbidden
  pass "ClickHouse writes and multi-statement queries are rejected"

  expect_rejected dashboard-unknown-ds.json unknown_datasource
  pass "a panel on an unknown datasource is rejected"

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
    || ! grep -q '"path":"/panels/0/query","code":"variable_unresolved"' "$TMP/api.body" \
    || grep -q '"path":"/panels/1' "$TMP/api.body"; then
    fail "broken variables ($out: $(cat "$TMP/api.body"))"
  fi
  pass "broken variables are reported with the panels that depend on them"
}

sse_open() {
  local id="$1" file="$2" tries=50
  : >"$file"
  command curl -sS -N --max-time 60 -H "Authorization: Bearer $API_TOKEN" "$BASE_URL/api/v1/dashboards/$id/events" >"$file" &
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
  grep -q "^data: {\"dashboard_id\":\"$id\",\"latest_version\":1}$" "$stream" || fail "SSE ready event should carry latest_version 1 ($(cat "$stream"))"
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
  pass "SSE responses are uncacheable text/event-stream"

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

stack_suite() {
  [[ -n "$TMP" ]] || TMP="$(mktemp -d)"
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
  { prom_positive 'count(node_filesystem_avail_bytes{instance="demo-node"})' \
    && prom_positive 'count(node_filesystem_files_free{instance="demo-node"})'; } \
    || fail "node disk and inode metrics missing for demo-node (last: $LAST)"
  pass "node disk and inode metrics present"
  prom_positive 'sum(rate(http_requests_total{service="checkout"}[5m] offset 1d))' || fail "no checkout request rate one day ago (got: $LAST)"
  pass "yesterday's request rate is backfilled"

  ch_is "SELECT getSetting('readonly')" 2 \
    || fail "ClickHouse readonly setting is not 2 (got: '$LAST', client: $(ch "SELECT getSetting('readonly')" 2>&1 >/dev/null | tail -3 || true))"
  if ro_out="$(ch "SELECT 1 SETTINGS max_result_rows = 0" 2>&1)"; then
    fail "ClickHouse user lifted max_result_rows"
  fi
  [[ "$ro_out" == *SETTING_CONSTRAINT_VIOLATION* || "$ro_out" == *"Code: 452"* ]] \
    || fail "ClickHouse max_result_rows refusal is not a constraint violation ($ro_out)"
  if ro_out="$(ch "INSERT INTO otel.otel_logs (Body) VALUES ('x')" 2>&1)"; then
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

wait_quiet() {
  local deadline=$((SECONDS + $1))
  shift
  until "$@"; do
    ((SECONDS < deadline)) || return 1
    sleep 2
  done
}

ensure_stack() {
  if stack_reusable; then
    if wait_quiet 30 data_ready; then
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
  bun run e2e:install
  SLASHOBSERVE_E2E_BASE_URL="$BASE_URL" bun run e2e
  check_api_contract
}

case "${1:-run}" in
  run | up | browser | down) acquire_lock ;;
esac

case "${1:-run}" in
  run)
    boot
    suite
    stack_suite
    ;;
  up) ensure_stack ;;
  browser) browser ;;
  down) compose down -v --remove-orphans ;;
  logs) compose logs --no-color --timestamps ;;
  -h | --help | help) usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
