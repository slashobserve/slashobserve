#!/usr/bin/env bash
set -euo pipefail

orca_checkout="${1:?usage: benchmarks/run-orca-replay.sh <ORCA-bench checkout> [task package, 16 hex characters]}"
bench_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
package="${2:-$(PYTHONPATH="$bench_dir" uv run --locked --project "$bench_dir/orca" python -c 'from slashobserve_bench import orca; print(orca.SUBSET[0].package)')}"
image="${SLASHOBSERVE_SERVER_IMAGE:-slashobserve-server:local}"
job="orca-replay-${package}-$(date -u +%Y%m%d-%H%M%S)"
workdir="$(mktemp -d)"
fixtures="$workdir/fixtures"
mkdir -m 755 "$fixtures"
trap 'rm -rf "$workdir"' EXIT

uv run --locked --project "$bench_dir/orca" harbor download "orca-bench/$package" -o "$workdir/task"
task_dir="$(dirname "$(find "$workdir/task" -name instruction.md -print -quit)")"

PYTHONPATH="$bench_dir" uv run --locked --project "$bench_dir/orca" python -m slashobserve_bench.orca replay-fixture \
  --task-dir "$task_dir" --template "$bench_dir/fixtures/orca-answer.json" --out "$fixtures"
mv "$fixtures/$(basename "$task_dir").json" "$fixtures/$package.json"

"$bench_dir/orca/run-orca.sh" "$orca_checkout" --yes \
  -t "orca-bench/$package" --model replay/replay \
  --ak "server_image=$image" --ak "replay_dir=$fixtures" \
  --ak "codeserver_image=${SLASHOBSERVE_CODESERVER_IMAGE:-slashobserve-codeserver:local}" \
  --ak "source_names=${SLASHOBSERVE_SOURCE_NAMES:-mcp}" \
  --disable-verification --n-concurrent 1 \
  --jobs-dir "$bench_dir/jobs" --job-name "$job"

echo "report: $(ls "$bench_dir"/jobs/"$job"/*/agent/report.md)"
