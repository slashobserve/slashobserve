#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/redact.sh
. "$here/redact.sh"
failures=0

check() {
  local input="$1" want="$2" got
  got="$(printf '%s\n' "$input" | redact)"
  if [[ "$got" != "$want" ]]; then
    echo "FAIL redact: $input" >&2
    echo "  got:  $got" >&2
    echo "  want: $want" >&2
    failures=$((failures + 1))
  fi
}

for prefix in sok sot sob soa sor soc pub; do
  check "token ${prefix}_AbC-12_xyz in a dump" "token ${prefix}_[redacted] in a dump"
done
check "Authorization: Bearer abc.def" "Authorization: [redacted]"
check "curl -H 'Bearer abc.def'" "curl -H 'Bearer [redacted]'"
check "postgres://user:pw@db:5432/x" "postgres://[redacted]@db:5432/x"
check '{"api_key":"abc","tokens":12}' '{"api_key":"[redacted]","tokens":12}'
check "GET /callback?code=xyz&state=1" "GET /callback?code=[redacted]&state=1"
check "--password hunter2" "--password [redacted]"
check "so_session abc" "so_session [redacted]"
check "the soft_limit and a plain word stay" "the soft_limit and a plain word stay"

got="$(printf 'held secret-value-1\n' | REDACT_LITERAL=secret-value-1 redact)"
[[ "$got" == "held [redacted]" ]] || {
  echo "FAIL redact: REDACT_LITERAL hides the given value, got: $got" >&2
  failures=$((failures + 1))
}

if ((failures > 0)); then
  exit 1
fi
echo "redact tests passed"
