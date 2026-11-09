#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
linter="$here/docs-lint.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

expect() {
  local name="$1" want="$2" got="$3"
  if [[ "$got" != "$want" ]]; then
    printf 'FAIL  %s\n  want: %q\n  got:  %q\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
}

lines() {
  local count="$1" i
  for ((i = 1; i <= count; i++)); do
    printf 'Line %d is a short sentence.\n' "$i"
  done
}

page() {
  printf -- '---\ntitle: T\ndescription: D\n---\n'
  cat
}

lint() {
  local dir="$work/$1"
  mkdir -p "$dir"
  page >"$dir/page.md"
  rc=0
  out="$(DOCS_DIR="$dir" "$linter" 2>&1)" || rc=$?
  out="${out//$dir\//}"
}

check() {
  local name="$1" want_out="$2" want_rc=0
  [[ -n "$want_out" ]] && want_rc=1
  expect "$name output" "$want_out" "$out"
  expect "$name exit code" "$want_rc" "$rc"
}

lint p8 < <(lines 8)
check "a paragraph of 8 lines passes" ""

lint p9 < <(lines 9)
check "a paragraph of 9 lines is reported at its first line" "page.md:5: paragraph of 9 lines (max 8)"

lint blank < <(lines 8; echo; lines 8)
check "a blank line ends the paragraph" ""

lint heading < <(lines 5; echo '### Part'; lines 5)
check "a heading ends the paragraph" ""

lint fence < <(lines 5; printf '```\ncode\n```\n'; lines 5)
check "a fence ends the paragraph" ""

lint fenced < <(printf '```\n'; lines 20; printf '```\n')
check "fenced lines are not prose" ""

lint table < <(lines 5; printf '| a | b |\n| - | - |\n'; lines 5)
check "a table ends the paragraph" ""

lint list < <(lines 5; printf -- '- item\n'; lines 5)
check "a list item ends the paragraph" ""

lint listcont < <(printf -- '- item\n'; for i in 1 2 3 4 5 6 7 8 9 10; do printf '  continues %d\n' "$i"; done; echo; lines 2)
check "indented list continuation lines are not prose" ""

lint listend < <(printf -- '- item\n'; echo; lines 9)
check "prose after a list and a blank line counts from its first line" "page.md:7: paragraph of 9 lines (max 8)"

lint quote < <(lines 5; printf '> note\n'; lines 5)
check "a blockquote line ends the paragraph" ""

lint two < <(lines 9; echo; lines 10)
check "each long paragraph is reported" "page.md:5: paragraph of 9 lines (max 8)
page.md:15: paragraph of 10 lines (max 8)"

spaced() {
  local count="$1" i
  for ((i = 1; i <= count; i++)); do
    printf 'Line %d is a short sentence.\n\n' "$i"
  done
}

lint wall < <(lines 61)
check "a long page with no section reports both rules" "page.md:5: paragraph of 61 lines (max 8)
page.md:1: no ## heading in a page of 61 lines (max 60)"

lint s60 < <(spaced 60)
check "60 non-blank lines with no ## heading pass" ""

lint s61 < <(spaced 61)
check "61 non-blank lines with no ## heading are reported at line 1" "page.md:1: no ## heading in a page of 61 lines (max 60)"

lint s61h < <(echo '## Part'; spaced 60)
check "a ## heading makes a long page pass" ""

lint s61h3 < <(echo '### Part'; spaced 60)
check "a ### heading is not a section" "page.md:1: no ## heading in a page of 61 lines (max 60)"

lint s61fence < <(printf '```\n## Part\n```\n'; spaced 58)
check "a ## line inside a fence is not a section" "page.md:1: no ## heading in a page of 61 lines (max 60)"

if ((failures > 0)); then
  exit 1
fi
echo "docs lint: ok"
