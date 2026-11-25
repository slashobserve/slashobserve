#!/usr/bin/env bash
set -euo pipefail

orca_checkout="${1:?usage: benchmarks/orca/run-orca.sh <ORCA-bench checkout> <harbor run arguments>}"
shift
orca_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bench_dir="$(dirname "$orca_dir")"
image="${SNAPSHOT_IMAGE:-orcabench/sre-otel-snapshot:data-0418-harbor-template-v2}"
cache_root="${SNAPSHOT_CACHE_ROOT:-$HOME/.cache/sre-snapshot-cache}"

args=()
for arg in "$@"; do
  if [ "$arg" = "--subset" ]; then
    flags="$(PYTHONPATH="$bench_dir" uv run --locked --project "$orca_dir" python -m slashobserve_bench.orca include-flags)"
    [ "$(printf '%s\n' "$flags" | wc -l)" -ge 20 ] || { echo "the subset gave fewer than 20 flags" >&2; exit 1; }
    while IFS= read -r flag; do args+=("$flag"); done <<<"$flags"
  else
    args+=("$arg")
  fi
done

"$orca_dir/patch-harbor.sh"

cache_dir="$(cd "$orca_checkout" && ./stage_snapshot_cache.sh --image "$image" --cache-root "$cache_root")"
mounts="$(printf '[{"type":"bind","source":"%s","target":"%s","read_only":true}]' "$cache_dir" "$cache_dir")"
export SNAPSHOT_CACHE_HOST_DIR="$cache_dir"
export PYTHONPATH="$bench_dir${PYTHONPATH:+:$PYTHONPATH}"

exec uv run --locked --project "$orca_dir" harbor run --mounts-json "$mounts" \
  --agent slashobserve_bench.orca_agent:OrcaAgent "${args[@]}"
