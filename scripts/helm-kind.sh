#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER="slashobserve-helm"
NODE="${CLUSTER}-control-plane"
NAMESPACE="slashobserve"
RELEASE="slashobserve"
KIND_DIR="$ROOT/deploy/helm/kind"
CHART="deploy/helm/slashobserve"
IMAGE_REPOSITORY="slashobserve-server"
IMAGE_TAG="helm"
CHECK_ACCOUNT="helm-check"
SERVER_PORT=18080
PROMETHEUS_PORT=19090

usage() {
  cat <<EOF
Usage: scripts/helm-kind.sh <command>

  up     create cluster ${CLUSTER} if absent, build and load ${IMAGE_REPOSITORY}:${IMAGE_TAG},
         deploy a throwaway Postgres and Prometheus, create generated Secrets and install the chart
  check  check readiness, /metrics, the Prometheus scrape and the provisioned internals dashboard,
         then upgrade and roll back the release
  down   delete cluster ${CLUSTER} only

Requirements: docker and go; kind and helm run as pinned Go tools and kubectl runs inside the control-plane node.
EOF
}

die() {
  printf 'helm-kind: %s\n' "$1" >&2
  exit 1
}

say() {
  printf 'helm-kind: %s\n' "$1"
}

kind() {
  go tool -modfile=tools/kind/go.mod kind "$@"
}

kc() {
  docker exec -i "$NODE" kubectl --kubeconfig=/etc/kubernetes/admin.conf "$@"
}

kcn() {
  kc -n "$NAMESPACE" "$@"
}

cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "$CLUSTER"
}

KUBECONFIG_DIR=""

write_kubeconfig() {
  KUBECONFIG_DIR="$(mktemp -d)"
  kind get kubeconfig --name "$CLUSTER" >"$KUBECONFIG_DIR/kubeconfig"
}

helm() {
  go tool -modfile=tools/helm/go.mod helm --kubeconfig "$KUBECONFIG_DIR/kubeconfig" --namespace "$NAMESPACE" "$@"
}

cleanup() {
  stop_forward server
  stop_forward prometheus
  [[ -z "$KUBECONFIG_DIR" ]] || rm -rf "$KUBECONFIG_DIR"
}

random_hex() {
  openssl rand -hex 32 | tr -d '\n'
}

random_key() {
  printf 'sok_%s' "$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')"
}

secret_from_stdin() {
  local name="$1" key="$2"
  shift 2
  kcn create secret generic "$name" --from-file="$key=/dev/stdin" "$@" >/dev/null
}

create_secrets() {
  local password
  if kcn get secret slashobserve-database >/dev/null 2>&1; then
    say "reusing the generated Secrets"
    return
  fi
  kcn delete secret --ignore-not-found slashobserve-postgres slashobserve-oidc slashobserve-secret-key slashobserve-service-accounts >/dev/null
  password="$(random_hex)"
  printf '%s' "$password" | secret_from_stdin slashobserve-postgres password
  random_hex | secret_from_stdin slashobserve-oidc client-secret
  random_hex | secret_from_stdin slashobserve-secret-key secret-key
  random_key | secret_from_stdin slashobserve-service-accounts "${CHECK_ACCOUNT}.key" \
    --from-literal=service-accounts.yaml="$(service_accounts_file)"
  printf 'postgres://slashobserve:%s@postgres:5432/slashobserve?sslmode=disable' "$password" |
    secret_from_stdin slashobserve-database url
}

service_accounts_file() {
  cat <<EOF
serviceAccounts:
  - name: ${CHECK_ACCOUNT}
    role: admin
    description: Checks the Helm chart on kind
    keyFile: /etc/slashobserve/service-accounts/${CHECK_ACCOUNT}.key
EOF
}

cmd_up() {
  cd "$ROOT"
  trap cleanup EXIT
  if cluster_exists; then
    say "cluster ${CLUSTER} already exists, reusing it"
  else
    kind create cluster --config "$KIND_DIR/kind.yaml" --wait 180s
  fi
  docker build -t "${IMAGE_REPOSITORY}:${IMAGE_TAG}" .
  kind load docker-image "${IMAGE_REPOSITORY}:${IMAGE_TAG}" --name "$CLUSTER"
  write_kubeconfig
  kc create namespace "$NAMESPACE" --dry-run=client -o yaml | kc apply -f - >/dev/null
  create_secrets
  kcn apply -f - <"$KIND_DIR/postgres.yaml"
  kcn apply -f - <"$KIND_DIR/prometheus.yaml"
  kcn rollout status deployment/postgres --timeout=300s
  kcn rollout status deployment/prometheus --timeout=300s
  if helm status "$RELEASE" >/dev/null 2>&1; then
    say "removing the existing release so its history starts at revision 1"
    helm uninstall "$RELEASE" --wait
  fi
  helm install "$RELEASE" "$CHART" -f "$KIND_DIR/values.yaml" \
    --set image.repository="$IMAGE_REPOSITORY" --set image.tag="$IMAGE_TAG" \
    --wait --timeout 10m
}

forward_pid_file() {
  printf '/tmp/helm-kind-forward-%s.pid' "$1"
}

