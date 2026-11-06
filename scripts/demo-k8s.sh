#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER="slashobserve-demo"
NODE="${CLUSTER}-control-plane"
NETWORK="kind"
K8S_DIR="$ROOT/demo/k8s"
SHOP_IMAGE="${SLASHOBSERVE_DEMO_K8S_SHOP_IMAGE:-slashobserve-demo-shop:k8s}"
SHOP_PORT="${SLASHOBSERVE_DEMO_K8S_SHOP_PORT:-38081}"
SHOP1_PORT="${SLASHOBSERVE_DEMO_K8S_SHOP1_PORT:-38082}"
PROMETHEUS_PORT="${SLASHOBSERVE_DEMO_K8S_PROMETHEUS_PORT:-39090}"
MANIFESTS=(shop kube-state-metrics node-exporter prometheus)
RULES_NAMESPACE="monitoring"
RULES_CONFIGMAP="prometheus-rules"
RULES_STAGING="/var/tmp/slashobserve-rules"
ROLLOUT_TIMEOUT="${SLASHOBSERVE_DEMO_K8S_ROLLOUT_TIMEOUT:-300s}"

usage() {
  cat <<EOF
Usage: scripts/demo-k8s.sh <command>

  up                       create cluster ${CLUSTER} (one control-plane, two workers), build and load
                           the shop image, apply demo/k8s and wait for every rollout
  connect <container>      attach a running container to the cluster network, so it reaches
                           ${NODE}:30090 by name
  down                     delete cluster ${CLUSTER} only
  status                   show the nodes and pods of ${CLUSTER}; fails when it does not exist
  fault <pod> <type> <pct> set a fault on checkout-0 or checkout-1 (type errors or latency, pct 0 clears it)

Environment:
  SLASHOBSERVE_DEMO_K8S_SHOP_PORT        host port of the checkout-0 fault API (default 38081)
  SLASHOBSERVE_DEMO_K8S_SHOP1_PORT       host port of the checkout-1 fault API (default 38082)
  SLASHOBSERVE_DEMO_K8S_PROMETHEUS_PORT  host port of Prometheus (default 39090)
  SLASHOBSERVE_DEMO_K8S_SHOP_IMAGE       tag of the shop image loaded into the cluster (default slashobserve-demo-shop:k8s)
  SLASHOBSERVE_DEMO_K8S_ROLLOUT_TIMEOUT  wait per rollout (default 300s)

Requirements: docker and go; kind runs as a pinned Go tool and kubectl runs inside the control-plane node.
EOF
}

die() {
  printf 'demo-k8s: %s\n' "$1" >&2
  exit 1
}

kind() {
  go tool -modfile=tools/kind/go.mod kind "$@"
}

kc() {
  docker exec -i "$NODE" kubectl --kubeconfig=/etc/kubernetes/admin.conf "$@"
}

cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "$CLUSTER"
}

render_config() {
  sed \
    -e "s|\${SLASHOBSERVE_DEMO_K8S_SHOP_PORT}|${SHOP_PORT}|g" \
    -e "s|\${SLASHOBSERVE_DEMO_K8S_SHOP1_PORT}|${SHOP1_PORT}|g" \
    -e "s|\${SLASHOBSERVE_DEMO_K8S_PROMETHEUS_PORT}|${PROMETHEUS_PORT}|g" \
    "$K8S_DIR/kind.yaml"
}

apply_manifests() {
  local name
  for name in "${MANIFESTS[@]}"; do
    [[ -f "$K8S_DIR/$name.yaml" ]] || die "missing demo/k8s/$name.yaml"
    kc apply -f - <"$K8S_DIR/$name.yaml"
  done
}

apply_rules() {
  docker exec "$NODE" rm -rf "$RULES_STAGING"
  docker cp "$K8S_DIR/rules/." "$NODE:$RULES_STAGING"
  kc -n "$RULES_NAMESPACE" create configmap "$RULES_CONFIGMAP" --from-file="$RULES_STAGING" --dry-run=client -o yaml |
    kc apply --server-side --force-conflicts -f -
}

wait_rollouts() {
  local namespace kind_name name
  while read -r namespace kind_name name; do
    kc -n "$namespace" rollout status "$kind_name/$name" --timeout="$ROLLOUT_TIMEOUT" </dev/null
  done < <(kc get deployments,daemonsets -A --no-headers \
    -o custom-columns='NS:.metadata.namespace,KIND:.kind,NAME:.metadata.name' | tr 'A-Z' 'a-z')
}

cmd_up() {
  cd "$ROOT"
  if cluster_exists; then
    printf 'demo-k8s: cluster %s already exists, reusing it\n' "$CLUSTER"
  else
    render_config | kind create cluster --config - --wait 180s
  fi
  docker build -t "$SHOP_IMAGE" demo
  kind load docker-image "$SHOP_IMAGE" --name "$CLUSTER"
  apply_manifests
  apply_rules
  wait_rollouts
  printf 'demo-k8s: Prometheus http://127.0.0.1:%s, checkout-0 fault API http://127.0.0.1:%s/fault, checkout-1 fault API http://127.0.0.1:%s/fault\n' \
    "$PROMETHEUS_PORT" "$SHOP_PORT" "$SHOP1_PORT"
}

cmd_connect() {
  local container="${1:-}" out
  [[ -n "$container" ]] || die "usage: connect <container>"
  out="$(docker network connect "$NETWORK" "$container" 2>&1)" || [[ "$out" == *"already exists"* ]] || die "$out"
}

cmd_down() {
  cd "$ROOT"
  printf 'demo-k8s: deleting cluster %s only\n' "$CLUSTER"
  kind delete cluster --name "$CLUSTER"
}

cmd_status() {
  cd "$ROOT"
  cluster_exists || die "cluster ${CLUSTER} does not exist; run scripts/demo-k8s.sh up"
  printf '%s\n' "$CLUSTER"
  kc get nodes
  kc get pods -A
}

cmd_fault() {
  local pod="${1:-}" type="${2:-}" pct="${3:-}" port
  [[ $# -eq 3 ]] || die "usage: fault <pod> <type> <pct>"
  case "$pod" in
    checkout-0) port="$SHOP_PORT" ;;
    checkout-1) port="$SHOP1_PORT" ;;
    *) die "pod must be checkout-0 or checkout-1" ;;
  esac
  curl -fsS -X POST "http://127.0.0.1:${port}/fault?type=${type}&pct=${pct}"
}

main() {
  local command="${1:-}"
  shift || true
  case "$command" in
    up) cmd_up ;;
    connect) cmd_connect "$@" ;;
    down) cmd_down ;;
    status) cmd_status ;;
    fault) cmd_fault "$@" ;;
    -h | --help | help) usage ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
