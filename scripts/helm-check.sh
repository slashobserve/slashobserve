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

if helm template t "$CHART" >/dev/null 2>&1; then
  die "the chart renders without its required values"
fi

cmp -s "deploy/compose/$INTERNALS" "$CHART/$INTERNALS" ||
  die "$CHART/$INTERNALS differs from deploy/compose/$INTERNALS; copy the compose file over it"
