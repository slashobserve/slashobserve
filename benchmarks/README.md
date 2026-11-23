# SlashObserve benchmarks

This folder holds the Harbor adapter that runs the SlashObserve Observe Agent on benchmark tasks.
It is not part of the server image and CI never calls a model from it.
`make test-benchmarks` runs its unit tests, which need no Docker, no network and no key.

The first benchmark is o11y-bench.
Each benchmark gets one module under `slashobserve_bench/`; `o11y.py` is the o11y-bench one.

## What one trial does

Harbor starts the task, and `slashobserve_bench.agent:SlashObserveAgent` does the rest on the host:

1. `setup` refuses a skipped task, checks the job's spend ledger, and pulls the server and Postgres images when they are missing.
2. `run` reads the store URLs (`PROMETHEUS_URL`, `LOKI_URL`, `TEMPO_URL`) and `O11Y_SCENARIO_TIME_ISO` from the task's `main` container.
3. It starts a throwaway Postgres and the SlashObserve server image on the Docker network `o11y-bench-shared`, with a generated Admin service-account key and a datasources file that points at the task's stores (uids `prometheus`, `loki` and `tempo`).
   The server's port is published on a free port of `127.0.0.1`, and the adapter waits for `/readyz`.
4. It starts a run with the task statement as the prompt and `as_of` set to the scenario time, so every relative time the agent resolves is relative to where the task's data ends.
   An `investigation` task is a `kind: investigate` run.
   A `prometheus_query`, `loki_query` or `tempo_query` task is a `kind: ask` run on a new throwaway observation, because the server accepts `as_of` only for an investigation or an ask about a dashboard or an observation.
5. It polls the run until it is `succeeded`, `failed` or `canceled`, or until `wall_timeout_sec` from the start of step 2 passes, which cancels the run, waits up to 30 seconds for it to stop and records a timeout.
6. It writes the result into the trial's `agent/` folder and removes both containers.

The answer is the finding's Markdown text.
o11y-bench grades the last agent message of `agent/trajectory.json`, so the last step of the trajectory is that text with no tool calls.
Full trace ids in the answer are the agent's job, not the adapter's.

| File in `agent/` | Content |
| --- | --- |
| `trajectory.json` | The ATIF trajectory: the task statement, one step per agent tool call as the run's progress steps report it, and the answer. `final_metrics` carries tokens, cache tokens and cost. |
| `run.json` | The run exactly as `GET /api/v1/observe/{id}` returned it: steps, usage, the finding and its evidence. |
| `server.log` | The server's log for the trial. |
| `skipped.txt` | Only for a skipped task: why. |

Harbor also gets `n_input_tokens` (cache included), `n_cache_tokens`, `n_output_tokens` and `cost_usd` on the trial result.
Grading reads only `agent/trajectory.json`, so it can be repeated without the server and without a model.

## Requirements

- Docker, with the image of the server build under test (`docker build -t slashobserve-server:local .` from a checkout of main or v0.1.5 or later, because `as_of` needs a server built from main or from v0.1.5 on).
- `uv`.
  It installs Python and Harbor 0.24.0, the version o11y-bench pins, into `benchmarks/.venv`.
