# SlashObserve benchmarks

This folder holds the Harbor adapter that runs the SlashObserve Observe Agent on benchmark tasks.
It is not part of the server image and CI never calls a model from it.
`make test-benchmarks` runs its unit tests, which need no Docker, no network and no key, once in this folder's environment and once in `orca/`.

The first benchmark is o11y-bench.
Each benchmark gets one module under `slashobserve_bench/`; `o11y.py` is the o11y-bench one and `orca.py` the ORCA-bench one, which has its own section below.

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

The three loop settings are sent to the server only when given, so one image can be measured with the loop off and then on.
The results file records the settings under `settings` with the values the server ran with (loop `false`, `investigation_calls_max` equal to the effective calls and grounding `off` when unset), so a job that leaves them unset agrees with one that sets the defaults; a job whose trials disagree on them is refused.

| `--ak` | Default | Meaning |
| --- | --- | --- |
| `server_image` | `ghcr.io/slashobserve/slashobserve:latest` | The server image, with its tag; use a local tag to test a build. |
| `network` | `o11y-bench-shared` | The Docker network of the task's stores. |
| `replay_dir` | unset | A directory of replay fixtures, readable by everyone; needed with `--model replay/replay`. |
| `llm_effort` | server default | `SLASHOBSERVE_LLM_EFFORT`. |
| `investigation_calls` | server default (12) | `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`, 6 to 60. |
| `investigation_loop` | server default (`false`) | `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP`: `true` or `false`. |
| `investigation_calls_max` | server default (the value of `investigation_calls`) | `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`, 6 to 60; the adapter refuses a value below the effective `investigation_calls` (the given value or 12), as the server does. |
| `finding_grounding` | server default (`off`) | `SLASHOBSERVE_AGENT_FINDING_GROUNDING`: `off`, `report` or `enforce`. |
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

## Keep a run's results

Every measured run is kept as one file under `benchmarks/results/`, written from the run's Harbor job folder or folders (a run split over several jobs takes each as `--job`):

```sh
uv run --project benchmarks python -m slashobserve_bench.results write \
  --job benchmarks/jobs/<job> [--job benchmarks/jobs/<other job>] \
  --benchmark o11y-bench [--benchmark-version <o11y-bench commit>] \
  --classifications <file> [--note "..."] \
  --out benchmarks/results/<benchmark>-<scope>-<server version>-<date>.json
```

The file holds the benchmark and its version, the server build, model, judge and settings, and per task and attempt the reward, pass or fail, tokens, cost and time.
A pass is reward 1.0, and the summary adds the leaderboard's Pass^n: the share of tasks whose every attempt passed, for any number of attempts per task.
Every task must have the same number of attempts.

Each failed trial needs a cause and one sentence, which the command cannot judge: read the trial's `agent/trajectory.json` and `verifier/grading_details.json`, then pass `--classifications` a JSON file of `{"<trial name>": {"cause": "<cause>", "summary": "<one sentence>"}}`.
The cause is `adapter_fault`, `missing_capability` or `agent_mistake`.
The command refuses to write while a failure has none, and keeps the classifications of an existing output file when it is run again with more jobs.
Copy no task statement and no benchmark text into the sentence.
It also rewrites the table in `benchmarks/results/README.md`, and `make test-benchmarks` fails when the table is out of date or a file is invalid.
A trial that Harbor reports as skipped (`TaskSkipped` or `agent/skipped.txt`) is listed under `skipped` and counted apart from the scores, and a trial with no `agent/` folder is an errored attempt recorded as an adapter fault unless you classify it otherwise.
A rerun keeps the notes and benchmark version of the existing file unless you pass new ones.

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

## ORCA-bench

