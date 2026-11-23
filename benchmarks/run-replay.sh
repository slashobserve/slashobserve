#!/usr/bin/env bash
set -euo pipefail

o11y_dir="${1:?usage: benchmarks/run-replay.sh <o11y-bench checkout> [task id]}"
task="${2:-promql-error-rate}"
bench_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
image="${SLASHOBSERVE_SERVER_IMAGE:-slashobserve-server:local}"
job="replay-${task}-$(date -u +%Y%m%d-%H%M%S)"
fixtures="$(mktemp -d)"
chmod 755 "$fixtures"
trap 'rm -rf "$fixtures"' EXIT

: "${O11Y_SCENARIO_TIME_ISO:=$(date -u +%Y-%m-%dT%H:%M:00Z)}"
export O11Y_SCENARIO_TIME_ISO

(cd "$o11y_dir" && mise run setup:sync && mise run setup:preflight)

uv run --locked --project "$bench_dir" python -m slashobserve_bench.replay \
  --tasks "$o11y_dir/tasks" --task "$task" --out "$fixtures"

uv run --locked --project "$bench_dir" harbor run --yes \
  --path "$o11y_dir/tasks" --include-task-name "$task" \
  --agent slashobserve_bench.agent:SlashObserveAgent --model replay/replay \
  --ak "server_image=$image" --ak "replay_dir=$fixtures" \
  --disable-verification --verifier-env ANTHROPIC_API_KEY=unused --n-concurrent 1 \
  --jobs-dir "$bench_dir/jobs" --job-name "$job"

trajectory="$(ls "$bench_dir"/jobs/"$job"/*/agent/trajectory.json)"
echo "trajectory: $trajectory"
