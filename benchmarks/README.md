# SlashObserve benchmarks

This folder holds the Harbor adapter that runs the SlashObserve Observe Agent on benchmark tasks.
It is not part of the server image and CI never calls a model from it.
`make test-benchmarks` runs its unit tests, which need no Docker, no network and no key, once in this folder's environment and once in `orca/`.

The first benchmark is o11y-bench.
Each benchmark gets one module under `slashobserve_bench/`; `o11y.py` is the o11y-bench one, `orca.py` the ORCA-bench one and `sregym.py` the SREGym one, and the last two have their own sections below.

## What one trial does

Harbor starts the task, and `slashobserve_bench.agent:SlashObserveAgent` does the rest on the host:

1. `setup` refuses a skipped task, checks the job's spend ledger, and pulls the server and Postgres images when they are missing.
2. `run` reads the store URLs (`PROMETHEUS_URL`, `LOKI_URL`, `TEMPO_URL`) and `O11Y_SCENARIO_TIME_ISO` from the task's `main` container.
   A Grafana task also reads `GRAFANA_URL` and the task's Grafana state; see [Grafana tasks](#grafana-tasks).
3. It starts a throwaway Postgres and the SlashObserve server image on the Docker network `o11y-bench-shared`, with a generated Admin service-account key and a datasources file that points at the task's stores (uids `prometheus`, `loki` and `tempo`).
   The server's port is published on a free port of `127.0.0.1`, and the adapter waits for `/readyz`.
4. It starts a run with the task statement as the prompt and `as_of` set to the scenario time, so every relative time the agent resolves is relative to where the task's data ends.
   An `investigation` task is a `kind: investigate` run.
   A `prometheus_query`, `loki_query` or `tempo_query` task is a `kind: ask` run on a new throwaway observation, because the server accepts `as_of` only for an investigation or an ask about a dashboard or an observation.
5. It polls the run until it is `succeeded`, `failed` or `canceled`, or until `wall_timeout_sec` from the start of step 2 passes, which cancels the run, waits up to 30 seconds for it to stop and records a timeout.
6. It writes the result into the trial's `agent/` folder and removes both containers.

