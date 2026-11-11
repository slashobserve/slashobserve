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

awk '/^## Helm values for two or more servers$/ {s=1; next} s && /^```yaml$/ {p=1; next} p && /^```$/ {exit} p' docs/operate/scaling.md >"$work/scaling.yaml"
[[ -s "$work/scaling.yaml" ]] || die "docs/operate/scaling.md has no yaml block under 'Helm values for two or more servers'"
run helm lint --strict "$CHART" -f "$work/required.yaml" -f "$work/scaling.yaml" >/dev/null
run helm template t "$CHART" -f "$work/required.yaml" -f "$work/scaling.yaml" >"$work/scaling.out"
grep -q '^  replicas: 2$' "$work/scaling.out" || die "the scaling doc's values do not render replicas: 2"
grep -q 'SLASHOBSERVE_DATABASE_DIRECT_URL' "$work/scaling.out" || die "the scaling doc's values do not set SLASHOBSERVE_DATABASE_DIRECT_URL"

if helm template t "$CHART" >/dev/null 2>&1; then
  die "the chart renders without its required values"
fi

cmp -s "deploy/compose/$INTERNALS" "$CHART/$INTERNALS" ||
  die "$CHART/$INTERNALS differs from deploy/compose/$INTERNALS; copy the compose file over it"