- A checkout of [grafana/o11y-bench](https://github.com/grafana/o11y-bench) and `mise`, which o11y-bench uses to generate its tasks and build its images.
- For grading, `ANTHROPIC_API_KEY` in the environment: the o11y-bench judge is a model call.

## Run one task with the replay model

```sh
benchmarks/run-replay.sh /path/to/o11y-bench promql-error-rate
```

The script runs `mise run setup:sync` and `mise run setup:preflight` in the o11y-bench checkout, writes a replay fixture for the task's statement, and runs Harbor with `--disable-verification`, so no key and no model is used; Harbor still asks for the judge's `ANTHROPIC_API_KEY`, so the script passes a placeholder with `--verifier-env`.
The replay model answers with a fixed text, so the answer is gradable but wrong; the run proves the plumbing.
It prints the path of the trajectory.
Set `SLASHOBSERVE_SERVER_IMAGE` to use another image than `slashobserve-server:local`.

The fixture is generated because its prompt is the task statement, which belongs to o11y-bench and is not copied into this repository.
`fixtures/store-answer.json` is the template.

## Run a subset with a real model

Pick the tasks, then run Harbor from the repository root:

```sh
export SLASHOBSERVE_BENCH_LLM_KEY_FILE=/path/to/anthropic.key
export O11Y_SCENARIO_TIME_ISO=$(date -u +%Y-%m-%dT%H:%M:00Z)
(cd /path/to/o11y-bench && mise run setup:sync && mise run setup:preflight)

uv run --project benchmarks harbor run --yes \
  --path /path/to/o11y-bench/tasks \
  --include-task-name promql-error-rate --include-task-name incident-triage \
  --agent slashobserve_bench.agent:SlashObserveAgent \
  --model anthropic/claude-haiku-5-5 \
  --ak server_image=slashobserve-server:local \
  --ak investigation_calls=30 \
  --ak run_max_cost_usd=0.5 \
  --ak spend_cap_usd=10 --ak ledger_path=benchmarks/jobs/spend.jsonl \
  --n-concurrent 2 --jobs-dir benchmarks/jobs
```

`--model` is `<provider>/<model>`: the provider is the server's `SLASHOBSERVE_LLM_PROVIDER` and the model its `SLASHOBSERVE_LLM_MODEL`.
The key is read from the file that `SLASHOBSERVE_BENCH_LLM_KEY_FILE` names, passed to the server container through a private env file, and never printed, logged or written to a result.
`--ae SLASHOBSERVE_BENCH_LLM_KEY_FILE=...` works too.

To name every store task at once, print the flags:

```sh
uv run --project benchmarks python -m slashobserve_bench.o11y include-flags --tasks /path/to/o11y-bench/tasks
```

`include-flags --subset` prints the flags for the fixed ten-task subset in `o11y.SUBSET`: three Prometheus, two Loki, two Tempo and three investigation tasks.
Every development run uses these ten, so results stay comparable; change the list only together with a new baseline.

### Options

| `--ak` | Default | Meaning |
| --- | --- | --- |
| `server_image` | `ghcr.io/slashobserve/slashobserve:latest` | The server image, with its tag; use a local tag to test a build. |
| `network` | `o11y-bench-shared` | The Docker network of the task's stores. |
| `replay_dir` | unset | A directory of replay fixtures, readable by everyone; needed with `--model replay/replay`. |
| `llm_effort` | server default | `SLASHOBSERVE_LLM_EFFORT`. |
| `investigation_calls` | server default (12) | `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`, 6 to 60. |
| `run_max_cost_usd` | unset | `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD`: the server fails a run with `budget_exhausted` once its cost reaches it. The server refuses to start with it set when the model has no price, the replay model included. |
| `spend_cap_usd`, `ledger_path` | unset | The job cap and its ledger file, which go together; see below. |
| `wall_timeout_sec` | 540 | How long a trial may take from the start of `run`, server start-up included, under Harbor's 600 second agent timeout. |

## Cost

A real run spends tokens: o11y-bench's published reference runs cost between about $0.02 and $0.30 per trial.
The store tasks are 51, and a full job of three attempts each is 153 trials.
The o11y-bench judge costs extra and runs in the verifier, not here.

`spend_cap_usd` is a cap for the whole Harbor job.
Every trial that started a run appends the run's last seen cost to the ledger file, also when polling fails or Harbor stops the trial.
The file is locked, so concurrent trials never lose a line.
A trial that starts when the total has reached the cap fails in `setup` with a spend cap error, which Harbor records as an agent exception, not as a pass or a fail.
Trials already running are not interrupted, so a job can pass the cap by up to `--n-concurrent` runs; `run_max_cost_usd` bounds each of them.
A run whose cost the server could not price counts as $0.00 in the ledger.

## Skipped tasks

The 7 `dashboarding` and 6 `grafana_api` tasks are graded on Grafana's own state, which this agent neither writes nor reads, so they do not apply.
`python -m slashobserve_bench.o11y skipped` lists them with the reason.
If one is run anyway, `setup` raises before any container starts, writes `agent/skipped.txt` and Harbor records an exception with the reason and no reward.
`include-flags` leaves them out, so a normal run never meets one.
A run over the 51 store tasks is not a leaderboard submission, which needs all 64.

## Cleaning up

The adapter removes its containers when a trial ends, also on a timeout or an error.
After a crash of Harbor itself, remove what is left:

```sh
docker ps -aq --filter label=slashobserve-bench | xargs -r docker rm -f
```

## License

o11y-bench is AGPL-3.0 and SlashObserve core is Apache-2.0.
The adapter uses o11y-bench as an external dataset: it reads a task's `instruction.md`, `task.toml` and the container environment at run time, and copies none of its code, prompts or task text into this repository.

## Dependencies

| Package | Why |
| --- | --- |
| `harbor==0.24.0` | The base class, capabilities and the ATIF models the adapter and its tests use, pinned to the version o11y-bench pins. |
| `hatchling==1.28.0` | The build backend that makes `slashobserve_bench` importable by Harbor. |
| `pytest==9.0.2` | The unit tests, in the `dev` group. |

`uv.lock` is resolved with `exclude-newer` set so that no package is less than 14 days old; move that date when you update the pins.
HTTP uses `urllib`, Docker uses `subprocess`, and the ledger lock uses `fcntl`, all from the standard library.
The adapter runs on Linux and macOS only.