The answer is the finding's Markdown text, led by one line per structured field when the finding has them.
A finding from a server built from `agent-rca-accuracy` or later leads with one line per cause, with its confidence band, whether it is stated or unconfirmed, the cause it is a knock-on of, its component, mechanism and trigger; an older server's finding leads with its root cause line.
The bands mean something only with finding grounding on, which the adapter turns on (see [Finding grounding](#finding-grounding)).
o11y-bench grades the last agent message of `agent/trajectory.json`, so the last step of the trajectory is that text with no tool calls.
Full trace ids in the answer are the agent's job, not the adapter's.

| File in `agent/` | Content |
| --- | --- |
| `trajectory.json` | The ATIF trajectory: the task statement, one step per agent tool call as the run's progress steps report it, and the answer. `final_metrics` carries tokens, cache tokens and cost. |
| `detail/run.json` | The run exactly as `GET /api/v1/observe/{id}` returned it: steps, usage, the finding and its evidence. |
| `server.log` | The server's log for the trial. |
| `detail/grafana-imports.json` | Only for a Grafana task: each Grafana dashboard's uid and title, and the SlashObserve dashboard it was imported as or why the import failed. |
| `skipped.txt` | Only for a skipped task: why. |

Harbor also gets `n_input_tokens` (cache included), `n_cache_tokens`, `n_output_tokens` and `cost_usd` on the trial result.
Grading reads only `agent/trajectory.json`, so it can be repeated without the server and without a model.
The o11y-bench grader parses every `.json` file directly under `agent/` as a transcript and fails on one that is not, so every other JSON file the adapter writes goes into `agent/detail/`.

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
It saves the answer a second time, expecting the first save to have been sent back, because an investigation with the loop on sends back a finding that leaves a triage candidate of the preflight sweep unaccounted for; a question ends at the first save.
Every replay fixture marks its second save with `expect` and `is_error: true`, since the replay provider fails a run whose tool result is an error it did not expect.
The replay provider finds a fixture by its prompt, and a `grafana_api` question's prompt is the statement plus the Grafana note, which names the dashboards imported during the trial.
So with `replay_dir` set the trial mounts its own copy of that folder, and once the note is written it adds `<task>-sent.json`, the task's fixture with the prompt the run is sent.

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

To name every task the adapter runs at once, print the flags:

```sh
uv run --project benchmarks python -m slashobserve_bench.o11y include-flags --tasks /path/to/o11y-bench/tasks
```

`include-flags --subset` prints the flags for the fixed ten-task subset in `o11y.SUBSET`: three Prometheus, two Loki, two Tempo and three investigation tasks.
Every development run uses these ten, so results stay comparable; change the list only together with a new baseline.

The judge reads `ANTHROPIC_API_KEY` from the environment of `harbor run`, because each task's `task.toml` passes `${ANTHROPIC_API_KEY}` to the verifier, and it grades with o11y-bench's `GRADING_MODEL`, `claude-haiku-5-5@high` in v1.1.
Export it before the command above, or use the run script below, which does.

### The run script

`run-o11y.sh` wraps the command above for a measured job:

```sh
export SLASHOBSERVE_BENCH_LLM_KEY_FILE=/path/to/anthropic.key
benchmarks/run-o11y.sh /path/to/o11y-bench --model anthropic/claude-haiku-5-5 --subset \
  --attempts 3 --run-max-cost-usd 0.5 --spend-cap-usd 10 --ledger benchmarks/jobs/spend.jsonl \
  --n-concurrent 4
```

| Argument | Default | Meaning |
| --- | --- | --- |
| first argument | required | The o11y-bench checkout. |
| `--model` | required | `<provider>/<model>`, as for Harbor; `replay/replay` runs the replay model. |
| `--subset`, `--all`, `--task <id>` | one is required | The ten tasks of `o11y.SUBSET`, all 64, or the named tasks; `--task` repeats. |
| `--attempts` | 3 | Harbor's `--n-attempts`, the attempts per task. |
| `--run-max-cost-usd` | unset | `--ak run_max_cost_usd`. |
| `--spend-cap-usd`, `--ledger` | required for a real model | `--ak spend_cap_usd` and `--ak ledger_path`. |
| `--n-concurrent` | 2 | Harbor's `--n-concurrent`. |
| `--effort` | unset | `--ak llm_effort`. |
| `--image` | `SLASHOBSERVE_SERVER_IMAGE`, else `slashobserve-server:local` | `--ak server_image`. |
| `--jobs-dir`, `--job-name` | `benchmarks/jobs`, a name from the model, the scope and the time | Where Harbor writes the job. |

The script reads the model key only from the file that `SLASHOBSERVE_BENCH_LLM_KEY_FILE` names, and exports its content as `ANTHROPIC_API_KEY` for the judge without printing it; Harbor masks it in the job's config files.
It sets `O11Y_SCENARIO_TIME_ISO` once for the whole job, to the current minute unless it is already set, runs `mise run setup:sync` and `mise run setup:preflight`, and prints the job folder at the end, also when Harbor fails.
Other loop options, such as `investigation_calls`, keep the adapter's defaults; add them to the Harbor command above when a job needs them.
With `--model replay/replay` it needs no key: it writes a replay fixture for each task and runs with verification off, which checks the arguments and the plumbing of a whole job without a model.

### Options

The adapter cannot read the server's defaults from outside, and they differ between images (see [Server defaults](#server-defaults)), so it does not rely on them.
The loop, its ceiling, the preflight sweep and finding grounding are always sent, with the adapter's defaults when they are not given, and those match the defaults of an image with task types.
`investigation_calls` is sent only when given, because unset it means one limit per task type on an image with task types, which one number cannot express.
A given `--ak` value always wins over the adapter's default.
The results file records under `settings` exactly what was sent, and `server default` for a setting that was not sent, so a job that leaves an option unset agrees with one that sets the adapter's default; a job whose trials disagree on them is refused.

| `--ak` | Default | Meaning |
| --- | --- | --- |
| `server_image` | `ghcr.io/slashobserve/slashobserve:latest` | The server image, with its tag; use a local tag to test a build. |
| `network` | `o11y-bench-shared` | The Docker network of the task's stores. |
| `replay_dir` | unset | A directory of replay fixtures, readable by everyone; needed with `--model replay/replay`. |
| `llm_effort` | server default, not sent | `SLASHOBSERVE_LLM_EFFORT`. |
| `investigation_calls` | server default, not sent | `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`, 6 to 60. |
| `investigation_loop` | `true` | `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP`: `true` or `false`, always sent. |
| `investigation_calls_max` | `40`, or `investigation_calls` when that is higher | `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`, 6 to 60, always sent; the adapter refuses a value below `investigation_calls`, or below 12 when that is unset, because an older image starts at 12 and refuses a lower ceiling. |
| `finding_grounding` | `enforce` | `SLASHOBSERVE_AGENT_FINDING_GROUNDING`: `off`, `report` or `enforce`, always sent. |
| `investigation_brief` | `on` | `SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF`: `on` or `off`, always sent; the preflight sweep of the datasources before an investigation's first model call, which `off` leaves out for a baseline. |
| `run_max_cost_usd` | unset | `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD`: the server fails a run with `budget_exhausted` once its cost reaches it. The server refuses to start with it set when the model has no price, the replay model included. |
| `spend_cap_usd`, `ledger_path` | unset | The job cap and its ledger file, which go together; see below. |
| `wall_timeout_sec` | 540 | How long a trial may take from the start of `run`, server start-up included, under Harbor's 600 second agent timeout. |
| `grafana_url` | unset | Reach Grafana from the host at this address instead of through the task's `main` container. |
| `grafana_user`, `grafana_password` | unset | Grafana basic auth, used only when the task environment carries no Grafana credentials. |
| `grafana_write_sec` | 150 | For a dashboard task, the seconds at the end of `wall_timeout_sec` kept for writing the dashboard to Grafana. |

### Server defaults

An image built from `agent-investigation-core` or later (an image with task types) and an older one, such as main or v0.1.5, have different defaults for an investigation:

| Setting | Image with task types | Older image |
| --- | --- | --- |
| `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` | `true` | `false` |
| `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` | by task type: `measure` 16, `logs` 20, `traces` 20, `incident` 30, `inventory` 12, `explain` 12, and 30 for a question no rule sorts | 12 |
| `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX` | 40, or the calls setting when higher | the calls setting |
| `SLASHOBSERVE_AGENT_FINDING_GROUNDING` | `enforce` | `off` |
| `SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF` | `on` | no sweep, and the setting is ignored |
| `SLASHOBSERVE_LLM_EFFORT` | on `anthropic`, `high` for an investigation and a question about a dashboard or an observation, `medium` for other runs; on `openai`, nothing sent | on `anthropic`, `medium`; on `openai`, nothing sent |

v0.1.5 has none of the loop, ceiling and grounding settings and ignores them when they are sent.
What omitting an option means:

- `investigation_loop`, `investigation_calls_max`, `investigation_brief` and `finding_grounding`: the adapter sends `true`, 40, `on` and `enforce`, so both kinds of image run with them, as far as the image has the setting.
- `investigation_calls`: nothing is sent, so an image with task types gives each investigation its task type's limit and an older image gives 12; the results file records `server default`, and each attempt's `call_limit` is the limit the run ended with when the server returns it.
- `llm_effort`: nothing is sent, so the server picks the effort per run as in the table; the results file records `server default`.

The configuration reference in the repository's `docs/reference/configuration.md` is the source of these values.

### Reproduce the older behaviour

A baseline that runs an image with task types the way an older image ran sets every option the newer defaults changed:

```sh
--ak investigation_calls=12 --ak investigation_loop=false --ak finding_grounding=off --ak investigation_brief=off
```

The SREGym runner takes the same values as `--investigation-calls 12 --investigation-loop false --finding-grounding off --investigation-brief off`.
On `anthropic`, add `--ak llm_effort=medium` (`--llm-effort medium` for SREGym) to keep the older effort too, because unset an investigation now runs at `high`.
Task types still sort the question and narrow its tools on such an image, since no setting turns them off, so the baseline is the older limits and loop, not the older server.
Finding grounding `off` also leaves a finding's causes unscored, so the ORCA-bench report and the SREGym diagnosis name every cause as stated (see [Finding grounding](#finding-grounding)).

### Finding grounding

The server scores each cause of a finding and decides whether the finding states it only with `SLASHOBSERVE_AGENT_FINDING_GROUNDING` set to `report` or `enforce`.
With `off` every cause counts as stated, so the ORCA-bench report and the SREGym diagnosis would name every guess as a cause, and both benchmarks take points for an extra cause.
The server default is `off` on images before the change that makes it `enforce`, so the adapters do not rely on it: every adapter sends `enforce` unless `finding_grounding` is given, and a run behaves the same on any image that has the setting.
`enforce` also sends a finding back once when an item in it does not match what the run read, which is what a person gets from the product's default.
An image older than the setting ignores it, has no causes and no scoring, and its findings are written from `root_cause` or the Markdown.
The results file and the ORCA-bench and SREGym `notes.txt` record the value that was sent.
A Harbor job folder from before the adapter sent a setting recorded it unset, and its trials ran with the image's default; rebuilding a results file from such a folder records the adapter's current default for them, so keep the file written at the time.

## Cost

A real run spends tokens: o11y-bench's published reference runs cost between about $0.02 and $0.30 per trial.
The tasks are 64, 51 of them store tasks, and a full job of three attempts each is 192 trials.
The o11y-bench judge costs extra and runs in the verifier, not here.
A dashboard task makes one or two extra model calls after its run; see [Dashboard tasks](#dashboard-tasks).

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
Each attempt also records, from the trial's `agent/detail/run.json`, the server's choice for the run: `task_type`, read from the step `Routed the question to the <type> task type`; `call_limit`, the investigation's model-call limit when it ended, after any growth; and `stage_at_save`, the loop's stage when the finding was saved.
Each is `null` when the run does not return it: an image without task types writes no routing step, a run that is not an investigation is not sorted, and the other two come only with the loop on.
The reasoning effort a run used is not returned by the server, so the file records only the `llm_effort` that was sent, or `server default`.
A pass is reward 1.0, and the summary adds the leaderboard's Pass^n: the share of tasks whose every attempt passed, for any number of attempts per task.
Every task must have the same number of attempts.

Each failed trial needs a cause and one sentence, which the command cannot judge: read the trial's `agent/trajectory.json` and `verifier/grading_details.json`, then pass `--classifications` a JSON file of `{"<trial name>": {"cause": "<cause>", "summary": "<one sentence>"}}`.
The cause is `adapter_fault`, `missing_capability` or `agent_mistake`.
The command refuses to write while a failure has none, and keeps the classifications of an existing output file when it is run again with more jobs.
Copy no task statement and no benchmark text into the sentence.
It also rewrites the table in `benchmarks/results/README.md`, and `make test-benchmarks` fails when the table is out of date or a file is invalid.
A trial that Harbor reports as skipped (`TaskSkipped` or `agent/skipped.txt`) is listed under `skipped` and counted apart from the scores, and a trial with no `agent/` folder is an errored attempt recorded as an adapter fault unless you classify it otherwise.
A rerun keeps the notes and benchmark version of the existing file unless you pass new ones.

## Check saved answers without a model

`answer_checks.py` reads the `agent/detail/run.json` of every trial in a Harbor job folder and flags answers that break a rule the graders apply, with no model call and no server:

```sh
uv run --project benchmarks python -m slashobserve_bench.answer_checks --job benchmarks/jobs/<job> [--list]
```

| Check | What it flags |
| --- | --- |
| `no_finding` | The run ended without a saved answer. |
| `short_trace_id` | A trace id cut short, where the grader needs all 32 hex characters. |
| `range_for_value` | A range where one value was asked for, such as `120-135 ms`. |
| `hedged_value` | A value behind a hedge, such as `about 20 minutes` or `~4.2%`. |

The summary counts the flagged trials among the failed ones and among the passed ones, so it shows how many recorded failures a rule would have caught and how often it fires on a good answer.
`--list` prints one line per flag with the trial name, its reward and the check.
The checks read only the agent's own answer and copy no task text.
They are text rules, so they cannot see two different figures for one quantity or a wrong number; those need the typed answer the server will check at save time.

## Grafana tasks

The 6 `grafana_api` tasks ask about the task's Grafana, and the 7 `dashboarding` tasks are graded on the dashboard saved in it.
The agent works on SlashObserve dashboards in both, as it does for any user; there is no Grafana converter in the product or the adapter.

### Reading Grafana

Before the server starts, the adapter reads Grafana's datasource list and every dashboard that Grafana's search returns.
It calls Grafana's HTTP API with `curl` inside the task's `main` container, because `GRAFANA_URL` names a host on the task's Docker network, and the adapter runs on the host.
`--ak grafana_url=<address>` calls Grafana from the host instead, for a Grafana whose port is published there.
The credentials come from the task environment: a token in `GRAFANA_SERVICE_ACCOUNT_TOKEN`, `GRAFANA_API_KEY` or `GRAFANA_TOKEN`, else a user and password in `GRAFANA_USER` (or `GRAFANA_USERNAME`, `GRAFANA_ADMIN_USER`) and `GRAFANA_PASSWORD` (or `GRAFANA_ADMIN_PASSWORD`).
Without them the adapter uses `--ak grafana_user` and `--ak grafana_password`, which have no default, and without those it sends no credentials.
o11y-bench v1.1 needs none of them: its Grafana lets anonymous users in as Admin with basic auth off, its task environment carries no Grafana credentials, and the `main` image installs `curl`, so the adapter calls Grafana through `curl` with no `Authorization` header.
Replay trials read the datasources and dashboards and imported them this way; the write of a dashboard task takes the same path but needs a real model, so it has not run yet.
Leave `grafana_user` and `grafana_password` unset for it.
The token or password reaches `curl` through the environment of the command, never its arguments, and is never written to the trial's folder.
The datasources file lists each Grafana datasource's uid and name under `grafanaUids` of the store of the same type, so the imports match them.
Each dashboard is then imported into the throwaway server through `POST /api/v1/imports`, and `agent/detail/grafana-imports.json` records the result.

### Grafana API questions

The question is a `kind: ask` run on a throwaway observation, like a store question, with a note after the task statement.
The note lists the Grafana datasources (name, type, uid, id, URL, access and whether it is the default) and the dashboards (title, uid, folder and tags, and the SlashObserve dashboard each was imported as).
It is cut short so that the prompt stays within the server's 4000 characters.
The answer is the finding's text, as for the other questions.

### Dashboard tasks

The run gets the task statement as its prompt and no `as_of`, because the server refuses `as_of` for a build or a revision.
When the statement names the title of an imported dashboard, the run is a `kind: revise` run on it; otherwise it is a build of a new dashboard.
A build that answers that an existing dashboard should change, with `ask_on` on its finding, is followed by a revision of that dashboard with the same prompt, and the first run is kept as `agent/detail/run-build.json`.
The last `grafana_write_sec` seconds of `wall_timeout_sec` are kept for the step after the run.

After the run, the adapter compares the Spec the run started from (none for a build) with the one it saved.
A panel or variable counts as changed when anything but its position differs.
A row the run added is left out: the agent groups a build under row headings of its own, and a task that names its panels counts a row as one more panel.
The adapter then makes one extra model call, with the trial's provider, model and key, outside the server.
The call gets the Grafana dashboard JSON as fetched (none for a new dashboard), the Grafana datasources, the map from each SlashObserve datasource to a Grafana one, and the added, changed and removed panels and variables, with the Spec before and after and the current Grafana JSON of each changed part.
It asks for those parts only, as Grafana JSON.
The Spec has no annotations, so the call also gets the task statement and writes an annotation only when the statement asks for one.
The adapter places the parts into the fetched JSON itself: an edited panel keeps its Grafana id and position, a new panel gets the next free id below the others, a removed panel or variable leaves, and every other panel stays exactly as fetched.
A part for a panel or variable the run did not change is ignored.
It saves the dashboard with `POST /api/dashboards/db` in its folder, reads it back, and runs each new or changed panel's queries once through `POST /api/ds/query` over the 6 hours before the scenario time, with each variable at its current value.
A query that fails, a reply that cannot be read or a changed part the reply left out goes back to the model once, and the second reply is placed, saved and checked the same way, with no further retry.
The read-back check runs each query with one value per variable, so it cannot see a wrong variable shape or a query that fails only with many or All values.
The replay model writes nothing to Grafana.

The answer of a dashboard task describes the Grafana dashboard, not the SlashObserve one the run worked on, so the answer the grader reads is written by the adapter from the dashboard as read back from Grafana, with no model call: the dashboard's title and uid, the panels added and changed with their type and queries, the panels removed, the panels left exactly as fetched, the variables and annotations added, changed and removed, and the names of the other settings that differ.
A finding the run also saved is left out of the answer, because it describes the SlashObserve dashboard, where an annotation is a stored point and not a query; it stays in `detail/run.json`.

The call's tokens and cost are added to the trial's usage and to `final_metrics` of the trajectory, and its cost is added to the spend ledger.
The cost is priced from the server's price table, `internal/llm/pricing/prices.yaml`, read from this checkout.
The key is passed only in the request header and is never printed, logged or written.

| File in `agent/` | Content |
| --- | --- |
| `detail/grafana-write.json` | The dashboard written, the parts placed, missing and ignored, each attempt's failed queries, and the call's usage. |
| `detail/grafana-call-<n>-request.json` | The body of model call `n`: the model, the system text and the messages. |
| `detail/grafana-call-<n>-response.json` | The provider's response to call `n`. |
| `detail/run-build.json` | Only when a build was followed by a revision: the build. |

### The skip list

`o11y.SKIPPED` is empty, so every task runs and `include-flags` names all of them.
`python -m slashobserve_bench.o11y skipped` lists any task put back on it with the reason.
If such a task is run anyway, `setup` raises before any container starts, writes `agent/skipped.txt` and Harbor records an exception with the reason and no reward.

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
2. `setup` pulls the server and Postgres images, and fails when the code server image is not on the host (see [Source code](#source-code)).
3. `run` waits for `/tmp/env-ready`, reads the store URLs from `/tmp/env-ports`, and finds the trial's network by inspecting the main container from the host.
4. It copies `/app/opentelemetry-demo` out of the main container and starts the code server over that copy on the trial's network, then a throwaway Postgres and the server image on the same network, beside the main container and not inside it.
   The datasources are `prometheus`, `logs` (OpenSearch `otel-logs-*`) and `traces` (OpenSearch `jaeger-main-jaeger-span-*`).
   The trial's Jaeger is 2.12.0, and the Jaeger datasource needs 2.21 or later, so spans are read through OpenSearch (see [Spans](#spans)).
5. It starts an `investigate` run with the task text and a short note as the question, and `as_of` set from `metadata.current` in the task's `task.toml`, or from "The current time is ... ET" in the task text when the metadata lacks it.
   The data is from April 2026 and the clocks are real, so the note gives the current time in UTC and says the task's times are US Eastern.
   It names the datasources, and with a span index it says that `traces` answers the trace tools and asks the agent to follow a failing request with them from its top operation down to the deepest failing span, quoting trace ids in full.
   When the run has source code, the note also says where to read it and the feature flag definitions, that a fault may have been switched on by a flag, and that a flag is named by its key exactly as the source spells it.
   The question, notes included, must fit the server's 4000 character prompt limit; a longer one fails the trial before any container starts.
6. It writes `/app/report.md` from the run's finding, in the four sections the benchmark asks for: Summary, Timeline, 5 Whys and Remediation.
   The report is built from the finding's `causes` when the server returns them (see [Causes in the report](#causes-in-the-report)), from `root_cause`, `onset`, `impact`, `ruled_out` and `next_steps` on an older server, and from the Markdown when neither is there, with every time written in UTC.
   The full Markdown stays under 5 Whys, because it holds the analysis the judge reads.
   A finding with `no_incident` writes an empty file, which is how the benchmark spells "no incident", and so does a finding whose causes are all unconfirmed and whose Markdown is empty.
7. A run that ends without a finding raises, so Harbor records an exception and skips the verifier; an empty report would pass a quiet control by default.

| File in `agent/` | Content |
| --- | --- |
| `report.md` | The report that was written to `/app/report.md`. |
| `trajectory.json`, `run.json`, `server.log` | As for o11y-bench. |
| `codeserver.log` | The code server's log, when the trial started one. |
| `notes.txt` | How traces were read, the finding grounding and loop settings that were sent, which code context applied, and the judge caveat. |

The options are those of o11y-bench without `network`, plus `code_context`, `codeserver_image` (default `slashobserve-codeserver:local`), `source_names` (`mcp`, the default, or `context`) and `span_index` (`true`, the default, or `false`), and `wall_timeout_sec` defaults to 3300 under the task's 3600 second agent timeout.
Leaderboard rules forbid a timeout or resource override, so leave Harbor's at their defaults.

### Causes in the report

ORCA-bench grades a report against every plausible cause of the task, takes points for a cause the report invents, and wants an onset close to the fault's start, so the report keeps what the server confirmed apart from what it did not.

- A stated cause that is not a knock-on is a root cause, numbered when there is more than one, named with its component (the name, its kind and its namespace when given), its mechanism, and its trigger key exactly as stored, with the value when there is one, so a flag key appears verbatim.
- A knock-on is written as a consequence of the root cause it follows, never as a root cause of its own.
- A cause with `stated: false` is listed under Unexplored branches in 5 Whys as not confirmed, beside the `ruled_out` lines, and never in a root cause line, the Summary or the Timeline.
- The Timeline has one line per root cause and per knock-on with an onset, earliest first, all in UTC, and the impact at the earliest onset; causes without an onset take the finding's `onset`.
- Under 5 Whys each root cause and knock-on has its onset, the components it affects, and its evidence: every query from `evidence.queries` verbatim and every trace id in full.
- With no stated cause, the Markdown fills the sections as for an older server, and the unconfirmed causes still go under Unexplored branches.

The server sends `causes` from branch `agent-rca-accuracy` on, so build the image under test from that branch or later.
It scores and bands the causes only with finding grounding on, which the adapter sends as `enforce` unless told otherwise (see [Finding grounding](#finding-grounding)); with `off` every cause counts as stated, so an unsupported guess reaches the report as a root cause.

### Spans

The `traces` entry of the datasources file sets `spans: jaeger` and `timeField: startTimeMillis`, which makes the span index a trace source.
`get_trace`, `trace_stats`, `compare_traces`, `trace_error_origins` and `trace_slow_path` then answer on it, and the question tells the agent to use them.
`logs_for_trace` reads a trace's log lines from `logs`, whose trace id and span id fields the server finds by name.

The server image under test must understand that field, so it must be built from `agent-rca-accuracy` or later.
The datasources file rejects unknown fields, so an older image refuses `spans` and does not start; the trial then fails, names `--ak span_index=false` and keeps the reason in `agent/server.log`.
With `span_index=false` the entry has no `spans` field, the spans are plain documents with the operation name as the message and `traceID` and `spanID` as the trace and span id roles, the trace tools do not answer, and the question only names the datasource.

Not verified against a real trial:

- That ORCA-bench's collector writes `otel-logs-*` in the OpenSearch exporter's default mapping, with the ids at `traceId` and `spanId`, which the server finds without a `fields` override; read one document's fields with `logs` before trusting `logs_for_trace`.
- That the Jaeger 2.12.0 of the trial writes span documents in the shape the server reads, which was checked against Jaeger 2.21.

### Source code

The task's ground truth is usually a feature flag, and its definition is in the demo source the task mounts at `/app/opentelemetry-demo`, so a trial reads that source by default.
`code_context` picks one of three ways, and `agent/notes.txt` says which one applied:

| `code_context` | What the trial does |
| --- | --- |
| not set (the default) | Starts this repository's read-only code server (`tools/codeserver`) as a container on the trial's network, with the copy of the source mounted read-only at `/code`, and provisions it as the MCP source `orca-code`. |
| `<module>:<function>` | Calls that function with the API client, the trial's Docker network and the local copy of the source, and lets it register a code source of its own. |
| `off` | Runs without source code and without the note about it, for example to measure what code access is worth. |

The code server container is named `slashobserve-bench-code-<hex>`, carries the `slashobserve-bench` label like the server and Postgres, listens on `:8080` under the network alias `codeserver.orca.internal`, and is removed when the trial ends, also when it fails; its log is saved as `agent/codeserver.log`.
The server's sources file lists `orca-code` with `allowPrivate: true` and `approve: [list_dir, read_file, grep]`, and `SLASHOBSERVE_OUTBOUND_DOMAINS` is fixed to `codeserver.orca.internal`.
After the server is ready the trial waits up to 60 seconds for the three tools to be approved and fails with the reason when they are not, so a run never goes on silently without code.

Build the code server image once from the repository root; `codeserver_image` names another tag:

```sh
docker build -f tools/codeserver/Dockerfile -t slashobserve-codeserver:local .
```

The server image under test decides how the sources file is named.
An image built after MCP sources were renamed reads `SLASHOBSERVE_MCP_SOURCES_FILE` with the key `mcpSources`, which is the default.
An image built before the rename, such as one built from `main` or the published image at the time of writing, reads `SLASHOBSERVE_CONTEXT_SOURCES_FILE` with the key `contextSources`; pass `--ak source_names=context` for it.
With the wrong naming the server does not know `orca-code`, and the trial fails and names the other option.

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
The trial starts the code server like any other, so build its image first; `SLASHOBSERVE_CODESERVER_IMAGE`, `SLASHOBSERVE_SOURCE_NAMES` and `SLASHOBSERVE_SPAN_INDEX` set `codeserver_image`, `source_names` and `span_index`, and the fixture's question carries the code and spans notes to match.
Run it by hand on a machine that has the snapshot; it is not part of any test.
The fixture saves a finding with one cause, so it needs a server built from `agent-rca-accuracy` or later; an older one refuses `causes` and the trial ends without a finding.
It saves twice, because with grounding enforced the server sends the first save back once, and the second is accepted.
The replay cause cites nothing the run read, so the server scores it low and marks it unconfirmed, and the report is written from the Markdown with the cause under Unexplored branches.

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

## SREGym

[SREGym](https://github.com/SREGym/SREGym) injects a fault into an application on a live Kubernetes cluster and grades an agent's diagnosis and mitigation.
`sregym.py` is its adapter.
It answers the diagnosis stage only, never the mitigation stage, and never writes to the cluster.

### How it plugs in

SREGym is not a Harbor benchmark: it deploys a problem, starts the agent named in its agent registry and waits for submissions on its conductor API.
Its `--use-external-harness` mode deploys the fault and then shuts the conductor down, so nothing would grade a submission; the adapter is a registered agent instead.
SREGym reads its registry from the file that `SREGYM_AGENT_REGISTRY` names, so no file in the SREGym checkout changes.
`python -m slashobserve_bench.sregym registry` writes that file with one agent, `slashobserve`, which runs `python -m slashobserve_bench.sregym run` in this folder's environment on the host, with `container_isolation: false`.
SREGym refuses an agent without container isolation under its default filtered network, so the run needs `--internet-access open`.
The file replaces SREGym's `agents.yaml`, so keep SREGym's default `api` judge backend, which needs no agent from it.
`run` reads what SREGym exports: `API_HOSTNAME` and `API_PORT` for the conductor, `MCP_SERVER_PORT`, `AGENT_MODEL_ID` (SREGym's `--model`) and `AGENT_LOGS_DIR`, the attempt's folder that SREGym publishes beside its results.

### What one run does

1. It reads the conductor's `GET /get_app` (app name, namespaces, description and stages) and waits on `GET /status` until the stage is `diagnosis`.
2. It starts `kubectl port-forward` to the Services `prometheus-server` (port 80) and `loki` (port 3100) in the namespace `observe`, on free host ports bound to every interface, as SREGym does for its MCP server.
   The kubeconfig is the one SREGym gives the agent, which goes through SREGym's filtering proxy.
   `--prometheus-url` and `--loki-url` skip a port-forward; Prometheus is also a NodePort (32000) on the cluster's nodes.
3. It creates a Docker network for the run and starts a throwaway Postgres and the server image on it, as for o11y-bench, with `host.docker.internal` mapped to the host.
   The datasources are `prometheus` and `loki`.
   SREGym's Jaeger is 1.57, in memory, and the Jaeger datasource needs 2.21 or later, so the run has no trace datasource.
4. The server's MCP sources file registers `sregym-kubectl`, SREGym's kubectl MCP server at `http://host.docker.internal:<MCP_SERVER_PORT>/kubectl/sse`, with `allowPrivate: true` and `approve: [exec_kubectl_cmd_safely]` only.
   `rollback_command` and `get_previous_rollbackable_cmd` stay unapproved, so the agent never sees them.
   `SLASHOBSERVE_OUTBOUND_DOMAINS` is `host.docker.internal`, so the server may call nothing else.
   The image must be built from branch `agent-rca-accuracy` or later, because only from there does the server speak SSE to an MCP source and approve, from the `approve` list, a tool its server does not mark read-only.
   SREGym's kubectl server sets no MCP tool annotations, so an older image never lists the tool or keeps it `refused`.
5. It waits up to `--kubectl-wait-sec` (120) for the server's discovery to approve the tool, and writes in `notes.txt` whether the tools listing marks it read-only.
   When the tool is not approved, it stops before any run with an error that says the image is too old, unless `--without-kubectl` lets the run work from telemetry alone.
6. It starts one `investigate` run with no `as_of`, because the cluster is live.
   The question gives the app name, namespaces and description, then a short playbook of our own: read-only kubectl commands only (get, describe, logs, top and events), what to inspect when telemetry shows only a symptom, and how to fill the finding.
   A long description is cut so that the question stays within the server's 4000 characters.
7. It polls the run until it ends, or until its time runs out: the smaller of `--wall-timeout-sec` (1500) and SREGym's remaining agent time less 360 seconds for the submission, which the conductor may hold for up to 300 seconds.
   A run still going then is cancelled.
8. It renders the diagnosis from the finding and submits it once with `POST /submit` and the stage `diagnosis`.

`exec_kubectl_cmd_safely` runs any kubectl command, and the SlashObserve server serves it because the provisioning file approves it by name, not because it is marked read-only.
Read-only on the cluster is therefore an instruction in the playbook, not something either server enforces.
That is acceptable only because SREGym throws the benchmark cluster away after each run.
The port-forwards read through the API server and create nothing in the cluster.

### The diagnosis

SREGym's judge checks the originating component, the mechanism with its concrete changed value, and the scope, and both an extra suspect and a missed component cost points.
The playbook asks for one entry of `causes` per origin, with the component's kind, name (as `Kind/name` for a Kubernetes kind the finding has no word for, such as a NetworkPolicy) and namespace, the mechanism with the concrete value, and the trigger's key and value, and for a component that only suffers from an origin as that cause's `affected` or as a knock-on.
The renderer writes each stated cause that is not a knock-on as an origin: its kind, name and namespace, its mechanism, its trigger as the key and the value only, and as affected scope its `affected` components and the component and `affected` of each stated knock-on that follows it.
A cause with `stated: false`, and a knock-on of one, is left out entirely, because an extra suspect costs points.
The finding's `impact` follows, and nothing else: `ruled_out`, `next_steps` and the Markdown are left out, because a component they mention could read as another suspect.
On an older server without `causes`, `root_cause.component` (`namespace/Kind/name`) and `root_cause.summary` give the origin and the mechanism.
Without a stated cause or a root cause the Markdown is submitted as it is.
The causes need a server image built from `agent-rca-accuracy` or later, which the kubectl tool needs anyway, and are scored only with finding grounding on, which `--finding-grounding` sets and which is `enforce` when it is not given (see [Finding grounding](#finding-grounding)); with `off` every cause counts as stated.
A finding with `no_incident` submits that no faulty component was found, and a run without a finding submits that no diagnosis was produced, so the attempt is scored as a miss and not lost.
A run that stops before the investigation (no `diagnosis` stage, the tool not approved, a port-forward or the server failing) submits nothing, and SREGym records the attempt as incomplete.

| File in `AGENT_LOGS_DIR` | Content |
| --- | --- |
| `diagnosis.txt` | The text submitted. |
| `submission.json` | The conductor's answer to the submission, or the error. |
| `trajectory.json`, `run.json`, `server.log` | As for o11y-bench; SREGym keeps this `trajectory.json`, because it has no converter for this agent. |
| `notes.txt` | How each store was reached, the finding grounding and loop settings that were sent, whether kubectl was approved and marked read-only, and that no mitigation was submitted. |

The options are those of ORCA-bench without `code_context`, as `--` flags, plus `--conductor-url`, `--mcp-url`, `--prometheus-url`, `--loki-url`, `--kubectl-wait-sec`, `--without-kubectl` and `--out`.
`--without-kubectl` is for a telemetry-only baseline, or for a run on an image older than `agent-rca-accuracy`, which cannot approve the tool.
`--model` defaults to SREGym's `--model`, `--server-image` to `slashobserve-server:local` and `--out` to `AGENT_LOGS_DIR`.
`--model replay/replay` without `--replay-dir` writes a replay fixture for the problem's question from `fixtures/sregym-answer.json`, so a run proves the plumbing with no model.
Like the ORCA-bench fixture it saves one cause twice, for the send-back of enforced grounding, and its cause comes back unconfirmed, so the Markdown is submitted; give SREGym a real `--judge-model`, or its judge leaves the attempt unscored.
The spend ledger, the cost cap and the key file work as for o11y-bench.

### Requirements

- A SREGym checkout with its kind cluster from `kind/setup_kind_cluster.sh`, which needs 8 vCPU and 16 GB for SREGym-Lite, plus `kubectl`, Docker and `uv` on the same Linux host.
- A server image built from branch `agent-rca-accuracy` or later, for example `docker build -t slashobserve-server:rca .` in a checkout of that branch, because the run needs both SSE to an MCP source and the approval of a tool that is not marked read-only.
- `SLASHOBSERVE_BENCH_LLM_KEY_FILE` for the model under test, and the judge's key as SREGym asks for it, such as `ANTHROPIC_API_KEY`.

### Run the subset

The subset is ten of the 17 SREGym-Lite problems whose fault telemetry can see (failed or slow requests, error logs, queue lag), which is where an observability agent starts.
The other seven are cluster-operations faults where a workload never starts or never finishes.
This split comes from the problem names and must be confirmed on a running cluster.
`python -m slashobserve_bench.sregym subset` prints the ten ids, one per line.
SREGym's `--problem` takes one id, so the subset is a loop, started from the root of this repository:

```sh
uv run --locked --project benchmarks python -m slashobserve_bench.sregym registry --out /tmp/slashobserve-agents.yaml -- \
  --server-image slashobserve-server:rca --investigation-calls 30 --run-max-cost-usd 1 \
  --spend-cap-usd 10 --ledger-path "$PWD/benchmarks/jobs/sregym-spend.jsonl"
problems=$(uv run --locked --project benchmarks python -m slashobserve_bench.sregym subset)
export SREGYM_AGENT_REGISTRY=/tmp/slashobserve-agents.yaml
export SLASHOBSERVE_BENCH_LLM_KEY_FILE=/path/to/anthropic.key
export ANTHROPIC_API_KEY=$(cat /path/to/anthropic.key)
cd /path/to/SREGym
for problem in $problems; do
  uv run main.py --problem "$problem" --agent slashobserve --stages diagnosis --internet-access open \
    --model anthropic/claude-sonnet-5-5 --judge-model anthropic/claude-opus-5-5 --n-attempts 3
done
```

`--stages diagnosis` matters: without it SREGym waits for a mitigation that never comes until the agent exits, then records the attempt as incomplete with `mitigation` missing.
With it the attempt has one stage and is `complete` once the judge scores it.
Measure one problem's cost before the loop.

### What SREGym records

SREGym writes `results/<minute>/slashobserve/<problem>/<problem>_slashobserve_results.csv` in its checkout, one row per attempt, and the attempt's folder `run_<n>/` beside it holds the files above.
The diagnosis columns are `Diagnosis.success` (the judge's verdict), `Diagnosis.accuracy` (its checklist score from 0 to 100), `Diagnosis.judgment` and `Diagnosis.reasoning`, with `TTL`, the seconds to the submission.
No mitigation column is written, and no problem is ever mitigated.
`results.py` does not read these files.
It reads Harbor job folders, a pass there is a reward of exactly 1.0, and a judged-correct diagnosis can score below 100, so mapping either column onto a reward would change the result.
Read the success rate from the CSV files, and the tokens, cost, call limit and stage from each `run.json`, whose steps name the task type the server chose.

### What is comparable

The diagnosis success rate on these ten problems compares loosely with the diagnosis column that SREGym publishes, which covers all 17 SREGym-Lite problems or the full benchmark, with a mitigation attempted after each diagnosis.
Our runs never mitigate, so there is no end-to-end number and no place in the published ranking.
SREGym lets each entrant pick the judge; use a strong one at high effort, as its maintainers recommend, and not the model under test.
Read the number as how well the agent finds a root cause when telemetry shows only the symptom.
The runs are private, and no result is submitted to the leaderboard.

### Not verified

The adapter has run only in its unit tests, never against a cluster.
A real run must confirm:

- That `exec_kubectl_cmd_safely` comes back `approved` from the provisioning file on an image built from `agent-rca-accuracy` or later, and that `notes.txt` records it as not marked read-only.
- That a container reaches the host's port-forwards through `host.docker.internal` mapped to `host-gateway`, which depends on the host's firewall.
- That `kubectl port-forward` works through SREGym's filtering proxy, which relays upgraded connections.
- That the Services and ports in `observe` are the ones a run deploys, and that its Prometheus and Loki hold the app's metrics and logs.
- That SSE to SREGym's server works through its port-forward, and that a slow kubectl command finishes within the server's MCP call timeout, because SREGym allows one up to 300 seconds.
- That the judge reads the rendered diagnosis as intended.

### SREGym license

SREGym is MIT licensed.
The adapter reads the app's name, namespaces and description from the conductor at run time, and copies none of SREGym's code, prompts, problem descriptions or judge checklists into this repository.

## First real runs

Each section lists what its adapter has not yet shown against a real trial.
Gathered in the order a person meets them, the first trial of each benchmark must confirm the following.

o11y-bench:

Replay trials on o11y-bench v1.1 with a server built from `agent-rca-accuracy` have run every adapter step for one task of each of the six categories and for the ten-task subset at `--n-concurrent 4`: the store URLs and scenario time, Postgres and the server, the datasources, the Grafana reads through `curl` without credentials, the dashboard imports, `as_of`, the preflight sweep of an investigation (22 reads in 3 to 5 seconds), the task routing, the polling, the trajectory, `run.json` and the removal of the containers.
A replay trial takes about 35 seconds in Harbor, and the four concurrent trials of the subset peaked at about 4 GB of memory in all.
What still needs a real model:

1. That the dashboard write-back saves to Grafana and passes its read-back check, since the replay model writes nothing (see [Dashboard tasks](#dashboard-tasks)).
2. That the judge runs: the verifier installs its packages with `uv` inside the `main` container and calls Anthropic, so the container needs to reach PyPI and the Anthropic API.
3. What one trial costs and how long it takes, before a full job.

ORCA-bench:

1. That the filesystem holding the snapshot cache and the jobs folder has room, since the 100 GB is an estimate from the scripts (see [Disk and memory](#disk-and-memory)).
2. That Harbor reports ten trials for `--subset` (see [Run the subset](#run-the-subset)).
3. That the Jaeger 2.12.0 of the trial writes span documents in the shape the server reads, which was checked against Jaeger 2.21 (see [Spans](#spans)).
4. That the collector writes `otel-logs-*` in the OpenSearch exporter's default mapping, with the ids at `traceId` and `spanId`; read one document's fields with `logs` before trusting `logs_for_trace`.

SREGym:

1. That a container reaches the host's port-forwards through `host.docker.internal` mapped to `host-gateway`, which depends on the host's firewall.
2. That `kubectl port-forward` works through SREGym's filtering proxy.
3. That the Services and ports in `observe` are the ones a run deploys, and that its Prometheus and Loki hold the app's metrics and logs.
4. That SSE to SREGym's kubectl server works through its port-forward.
5. That `exec_kubectl_cmd_safely` comes back `approved` from the provisioning file, and that `notes.txt` records it as not marked read-only.
6. That a slow kubectl command finishes within the server's MCP call timeout, because SREGym allows one up to 300 seconds.
7. What one problem costs, before the loop over the subset.
8. That the judge reads the rendered diagnosis as intended.
9. That the ten problems of the subset are the ones whose fault telemetry can see, a split made from the problem names (see [Run the subset](#run-the-subset-1)).

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
HTTP uses `urllib`, the extra model call of a dashboard task included, Docker and the SREGym port-forwards use `subprocess`, and the ledger lock uses `fcntl`, all from the standard library.
The price table is read with PyYAML, which Harbor already depends on, so it adds no package.
The adapter runs on Linux and macOS only.
