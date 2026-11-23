#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
CHART="deploy/helm/slashobserve"
KIND_VALUES="deploy/helm/kind/values.yaml"
INTERNALS="dashboards/SlashObserve/internals.json"

helm() {
  go tool -modfile=tools/helm/go.mod helm "$@"
}

die() {
  printf 'helm-check: %s\n' "$1" >&2
  exit 1
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cat >"$work/required.yaml" <<'EOF'
publicURL: https://slashobserve.example.com
database:
  existingSecret: slashobserve-database
oidc:
  issuerURL: https://idp.example.com
  clientID: slashobserve
  existingSecret: slashobserve-oidc
auth:
  allowedEmailDomains: [example.com]
EOF

cat >"$work/everything.yaml" <<'EOF'
database:
  directUrlKey: direct-url
image:
  digest: sha256:0000000000000000000000000000000000000000000000000000000000000000
imagePullSecrets:
  - name: registry
auth:
  adminEmails: [admin@example.com]
secretKey:
  existingSecret: slashobserve-secret-key
llm:
  provider: anthropic
  model: claude-sonnet-5-5
  effort: medium
  apiKey:
    existingSecret: slashobserve-llm
outboundDomains: [hooks.example.com]
trustedProxies: [10.0.0.0/8, 2001:db8::/32]
env:
  - name: SLASHOBSERVE_CLICKHOUSE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: slashobserve-clickhouse
        key: password
  - name: SLASHOBSERVE_SESSION_TTL
    value: 24h
datasources:
  existingConfigMap: slashobserve-datasources
serviceAccounts:
  existingSecret: slashobserve-service-accounts
provisioning:
  folders:
    - directory: Team %2F Payments
      configMap: payments-dashboards
metrics:
  serviceMonitor:
    enabled: true
    labels:
      release: prometheus
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
  tlsSecretName: slashobserve-tls
podAnnotations:
  team: observability
resources:
  limits:
    memory: 256Mi
nodeSelector:
  kubernetes.io/os: linux
tolerations:
  - key: dedicated
    operator: Equal
    value: observability
    effect: NoSchedule
affinity:
  nodeAffinity: {}
EOF

run() {
  local out
  if ! out="$("$@" 2>&1)"; then
    printf '%s\n' "$out" >&2
    die "failed: $*"
  fi
  printf '%s\n' "$out"
}

run helm lint --strict "$CHART" -f "$KIND_VALUES" >/dev/null
run helm lint --strict "$CHART" -f "$work/required.yaml" -f "$work/everything.yaml" >/dev/null

for values in required everything; do
  args=(-f "$work/required.yaml")
  [[ "$values" == everything ]] && args+=(-f "$work/everything.yaml")
  run helm template t "$CHART" "${args[@]}" >"$work/$values.out"
  if grep -q '^kind: Secret$' "$work/$values.out"; then
    die "the $values render creates a Secret"
  fi
done

grep -q 'SLASHOBSERVE_DATABASE_DIRECT_URL' "$work/everything.out" || die "the everything render has no SLASHOBSERVE_DATABASE_DIRECT_URL"
! grep -q 'SLASHOBSERVE_DATABASE_DIRECT_URL' "$work/required.out" || die "the required render sets SLASHOBSERVE_DATABASE_DIRECT_URL"

grep -q 'value: "10.0.0.0/8,2001:db8::/32"' "$work/everything.out" || die "the everything render does not set SLASHOBSERVE_TRUSTED_PROXIES"
! grep -q 'SLASHOBSERVE_TRUSTED_PROXIES' "$work/required.out" || die "the required render sets SLASHOBSERVE_TRUSTED_PROXIES"

grep -q 'name: t-slashobserve-no-metrics' "$work/everything.out" || die "the everything render does not hold /metrics back from the Ingress"
run helm template t "$CHART" -f "$work/required.yaml" -f "$work/everything.yaml" --set ingress.exposeMetrics=true >"$work/exposed.out"
! grep -q 'no-metrics' "$work/exposed.out" || die "ingress.exposeMetrics=true still holds /metrics back"

awk '/^## Helm values for two or more servers$/ {s=1; next} s && /^```yaml$/ {p=1; next} p && /^```$/ {exit} p' docs/operate/scaling.md >"$work/scaling.yaml"
[[ -s "$work/scaling.yaml" ]] || die "docs/operate/scaling.md has no yaml block under 'Helm values for two or more servers'"
run helm lint --strict "$CHART" -f "$work/required.yaml" -f "$work/scaling.yaml" >/dev/null
run helm template t "$CHART" -f "$work/required.yaml" -f "$work/scaling.yaml" >"$work/scaling.out"
grep -q '^  replicas: 2$' "$work/scaling.out" || die "the scaling doc's values do not render replicas: 2"
grep -q 'SLASHOBSERVE_DATABASE_DIRECT_URL' "$work/scaling.out" || die "the scaling doc's values do not set SLASHOBSERVE_DATABASE_DIRECT_URL"

cat >"$work/sign-in-off.yaml" <<'EOF2'
publicURL: https://slashobserve.example.com
database:
  existingSecret: slashobserve-database
serviceAccounts:
  existingSecret: slashobserve-service-accounts
EOF2
run helm lint --strict "$CHART" -f "$work/sign-in-off.yaml" >/dev/null
run helm template t "$CHART" -f "$work/sign-in-off.yaml" >"$work/sign-in-off.out"
! grep -q 'SLASHOBSERVE_OIDC_\|SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS' "$work/sign-in-off.out" || die "the sign-in off render sets an OIDC or sign-in variable"
grep -q 'SLASHOBSERVE_SERVICE_ACCOUNTS_FILE' "$work/sign-in-off.out" || die "the sign-in off render does not set SLASHOBSERVE_SERVICE_ACCOUNTS_FILE"
grep -q 'SLASHOBSERVE_OIDC_CLIENT_SECRET' "$work/required.out" || die "the required render does not set SLASHOBSERVE_OIDC_CLIENT_SECRET"

refused() {
  local name="$1" want="$2"
  shift 2
  local out
  if out="$(helm template t "$CHART" -f "$work/required.yaml" "$@" 2>&1)"; then
    die "the chart renders with $name"
  fi
  grep -q "$want" <<<"$out" || die "the render with $name does not say '$want'"
}
refused "only some OIDC settings" "missing oidc.clientID" --set oidc.clientID=
refused "blank allowed email domains" "auth.allowedEmailDomains is required" --set-json 'auth.allowedEmailDomains=[" "]'
refused "no allowed email domains" "auth.allowedEmailDomains is required" --set-json 'auth.allowedEmailDomains=[]'
if out="$(helm template t "$CHART" -f "$work/sign-in-off.yaml" --set-json 'serviceAccounts.existingSecret=""' 2>&1)"; then
  die "the chart renders with neither sign-in nor service-account keys"
fi
grep -q 'nobody could sign in' <<<"$out" || die "the render with neither sign-in nor service-account keys does not say nobody could sign in"
if out="$(helm template t "$CHART" -f "$work/sign-in-off.yaml" --set 'auth.adminEmails={a@example.com}' 2>&1)"; then
  die "the chart renders admin emails with sign-in off"
fi
grep -q 'only apply to browser sign-in' <<<"$out" || die "admin emails with sign-in off do not say they only apply to browser sign-in"

if helm template t "$CHART" >/dev/null 2>&1; then
  die "the chart renders without its required values"
fi

cmp -s "deploy/compose/$INTERNALS" "$CHART/$INTERNALS" ||
  die "$CHART/$INTERNALS differs from deploy/compose/$INTERNALS; copy the compose file over it"