start_forward() {
  local name="$1" target="$2" ports="$3" pid_file
  pid_file="$(forward_pid_file "$name")"
  docker exec -d "$NODE" sh -c \
    "echo \$\$ >$pid_file; exec timeout 900 kubectl --kubeconfig=/etc/kubernetes/admin.conf -n $NAMESPACE port-forward $target $ports"
}

stop_forward() {
  local pid_file
  pid_file="$(forward_pid_file "$1")"
  docker exec "$NODE" sh -c "[ ! -f $pid_file ] || { kill \"\$(cat $pid_file)\" 2>/dev/null; rm -f $pid_file; }" 2>/dev/null || true
}

node_curl() {
  docker exec -i "$NODE" curl -fsS --max-time 10 "$@"
}

eventually() {
  local seconds="$1" what="$2" deadline
  shift 2
  deadline=$((SECONDS + seconds))
  until "$@"; do
    ((SECONDS < deadline)) || die "$what did not happen within ${seconds}s"
    sleep 5
  done
}

server_ready() {
  stop_forward server
  start_forward server "svc/${RELEASE}" "${SERVER_PORT}:8080"
  sleep 2
  [[ "$(node_curl -o /dev/null -w "%{http_code}" "http://127.0.0.1:${SERVER_PORT}/readyz" 2>/dev/null)" == 200 ]]
}

check_ready() {
  kcn rollout status "deployment/${RELEASE}" --timeout=300s
  eventually 60 "/readyz answering 200" server_ready
  say "/readyz answers 200 ($1)"
}

metrics_served() {
  local body
  body="$(node_curl "http://127.0.0.1:${SERVER_PORT}/metrics")" || return 1
  grep -q '^slashobserve_http_request_duration_seconds' <<<"$body"
}

prometheus_scrapes() {
  local body
  body="$(node_curl -G --data-urlencode 'query=up{job="slashobserve"}' "http://127.0.0.1:${PROMETHEUS_PORT}/api/v1/query" 2>/dev/null)" || return 1
  perl -MJSON::PP -e '
    my $d = decode_json(join "", <STDIN>);
    my @r = @{$d->{data}{result} || []};
    exit(@r && $r[0]{value}[1] eq "1" ? 0 : 1);
  ' <<<"$body"
}

provision_status() {
  printf 'Authorization: Bearer %s\n' "$CHECK_KEY" |
    node_curl -H @- "http://127.0.0.1:${SERVER_PORT}/api/v1/provision"
}

internals_provisioned() {
  local body
  body="$(provision_status 2>/dev/null)" || return 1
  perl -MJSON::PP -e '
    my $d = decode_json(join "", <STDIN>);
    for my $f (@{$d->{dashboards}}) {
      exit 0 if $f->{path} eq "SlashObserve/internals.json" && !defined $f->{last_error} && $f->{dashboard_id};
    }
    exit 1;
  ' <<<"$body"
}

revisions() {
  helm history "$RELEASE" -o json | perl -MJSON::PP -e 'print scalar @{decode_json(join "", <STDIN>)}'
}

cmd_check() {
  local before after
  cd "$ROOT"
  cluster_exists || die "cluster ${CLUSTER} does not exist; run scripts/helm-kind.sh up"
  write_kubeconfig
  trap cleanup EXIT
  CHECK_KEY="$(kcn get secret slashobserve-service-accounts -o "jsonpath={.data.${CHECK_ACCOUNT}\\.key}" | base64 -d)"
  [[ -n "$CHECK_KEY" ]] || die "the ${CHECK_ACCOUNT} key Secret is empty"

  check_ready "install"
  metrics_served || die "/metrics has no slashobserve_http_request_duration_seconds series"
  say "/metrics serves slashobserve_http_request_duration_seconds"

  start_forward prometheus svc/prometheus "${PROMETHEUS_PORT}:9090"
  eventually 120 'Prometheus answering up{job="slashobserve"} 1' prometheus_scrapes
  say 'Prometheus answers up{job="slashobserve"} 1'

  if ! (eventually 300 "the internals dashboard provisioning" internals_provisioned); then
    provision_status | perl -MJSON::PP -e 'print JSON::PP->new->pretty->encode(decode_json(join "", <STDIN>))' >&2 || true
    die "SlashObserve/internals.json was not provisioned"
  fi
  say "SlashObserve/internals.json is provisioned with no error"

  before="$(revisions)"
  helm upgrade "$RELEASE" "$CHART" --reuse-values --set logLevel=debug --wait --timeout 10m >/dev/null
  check_ready "upgrade"
  helm rollback "$RELEASE" 1 --wait --timeout 10m
  check_ready "rollback"
  after="$(revisions)"
  ((after == before + 2)) || die "helm history went from ${before} to ${after} revisions, expected $((before + 2))"
  helm history "$RELEASE"
}

cmd_down() {
  cd "$ROOT"
  say "deleting cluster ${CLUSTER} only"
  kind delete cluster --name "$CLUSTER"
}

main() {
  local command="${1:-}"
  case "$command" in
    up) cmd_up ;;
    check) cmd_check ;;
    down) cmd_down ;;
    -h | --help | help) usage ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
