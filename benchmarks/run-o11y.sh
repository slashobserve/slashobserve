#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: benchmarks/run-o11y.sh <o11y-bench checkout> --model <provider>/<model> (--subset | --all | --task <id> ...)
         [--attempts <n>] [--run-max-cost-usd <usd>] [--spend-cap-usd <usd> --ledger <file>]
         [--n-concurrent <n>] [--effort <effort>] [--image <tag>] [--jobs-dir <dir>] [--job-name <name>]

The model key is read from the file named by SLASHOBSERVE_BENCH_LLM_KEY_FILE, which also feeds the judge.
--model replay/replay needs no key: it writes replay fixtures for the tasks and turns verification off.
EOF
  exit 2
}

[ $# -ge 1 ] || usage
o11y_dir="$1"
shift
[ -d "$o11y_dir" ] || { echo "no o11y-bench checkout at $o11y_dir" >&2; exit 2; }
o11y_dir="$(cd "$o11y_dir" && pwd)"
bench_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

model=""
scope=""
tasks=()
attempts=3
run_max_cost=""
spend_cap=""
ledger=""
concurrent=2
effort=""
image="${SLASHOBSERVE_SERVER_IMAGE:-slashobserve-server:local}"
jobs_dir="$bench_dir/jobs"
job=""

value() {
  [ -n "${2:-}" ] || { echo "$1 needs a value" >&2; exit 2; }
}

while [ $# -gt 0 ]; do
  case "$1" in
    --model) value "$1" "${2:-}"; model="$2"; shift 2 ;;
    --subset) scope="subset"; shift ;;
    --all) scope="all"; shift ;;
    --task) value "$1" "${2:-}"; scope="tasks"; tasks+=("$2"); shift 2 ;;
    --attempts) value "$1" "${2:-}"; attempts="$2"; shift 2 ;;
    --run-max-cost-usd) value "$1" "${2:-}"; run_max_cost="$2"; shift 2 ;;
    --spend-cap-usd) value "$1" "${2:-}"; spend_cap="$2"; shift 2 ;;
    --ledger) value "$1" "${2:-}"; ledger="$2"; shift 2 ;;
    --n-concurrent) value "$1" "${2:-}"; concurrent="$2"; shift 2 ;;
    --effort) value "$1" "${2:-}"; effort="$2"; shift 2 ;;
    --image) value "$1" "${2:-}"; image="$2"; shift 2 ;;
    --jobs-dir) value "$1" "${2:-}"; jobs_dir="$2"; shift 2 ;;
    --job-name) value "$1" "${2:-}"; job="$2"; shift 2 ;;
    -h | --help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

case "$model" in
  */?*) ;;
  *) echo "--model must be <provider>/<model>, for example anthropic/claude-haiku-5-5" >&2; exit 2 ;;
esac
[ -n "$scope" ] || { echo "pick the tasks with --subset, --all or --task" >&2; exit 2; }
for number in "$attempts" "$concurrent"; do
  [[ "$number" =~ ^[1-9][0-9]*$ ]] || { echo "--attempts and --n-concurrent take a positive whole number" >&2; exit 2; }
done
if { [ -n "$spend_cap" ] && [ -z "$ledger" ]; } || { [ -z "$spend_cap" ] && [ -n "$ledger" ]; }; then
  echo "--spend-cap-usd and --ledger go together" >&2
  exit 2
fi

replay=false
[ "$model" = "replay/replay" ] && replay=true
if ! $replay; then
  [ -n "$spend_cap" ] || { echo "a real model needs --spend-cap-usd and --ledger" >&2; exit 2; }
  key_file="${SLASHOBSERVE_BENCH_LLM_KEY_FILE:-}"
  [ -n "$key_file" ] && [ -s "$key_file" ] || { echo "set SLASHOBSERVE_BENCH_LLM_KEY_FILE to a file that holds the model key" >&2; exit 2; }
  export SLASHOBSERVE_BENCH_LLM_KEY_FILE="$key_file"
  ANTHROPIC_API_KEY="$(tr -d '\r\n' < "$key_file")"
  export ANTHROPIC_API_KEY
fi
mkdir -p "$jobs_dir"
jobs_dir="$(cd "$jobs_dir" && pwd)"
if [ -n "$ledger" ]; then
  mkdir -p "$(dirname "$ledger")"
  ledger="$(cd "$(dirname "$ledger")" && pwd)/$(basename "$ledger")"
fi

: "${O11Y_SCENARIO_TIME_ISO:=$(date -u +%Y-%m-%dT%H:%M:00Z)}"
export O11Y_SCENARIO_TIME_ISO
[ -n "$job" ] || job="o11y-${model//\//-}-$scope-$(date -u +%Y%m%d-%H%M%S)"

(cd "$o11y_dir" && mise run setup:sync && mise run setup:preflight)

uv_bench=(uv run --locked --project "$bench_dir")
case "$scope" in
  subset) read -r -a include <<< "$("${uv_bench[@]}" python -m slashobserve_bench.o11y include-flags --tasks "$o11y_dir/tasks" --subset)" ;;
  all) read -r -a include <<< "$("${uv_bench[@]}" python -m slashobserve_bench.o11y include-flags --tasks "$o11y_dir/tasks")" ;;
  tasks)
    include=()
    for task in "${tasks[@]}"; do
      [ -f "$o11y_dir/tasks/$task/task.toml" ] || { echo "no task $task in $o11y_dir/tasks" >&2; exit 2; }
      include+=(--include-task-name "$task")
    done
    ;;
esac

args=(
  --path "$o11y_dir/tasks" "${include[@]}"
  --agent slashobserve_bench.agent:SlashObserveAgent
  --model "$model"
  --ak "server_image=$image"
  --n-attempts "$attempts" --n-concurrent "$concurrent"
  --jobs-dir "$jobs_dir" --job-name "$job"
)
[ -n "$run_max_cost" ] && args+=(--ak "run_max_cost_usd=$run_max_cost")
[ -n "$spend_cap" ] && args+=(--ak "spend_cap_usd=$spend_cap" --ak "ledger_path=$ledger")
[ -n "$effort" ] && args+=(--ak "llm_effort=$effort")

if $replay; then
  fixtures="$(mktemp -d)"
  chmod 755 "$fixtures"
  trap 'rm -rf "$fixtures"' EXIT
  for ((i = 1; i < ${#include[@]}; i += 2)); do
    "${uv_bench[@]}" python -m slashobserve_bench.replay --tasks "$o11y_dir/tasks" --task "${include[$i]}" --out "$fixtures"
  done
  args+=(--ak "replay_dir=$fixtures" --disable-verification --verifier-env ANTHROPIC_API_KEY=unused)
fi

echo "scenario time: $O11Y_SCENARIO_TIME_ISO, tasks: $((${#include[@]} / 2)), attempts: $attempts, concurrency: $concurrent"
status=0
(cd "$bench_dir/.." && "${uv_bench[@]}" harbor run --yes "${args[@]}") || status=$?
echo "job folder: $jobs_dir/$job"
exit "$status"