[ORCA-bench](https://github.com/ORCA-bench/ORCA-bench) asks an agent for a root cause report on recorded OpenTelemetry Astronomy Shop telemetry.
Its code is Apache-2.0; no licence was found for its dataset, so this repository holds only the hashed names of ten tasks, never their text or answers.

It runs on Harbor 0.20.0 with a patched `Task.checksum`, while o11y-bench pins Harbor 0.24.0, so it has its own environment in `benchmarks/orca/` (`pyproject.toml` and `uv.lock`, Harbor 0.20.0 pinned).
`orca.py` holds the logic that needs no Harbor (the report writer, the time handling, the datasources, the subset) and `orca_agent.py` is the Harbor agent; it imports from this folder through `PYTHONPATH`, which `orca/run-orca.sh` sets.
`orca/patch-harbor.sh` applies ORCA-bench's one-line patch to the installed Harbor and is safe to run twice.

### What one trial does

1. Harbor starts the task's main container, whose entrypoint starts the OpenTelemetry Demo 2.2.0 (Prometheus, Jaeger and OpenSearch) on a per-trial Docker network `otel-<hash>_default` through the host's Docker socket and restores the telemetry.
2. `setup` pulls the server and Postgres images.
3. `run` waits for `/tmp/env-ready`, reads the store URLs from `/tmp/env-ports`, and finds the trial's network by inspecting the main container from the host.
4. It starts a throwaway Postgres and the server image on that network, beside the main container and not inside it.
   The datasources are `prometheus`, `logs` (OpenSearch `otel-logs-*`) and `traces` (OpenSearch `jaeger-main-jaeger-span-*`).
   The trial's Jaeger is 2.12.0, and the Jaeger datasource needs 2.21 or later, so spans are read through OpenSearch.
5. It starts an `investigate` run with the task text and a short note as the question, and `as_of` set from `metadata.current` in the task's `task.toml`, or from "The current time is ... ET" in the task text when the metadata lacks it.
   The data is from April 2026 and the clocks are real, so the note gives the current time in UTC and says the task's times are US Eastern.
6. It writes `/app/report.md` from the run's finding, in the four sections the benchmark asks for: Summary, Timeline, 5 Whys and Remediation.
   The structured fields of the finding (`root_cause`, `onset`, `impact`, `ruled_out`, `next_steps`) fill the sections when the server returns them, with every time written in UTC; otherwise the Markdown fills them.
   The full Markdown stays under 5 Whys, because it holds the evidence the judge looks for.
   A finding with `no_incident` writes an empty file, which is how the benchmark spells "no incident".
7. A run that ends without a finding raises, so Harbor records an exception and skips the verifier; an empty report would pass a quiet control by default.

The code context source is a hook: `--ak code_context=<module>:<function>` names a function that is called with the API client, the trial's Docker network and a local copy of `/app/opentelemetry-demo`, and registers the source.
Without it the trial runs without code and `agent/notes.txt` says so.

| File in `agent/` | Content |
| --- | --- |
| `report.md` | The report that was written to `/app/report.md`. |
| `trajectory.json`, `run.json`, `server.log` | As for o11y-bench. |
| `notes.txt` | How traces were read, whether a code source was registered, and the judge caveat. |

The options are those of o11y-bench without `network`, plus `code_context`, and `wall_timeout_sec` defaults to 3300 under the task's 3600 second agent timeout.
Leaderboard rules forbid a timeout or resource override, so leave Harbor's at their defaults.

### Disk and memory

The snapshot image is about 50 GB.
`stage_snapshot_cache.sh` pulls it and then extracts `/app` from it into a cache that every trial hardlinks from, so plan for about 100 GB free on the one filesystem that holds the cache and the jobs folder, plus the OpenSearch data each trial restores.
This is an estimate from the scripts, not a measurement.
Each trial runs the whole demo with OpenSearch capped at 2 GB, so run one trial at a time.
The staging script uses `flock`, so run it on Linux.
CI never pulls the snapshot: it runs the unit tests only.

### Run the subset

The subset is ten tasks chosen once from ORCA-bench's own 50-task verified sample, one per question style (exact time, time range, part of a day, a whole day) for each of medium and hard, plus two quiet controls.
Four are medium, four are hard and two are controls.
`--subset` expands to their `--include-task-name` flags.
Confirm that Harbor reports ten trials: a task from the private split has only a `-hidden` variant, which this pattern does not match and which has no answers to judge against.

```sh
export SLASHOBSERVE_BENCH_LLM_KEY_FILE=/path/to/anthropic.key
benchmarks/orca/run-orca.sh /path/to/ORCA-bench --yes \
  -d orca-bench/orca-bench --subset \
  --model anthropic/claude-sonnet-5-5 \
  --ak server_image=slashobserve-server:local \
  --ak investigation_calls=30 --ak run_max_cost_usd=0.5 \
  --ak spend_cap_usd=10 --ak ledger_path=benchmarks/jobs/orca-spend.jsonl \
  --disable-verification --n-concurrent 1 --jobs-dir benchmarks/jobs
```

`SNAPSHOT_IMAGE` and `SNAPSHOT_CACHE_ROOT` override the snapshot image and the cache folder.
The run needs a server built from a checkout that has the structured finding; an older one still gets a report from the Markdown.
Measure one task's cost before the subset.

### Prove the plumbing with the replay model

```sh
benchmarks/run-orca-replay.sh /path/to/ORCA-bench
```

The script downloads one task's package, writes a replay fixture for its question from `fixtures/orca-answer.json`, runs it with `--model replay/replay` and verification off, and prints the path of `agent/report.md`.
No model is called, but the snapshot must already be staged, because the trial needs the demo's data.
Run it by hand on a machine that has the snapshot; it is not part of any test.
The fixture saves a finding with the structured fields, so it needs a server that has them.

### Judge

The leaderboard judge is a GPT model that needs an OpenAI key.
`tests/test.sh` calls `check_prediction.py` with no arguments, and the judge model is chosen only from the host of `OPENAI_BASE_URL`, so the in-trial verifier cannot be pointed at another model by an environment variable.
Run with `--disable-verification` and judge the saved reports afterwards, with a different Claude model than the one under test, through Anthropic's OpenAI-compatible endpoint:
The download runs from the repository root in the `orca/` project, which pins the Harbor that can read the package; the scoring runs from the ORCA-bench checkout.

```sh
uv run --locked --project benchmarks/orca harbor download orca-bench/<task package> -o /path/to/task
export OPENAI_BASE_URL=https://api.anthropic.com/v1/
export OPENAI_API_KEY=$(cat /path/to/anthropic.key)
cd /path/to/ORCA-bench
uv run python harbor-template/tests/check_prediction.py \
  --expected /path/to/task/tests/expected.json --rubrics-dir /path/to/task/tests/rubrics \
  --predictions /path/to/jobs/<job>/<trial>/agent/report.md --model claude-opus-5-5 \
  --reward /tmp/reward.json --details /tmp/details.json
```

The judge calls the Responses API and falls back to Chat Completions on a 404, with a strict JSON schema and high reasoning effort.
Whether Anthropic's endpoint honours those settings was not checked, so score one saved report first and read its details file before trusting a number.
A judge call costs money on the same Anthropic credits as the run.
Scores from a Claude judge on a ten-task subset are our own tracking and are not comparable with published ORCA-bench scores.

### Leaderboard

Do not submit before the maintainers answer whether an agent that runs beside the task's main container, and not inside it, is allowed.
The leaderboard also rejects any run that is not the full dataset with default timeouts.

## License

o11y-bench is AGPL-3.0 and SlashObserve core is Apache-2.0.
The adapter uses o11y-bench as an external dataset: it reads a task's `instruction.md`, `task.toml` and the container environment at run time, and copies none of its code, prompts or task text into this repository.

## Dependencies

| Package | Why |
| --- | --- |
| `harbor==0.24.0` | The base class, capabilities and the ATIF models the adapter and its tests use, pinned to the version o11y-bench pins. |
| `hatchling==1.28.0` | The build backend that makes `slashobserve_bench` importable by Harbor. |
| `pytest==9.0.2` | The unit tests, in the `dev` group. |
| `harbor==0.20.0` | In `orca/` only: the version ORCA-bench runs on. |

`uv.lock` is resolved with `exclude-newer` set so that no package is less than 14 days old; move that date when you update the pins.
HTTP uses `urllib`, Docker uses `subprocess`, and the ledger lock uses `fcntl`, all from the standard library.
The adapter runs on Linux and macOS only.
