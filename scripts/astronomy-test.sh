#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
failures=0

mkdir -p "$tmp/flagd"
cat >"$tmp/flagd/demo.flagd.json" <<'JSON'
{
  "flags": {
    "adFailure": {"state": "ENABLED", "variants": {"on": true, "off": false}, "defaultVariant": "off"},
    "cartFailure": {"state": "ENABLED", "variants": {"10%": 0.1, "100%": 1, "off": 0}, "defaultVariant": "off"}
  }
}
JSON

flag() {
  ASTRONOMY_STATE_DIR="$tmp" "$here/astronomy.sh" flag "$@"
}

variant_of() {
  jq -r --arg f "$1" '.flags[$f].defaultVariant' "$tmp/flagd/demo.flagd.json"
}

fail() {
  echo "FAIL $1" >&2
  failures=$((failures + 1))
}

listing="$(flag)"
[[ "$listing" == *"adFailure"*"off"*"off,on"* ]] || fail "the listing shows a flag, its variant and its variants: $listing"
[[ "$listing" == *"cartFailure"*"10%,100%,off"* ]] || fail "the listing shows a percentage flag: $listing"

flag adFailure >/dev/null
[[ "$(variant_of adFailure)" == on ]] || fail "a flag with no variant given is switched on"

flag cartFailure 100% >/dev/null
[[ "$(variant_of cartFailure)" == 100% ]] || fail "a flag is set to the variant named"
[[ "$(variant_of adFailure)" == on ]] || fail "setting one flag leaves the others alone"

flag adFailure off >/dev/null
[[ "$(variant_of adFailure)" == off ]] || fail "a flag is switched off"

before="$(cat "$tmp/flagd/demo.flagd.json")"
if message="$(flag cartFailure on 2>&1)"; then fail "a variant the flag lacks is refused"; fi
[[ "$message" == *"has no variant on; it has 10%, 100%, off"* ]] || fail "the refusal names the variants: $message"
if message="$(flag nope 2>&1)"; then fail "an unknown flag is refused"; fi
[[ "$message" == *"no flag named nope"* ]] || fail "the refusal names the flag: $message"
[[ "$(cat "$tmp/flagd/demo.flagd.json")" == "$before" ]] || fail "a refused change leaves the file alone"

inode_before="$(ls -i "$tmp/flagd/demo.flagd.json" | cut -d' ' -f1)"
flag adFailure on >/dev/null
[[ "$(ls -i "$tmp/flagd/demo.flagd.json" | cut -d' ' -f1)" == "$inode_before" ]] || fail "the flag file is rewritten in place"

if [[ "$failures" -gt 0 ]]; then
  exit 1
fi
echo "astronomy-test: ok"
