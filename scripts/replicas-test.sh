#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$here/replicas.sh"
failures=0
export REPLICAS_POLL_SECONDS=1

fail() {
  echo "FAIL $1" >&2
  failures=$((failures + 1))
}

counter="$(mktemp)"
trap 'rm -f "$counter"' EXIT
: >"$counter"
curl() {
  echo x >>"$counter"
  if (($(wc -l <"$counter") < 3)); then
    printf '{"status":"success","data":{"resultType":"vector","result":[]}}'
  else
    printf '{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"0.4"]}]}}'
  fi
}
(wait_prometheus_series "up" 10) || fail "the wait returns once the query has a value"
(($(wc -l <"$counter") == 3)) || fail "the wait polls until the third answer, polled $(wc -l <"$counter") times"

curl() { printf '{"status":"success","data":{"resultType":"vector","result":[]}}'; }
if message="$( (wait_prometheus_series "up" 2) 2>&1)"; then fail "an empty result for the whole wait fails"; fi
[[ "$message" == *"Prometheus returned no value for up within 2s"* ]] || fail "the failure names the query: $message"

curl() { return 7; }
if (wait_prometheus_series "up" 2) >/dev/null 2>&1; then fail "an unreachable Prometheus fails the wait"; fi

if [[ "$failures" -gt 0 ]]; then
  exit 1
fi
echo "replicas-test: ok"
