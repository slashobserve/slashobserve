#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
. "$here/astronomy.sh"
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

chmod 600 "$tmp/flagd/demo.flagd.json"
inode_before="$(ls -i "$tmp/flagd/demo.flagd.json" | cut -d' ' -f1)"
flag adFailure on >/dev/null
[[ "$(ls -i "$tmp/flagd/demo.flagd.json" | cut -d' ' -f1)" != "$inode_before" ]] || fail "the flag file is replaced by a rename, not rewritten in place"
[[ "$(variant_of adFailure)" == on ]] || fail "the replaced file has the new variant"
[[ "$(ls -A "$tmp/flagd")" == demo.flagd.json ]] || fail "no temporary file is left in the flag directory: $(ls -A "$tmp/flagd")"
mode="$(ls -l "$tmp/flagd/demo.flagd.json" | cut -c2-10)"
[[ "$mode" == rw-r--r-- ]] || fail "the replaced file is readable by flagd and writable only by its owner, got $mode"

mkdir -p "$tmp/keys"
echo sok_test >"$tmp/keys/astronomy.key"
started=$SECONDS
status=0
output="$(ASTRONOMY_STATE_DIR="$tmp" ASTRONOMY_CHECK_WAIT=4 SLASHOBSERVE_HOST_PORT=1 "$here/astronomy.sh" check 2>&1)" || status=$?
[[ "$status" == 1 ]] || fail "check exits 1 when the server never answers, got $status"
[[ "$output" == *"FAIL"*"the datasource list is not available"* ]] || fail "check prints a FAIL line for the list: $output"
((SECONDS - started >= 4)) || fail "check keeps retrying the list until its deadline"

state="$tmp/state"
mkdir -p "$tmp/up/src/flagd"
echo '{"flags":{}}' >"$tmp/up/src/flagd/demo.flagd.json"
UPSTREAM_DIR="$tmp/up" STATE_DIR="$state" KEY_FILE="$state/keys/astronomy.key" FLAGS_FILE="$state/flagd/demo.flagd.json"
prepare_state
first_key="$(cat "$KEY_FILE")"
echo '{"flags":{"kept":{}}}' >"$FLAGS_FILE"
prepare_state
[[ "$(cat "$KEY_FILE")" == "$first_key" ]] || fail "a second prepare keeps the key"
[[ "$(cat "$FLAGS_FILE")" == *kept* ]] || fail "a second prepare keeps the flag file"
[[ "$first_key" == sok_* ]] || fail "the generated key has the sok_ prefix"
[[ "$(ls -ld "$state/keys" | cut -c1-10)" == drwxr-x--- ]] || fail "the keys directory is closed to others"
[[ "$(ls -l "$KEY_FILE" | cut -c1-10)" == -rw-r----- ]] || fail "the key is closed to others"
[[ "$(ls -ld "$state/flagd" | cut -c1-10)" == drwxr-xr-x ]] || fail "the flag directory is not writable by others"
[[ "$(ls -l "$FLAGS_FILE" | cut -c1-10)" == -rw-r--r-- ]] || fail "the flag file is not writable by others"

upstream='["--config=/etc/otelcol-config.yml","--config=/etc/otelcol-config-full.yml","--config=/etc/otelcol-config-extras.yml","--feature-gates=service.profilesSupport"]'
copied='["--config=/etc/otelcol-config.yml","--config=/etc/otelcol-config-full.yml","--config=/etc/otelcol-config-extras.yml","--config=/etc/otelcol-config-search.yml","--feature-gates=service.profilesSupport"]'
search_command_matches "$upstream" "$copied" || fail "a search command equal to upstream's plus the search config is accepted"
drifted='["--config=/etc/otelcol-config.yml","--config=/etc/otelcol-config-extras.yml","--config=/etc/otelcol-config-search.yml","--feature-gates=service.profilesSupport"]'
if search_command_matches "$upstream" "$drifted"; then fail "a copy that dropped an upstream argument is refused"; fi
grown='["--config=/etc/otelcol-config.yml","--config=/etc/otelcol-config-full.yml","--config=/etc/otelcol-config-extras.yml","--feature-gates=service.profilesSupport","--set=x=y","--config=/etc/otelcol-config-search.yml"]'
if search_command_matches "$upstream" "$grown"; then fail "a copy with an argument upstream lacks is refused"; fi
if search_command_matches "$upstream" "$upstream"; then fail "a command without the search config is refused"; fi

if [[ "$failures" -gt 0 ]]; then
  exit 1
fi
echo "astronomy-test: ok"
