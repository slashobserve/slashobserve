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

curl() { command curl --max-time 10 "$@"; }

pass() { printf 'PASS  %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n' "$1" >&2
  exit 1
}

if [[ -n "${SLASHOBSERVE_IMAGE:-}" ]]; then
  [[ -n "${SLASHOBSERVE_SHOP_IMAGE:-}" ]] || fail "SLASHOBSERVE_SHOP_IMAGE must be set with SLASHOBSERVE_IMAGE"
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

is_ready() { command curl -fsS --max-time 3 "$BASE_URL/readyz" >/dev/null 2>&1; }

stack_reusable() {
  [[ -n "$(compose ps --status running -q server 2>/dev/null)" ]] && is_ready
}

boot() {
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  if ! compose up -d "${BUILD_FLAGS[@]}" --wait --wait-timeout 300; then
    compose logs --no-color --tail 200 >&2 || true
    fail "stack did not become healthy"
  fi
  for _ in $(seq 1 60); do
    is_ready && return 0
    sleep 1
  done
  fail "server ready ($BASE_URL/readyz did not return 200)"
}

TMP=""
cleanup() { if [[ -n "$TMP" ]]; then rm -rf "$TMP"; fi; }
trap cleanup EXIT

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
  local out asset

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
}

ensure_stack() {
  if stack_reusable; then
    echo "Reusing the healthy $PROJECT stack at $BASE_URL"
  else
    boot
  fi
}

browser() {
  ensure_stack
  cd "$ROOT/web"
  bun run e2e:install
  SLASHOBSERVE_E2E_BASE_URL="$BASE_URL" bun run e2e
}

case "${1:-run}" in
  run)
    boot
    suite
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
