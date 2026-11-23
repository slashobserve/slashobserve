#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"
failures=0

expect() {
  local name="$1" want="$2" got
  shift 2
  got="$(printf '%s\n' "$@" | "$here/ci-changes.sh" | tr '\n' ' ')"
  if [[ "$got" != "$want " ]]; then
    echo "FAIL $name: got '$got', want '$want '" >&2
    failures=$((failures + 1))
  fi
}

expect_draft() {
  local name="$1" want="$2" got
  shift 2
  got="$(printf '%s\n' "$@" | "$here/ci-changes.sh" draft | tr '\n' ' ')"
  if [[ "$got" != "$want " ]]; then
    echo "FAIL $name: got '$got', want '$want '" >&2
    failures=$((failures + 1))
  fi
}

expect_draft "a draft pull request skips the compose jobs" \
  "go=true web=false build=true stack=false image=false audit=false" internal/agent/tools/query.go
expect_draft "a draft pull request that changes the workflow still skips the compose jobs" \
  "go=true web=true build=true stack=false image=true audit=true" .github/workflows/ci.yml
expect "a docs page runs the Go tests that read docs and nothing else" \
  "go=true web=false build=false stack=false image=false audit=false" docs/use/mcp.md
expect "a web component skips Go" \
  "go=false web=true build=true stack=true image=false audit=false" web/src/inbox/inbox-row.tsx
expect "a web unit test skips the stack" \
  "go=false web=true build=true stack=false image=false audit=false" web/src/inbox/inbox-row.test.tsx
expect "a web file that a Go test reads runs Go" \
  "go=true web=true build=true stack=true image=false audit=false" web/src/spec/defaults.gen.ts
expect "a Go handler skips web" \
  "go=true web=false build=true stack=true image=false audit=false" internal/observations/http.go
expect "a Go unit test skips web and the stack" \
  "go=true web=false build=true stack=false image=false audit=false" internal/agent/tools/query_test.go
expect "an agent tool needs the stack" \
  "go=true web=false build=true stack=true image=false audit=false" internal/agent/tools/query.go
expect "an agent prompt is not prose" \
  "go=true web=true build=true stack=true image=false audit=false" internal/agent/prompts/spec_guide.md
expect "the sidecar does not run Go" \
  "go=false web=true build=true stack=true image=false audit=false" observe-agent/src/main.ts
expect "the sidecar protocol runs Go too" \
  "go=true web=true build=true stack=true image=false audit=false" observe-agent/src/protocol.ts
expect "a test fixture that the web reads runs web" \
  "go=true web=true build=true stack=true image=false audit=false" internal/kql/testdata/cases.json
expect "a datasource adapter needs the stack" \
  "go=true web=false build=true stack=true image=false audit=false" internal/datasource/loki/query.go
expect "the importer needs the stack" \
  "go=true web=false build=true stack=true image=false audit=false" pkg/grafana/convert.go
expect "the Dockerfile builds the image from its own stages" \
  "go=true web=true build=true stack=true image=true audit=false" Dockerfile
expect "a lockfile runs the audit" \
  "go=false web=true build=true stack=true image=false audit=true" web/bun.lock
expect "go.mod runs everything but web" \
  "go=true web=false build=true stack=true image=false audit=true" go.mod
expect "the workflow runs everything" \
  "go=true web=true build=true stack=true image=true audit=true" .github/workflows/ci.yml
expect "one broad file widens a narrow change" \
  "go=true web=true build=true stack=true image=false audit=false" docs/use/mcp.md web/src/app.tsx internal/db/pool.go
expect "no files need nothing" \
  "go=false web=false build=false stack=false image=false audit=false" ""

all="$("$here/ci-changes.sh" all | tr '\n' ' ')"
[[ "$all" == "go=true web=true build=true stack=true image=true audit=true " ]] || {
  echo "FAIL all: got '$all'" >&2
  failures=$((failures + 1))
}

declared="$("$here/ci-changes.sh" go-reads | LC_ALL=C sort)"
literal_paths() {
  grep -rhoE '"(\.\./)+(web|observe-agent)/[^"]+"' --include='*.go' internal pkg cmd loadtest | tr -d '"'
}

joined_paths() {
  grep -rhoE 'filepath\.Join\("\.\."(, "[^"]+")*, "(web|observe-agent)"(, "[^"]+")+\)' --include='*.go' internal pkg cmd loadtest |
    sed -E 's/^filepath\.Join\(//; s/\)$//; s/", "/\//g' | tr -d '"'
}

read_by_go="$(cd "$root" && { literal_paths || true; joined_paths || true; } | sed -E 's#^(\.\./)+##' | LC_ALL=C sort -u)"
if [[ "$declared" != "$read_by_go" ]]; then
  echo "FAIL go-reads: Go files read these web and agent files:" >&2
  echo "$read_by_go" >&2
  echo "but scripts/ci-changes.sh lists:" >&2
  echo "$declared" >&2
  failures=$((failures + 1))
fi

matrix_shards="$(sed -n 's/^ *shard: \[\(grafana.*\)\]$/\1/p' "$root/.github/workflows/ci.yml" | tr -d ',')"
script_shards="$("$here/go-test-shard.sh" shards)"
if [[ "$matrix_shards" != "$script_shards" ]]; then
  echo "FAIL shards: the go-test matrix runs '$matrix_shards' but scripts/go-test-shard.sh serves '$script_shards'" >&2
  failures=$((failures + 1))
fi

if ((failures > 0)); then
  echo "$failures ci-changes test(s) failed" >&2
  exit 1
fi
echo "ci-changes tests passed"
