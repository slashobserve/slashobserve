#!/usr/bin/env bash
set -euo pipefail

heavy=(./pkg/grafana/... ./evals/...)

case "${1:-}" in
  grafana) echo "./pkg/grafana/..." ;;
  evals) echo "./evals/..." ;;
  rest)
    go list ./... | grep -v -F -f <(go list "${heavy[@]}") | tr '\n' ' '
    ;;
  *)
    echo "usage: $0 grafana|evals|rest" >&2
    exit 2
    ;;
esac
