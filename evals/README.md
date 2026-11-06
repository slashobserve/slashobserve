# Evals

The eval runner (`go run ./evals/runner`) asks a running SlashObserve server to build, change or investigate, then checks what the agent saved.
It runs against scripted model turns (the replay suite, no key, what CI runs) or against a real model (the live suite, which spends tokens).
Every run prints a Markdown report and, with `--out`, writes a JSON report.
This file is the contributor's guide: read it before adding a scenario or changing how a run is scored.

## Layout

| Path | What it holds |
| --- | --- |
| `evals/scenarios/*.yaml` | One scenario per file: a prompt, its kind, its suites and what the result must satisfy. |
| `evals/fixtures/llm/*.json` | Replay fixtures written by hand for the scenarios in the replay suite; `e2e/fixtures/llm/` adds the ones the e2e suite shares. |
| `evals/corpus-sample.yaml` | The corpus dashboards the live suite builds tasks from, and the Kubernetes dashboards of `--profile k8s`. |
| `evals/k8s-gaps.yaml` | The Kubernetes corpus panels known to have no data on the demo cluster. |
| `evals/pricing.yaml` | Model prices, used for the cost of each run. |
| `evals/matrix.example.yaml` | The shape of the git-ignored `evals/matrix.yaml` that `make evals-live` reads. |
| `evals/runner/` | The runner. |
| `internal/taste/` | The layout and field rules the quality score applies, shared with the reference dashboards' test. |

Reports go under `evals/reports/`, which is git-ignored; never commit one.

## Suites

A suite is a name that scenarios and corpus tasks declare, and `--suite <name>` runs everything that declares it.

| Suite | What it is for | What declares it | How it runs |
| --- | --- | --- | --- |
| `replay` | Proving the runner, the server and the agent loop still work, with no model. | Scenarios with a replay fixture, plus every corpus task the replay provider can script. | `make evals-replay`, in CI, with no key. |
| `live` | Measuring a real model: pass rate, quality, latency and cost. | Scenarios for a real model, including those that inject a fault, plus the corpus tasks of the sample. | `make evals-live` with a model matrix, by hand. |
| `smoke` | A quick check of a model, prompt or agent change before a full live run. | Thirteen live tasks: six `create` (three Prometheus, two ClickHouse, one mixed), two scenario revisions, the demo shop `change`, `fix` and `suggest` corpus tasks, `chat-import-small` and `investigate-checkout-errors-on-one-pod`. | `go run ./evals/runner --suite smoke ...` |

`--suite` accepts any suite a scenario or corpus task declares, so a new suite needs no runner change.
Without `--suite`, only the `create` and `question` scenarios of the live suite and those tagged `replay-smoke` run.
List a selection without running it:

```sh
go run ./evals/runner list --suite smoke
```

Corpus tasks are generated from the dashboards under `pkg/grafana/testdata/corpus`: `paste`, `add`, `fix`, `change`, `suggest`, `rebuild`, `retime` and `rowedit` tasks, named `corpus-<task>-<source>-<dashboard>`.
`--corpus all|sample|none` picks which dashboards they come from; replay defaults to all, every other suite to the sample.

## Adding a scenario

Add one file to `evals/scenarios/` with an `id`, a `prompt`, a `kind` and the `suites` that run it; loading fails when `kind` or `suites` is missing.
Then run `go run ./evals/runner list --suite <suite>`, which fails on a file that does not validate.

```yaml
id: red-metrics-by-endpoint
prompt: demo shop RED metrics by endpoint
category: prometheus
kind: create
suites: [live, replay, smoke]
done_when: true
tags: [replay-smoke]
expect:
  datasources: [prometheus]
  metrics: [http_requests_total, http_request_duration_seconds]
  scope: '{job="shop"}'
  units: [reqps, rps, ops, short, s, ms]
  min_panels: 2
```

| Field | Meaning |
| --- | --- |
| `id` | Lowercase letters, digits and dashes, 3 to 64 characters, unique across files and corpus tasks. |
| `prompt` | The text sent to the agent, at most 4000 characters except for `chat_import` and `size_guard`; it may use `{panel_id}`, `{panel_title}` and `{row_title}`, filled from the seed. |
| `kind` | What the run does, from the table below. |
| `suites` | The suites that run it, each a lowercase name such as `live`, `replay` or `smoke`. |
| `category` | `prometheus`, `clickhouse`, `loki`, `tempo` or `mixed`; required for `create`, and it fixes `expect.datasources`. |
| `done_when` | Counts towards `--min-done-when`. |
| `tags` | Free labels for `--tag`; `replay-smoke` also selects the scenario when no `--suite` is given. |
| `seed` | The corpus dashboard a revision starts from: `corpus` (a path under `pkg/grafana/testdata/corpus`), `break_panel`, `mapping`, `target` and `row`. |
| `fault` | A demo shop fault held during the run; see Faults below. |
| `expect` | The checks below. |

Scenario prompt text is part of the measurement: change a prompt only together with a new baseline.
A `loki` or `tempo` scenario names its datasource in the prompt, because ClickHouse holds the same logs and traces.
A live prompt reads like a person's request and does not name the answer, such as the pod a fault sits on.

### Kinds

| Kind | The run | Needs | Checks it always gets |
| --- | --- | --- | --- |
| `create` | Builds a new dashboard from the prompt. | `category` and `expect.datasources`. | `validity`, `data`, `source`, `panels`, `latency` |
| `revise` | Changes a seeded dashboard. | `seed`. | `failure_code`, `keeps_panels`, `new_panel_data` (unless `changed_paths`, `only_target_changed` or `changed_panels` is set), `panel_values`, `latency` |
| `fix_unverified` | Fixes a seeded unverified panel. | `seed`, usually with `break_panel: true`. | `failure_code`, `only_target_changed`, `target_data`, `unverified_cleared`, `latency` |
| `suggest` | Proposes one panel without saving a version. | `seed`. | `failure_code`, `proposal`, `no_version_added`, `latency` |
| `chat_import` | Imports pasted Grafana JSON through the chat. | The JSON in the prompt. | `failure_code`, `imported`, `latency` |
| `size_guard` | Sends a prompt over the size limit, which must fail. | `expect.failure_code: prompt_too_long`. | `failure_code` |
| `investigate` | Asks why something is wrong and reads the saved finding. | `expect.finding_mentions` or `expect.evidence_min`, and no seed or panel expectations. | `failure_code`, `finding_mentions`, `evidence`, `latency` |
| `question` | Asks a question, asks for an explanation, or asks something outside the server's data, with no command; the run must answer with a finding and build nothing. | No seed or panel expectations. | `failure_code`, `answered`, `latency` |

### Faults

A `fault` makes the demo shop misbehave while the run is in flight, so a `revise` or `investigate` scenario can be judged against a known cause.

```yaml
fault:
  pod: checkout-0
  type: errors
  pct: 30
  soak_seconds: 120
```

| Field | Meaning |
| --- | --- |
| `pod` | The shop pod whose fault API the runner calls: `checkout-0` (:28081 on the evals stack) or `checkout-1` (:28082); `--shop-url pod=url` adds or overrides one. |
| `type` | `errors` (that share of requests answer 500 and log `database connection refused`), `latency` (that share of requests wait 0.5 to 1.5 extra seconds), `queue` (that share of the pod's orders queue workers stop, so orders wait) or `slow_db` (that share of the pod's inventory database queries take 300 to 900 ms longer). |
| `pct` | 1 to 100. |
| `soak_seconds` | How long the fault runs before the request is sent, so rates and quantiles over a few minutes show it. |

The runner posts `/fault?type=<type>&pct=<pct>` to the pod, waits the soak, runs the scenario and clears the fault with `DELETE /fault`, even when the run fails.
Fault scenarios run one at a time; other scenarios still run beside them, so a fault scenario should judge only the pod it faults.
A fault scenario belongs in the live suite only: the replay provider scripts the model, so a fault changes nothing it does.
If a run is killed and a fault stays set, clear it with `curl -fsS -X DELETE http://127.0.0.1:28081/fault` and the same on :28082.

The live investigations judge a finding by the words a correct one must contain, a deterministic check rather than a model judge:

| Scenario | Fault | Suites | The finding mentions |
| --- | --- | --- | --- |
| `investigate-checkout-errors-on-one-pod` | `checkout-0` `errors` 30% | `live`, `smoke` | `checkout-0`, `error` |
| `investigate-checkout-latency-on-one-pod` | `checkout-0` `latency` 30% | `live` | `checkout-0`, `latency` |
| `investigate-orders-queue` | `checkout-0` `queue` 100% | `live` | `checkout-0`, `queue` |
| `investigate-inventory-slow-db` | `checkout-0` `slow_db` 50% | `live` | `checkout-0`, `database` |

### Checks

Each check is a pure function of what the run saved; a scenario passes when every check passes.

| Check | `expect` fields | Passes when |
| --- | --- | --- |
| `validity` | | The run succeeded and its saved Spec parses. |
| `data` | | Every data panel returned data. |
| `source` | `datasources` | Panels query exactly these datasource types (`prometheus`, `clickhouse`, `loki`, `tempo`). |
| `metrics` | `metrics`, `rebuild_metric_share`, `rebuild_metric_cap` | A Prometheus query contains each name, or the given share of them. |
| `tables` | `tables` | A ClickHouse query contains each table. |
| `units` | `units` | Some panel uses one of these units. |
| `query` | `query_contains_any` | Each group has one alternative some query contains. |
| `time_range` | `time_range_from` | The dashboard's `time.from` is one of these. |
| `panels` | `min_panels` (default 1), `max_panels` (default 12 for `create`) | The data panel count is in range. |
| `thresholds` | `thresholds`, `threshold_at` | A panel has thresholds, or a threshold at this value for this unit. |
| `coverage` | `scope`, `coverage` (default 0.9) | Queries name the share `coverage` of the metric families in `scope`; see Coverage. |
| `latency` | `max_seconds` (default 60) | The run finished in time. |
| `failure_code` | `failure_code` | The run failed with this code, or succeeded when none is set. |
| `budgets` | `max_model_calls`, `max_output_tokens`, `max_save_output_tokens` | The run stayed within each budget. |
| `usage` | `usage` | Each named counter (`build_saves`, `renewals`, `compactions`, `nudges`, `retries`, `save_now_refusals`, `stalled_turns`, `one_panel_refusals`) has exactly this value. |
| `finding_mentions` | `finding_mentions` | The finding's text contains each entry, ignoring case. |
| `evidence` | `evidence_min` (0 to 4) | The finding cites at least this many evidence panels. |
| `answered` | | The run succeeded with a finding and saved no dashboard or version. |
| `keeps_panels`, `new_panel_data`, `panel_values`, `changed_paths`, `changed_panels`, `only_target_changed`, `target_data`, `target_type`, `unverified_cleared` | `keeps_panels`, `panel_values`, `changed_paths`, `changed_panels`, `only_target_changed`, `target_returns_data`, `target_type`, `unverified_cleared` | The revision kept, changed or fixed what the field names. |

A failure on `latency` or `budgets` alone is reported as `FAIL (latency only)` and still counts as a failure.

### Replay fixtures

A scenario in the replay suite needs a fixture, because the replay provider answers only prompts it has a script for; a live-only scenario needs none.
A fixture is a JSON file in `evals/fixtures/llm/` whose `prompt` matches the scenario's prompt and whose turns script the model: `text`, `tool_calls`, and an optional `fault` (`rate_limited`, `server_error`, `call_timeout`, `max_tokens`, `refusal` or `delay`).
Write scenario fixtures by hand; corpus task fixtures are generated.
`go run ./evals/runner prepare --out <dir>` writes the generated corpus fixtures, copies the hand-written ones from `evals/fixtures/llm/` and `e2e/fixtures/llm/`, and writes the synthetic series; `make evals-stack-up` runs it and mounts the result.
A new fixture is read when the server starts, so restart the stack's server after adding one to a running stack.

## Quality

Every run that saved a dashboard gets a quality score from 0 to 1, computed from the saved Spec alone.
The rules live in `internal/taste`, which also judges the reference dashboards under `deploy/compose/dashboards`, so the agent is held to the bar the shipped dashboards meet.
`taste.Judge` reports, per rule, how many things it checked and which failed; the score weighs each rule's share `1 - failed / checked`, and a rule that checked nothing scores full.

| Rule | Weight | Checks |
| --- | --- | --- |
| `titles` | 0.10 | Every panel except a row has a title. |
| `units` | 0.20 | Every timeseries, stat, gauge and bargauge has a unit. |
| `legends` | 0.15 | Every visible query of a timeseries, bargauge, barchart or piechart has a legend when its panel shows more than one query or the query groups with `by (...)` or `without (...)`; a quantile grouped only by `le` needs none. |
| `descriptions` | 0.10 | Every stat and gauge has a description. |
| `thresholds` | 0.10 | Threshold steps sit only on stat, gauge, bargauge and state-timeline panels. |
| `full_lines` | 0.15 | Each line of panels, at the top level and in each row, fills the 24 columns with no gap, holds 1, 2, 3, 4 or 6 panels and keeps one height. |
| `headline` | 0.10 | The dashboard opens at y 0 with 3, 4 or 6 stat panels of one width. |
| `row_questions` | 0.05 | Every top-level panel below the headline is a row whose title ends with `?`. |
| `lone_panels` | 0.05 | A panel alone on a full-width line is a logs, table, traces, state-timeline, status-history, nodeGraph or text panel, or a timeseries with a table legend or a right legend. |
| `stacking` | 0 | Each line starts where the line above ends. |
| `ids` | 0 | Panel ids start with `p_` and row ids with `r_`. |

`stacking` and `ids` weigh nothing because the server places panels and picks ids in a build.
The weights are the scoring policy and live in `evals/runner/quality.go`; a new rule goes into `internal/taste` and gets a weight there.
Each result carries `quality` and `quality_findings` (the first five failures of each weighted rule), the aggregates carry `quality_median` per kind, and `--min-quality` (default 0, off) fails the run when any kind's median is below it.
Scores from reports made before the rules moved into `internal/taste` are not comparable with later ones.

## Coverage

A `create` scenario whose prompt asks for a whole service or job sets `expect.scope` to a Prometheus series selector, such as `'{job="shop"}'`.
After the run, the runner reads `/api/v1/series?match[]=<scope>` from `--prometheus` (default `http://localhost:29090`, the evals stack) and groups the series names into metric families.
The `_bucket`, `_sum` and `_count` series of one histogram are one family.
`up` and the `scrape_`, `process_`, `go_` and `promhttp_` families are left out unless the scope names them.
The score is the share of families that some panel query names as a whole token; the check passes at `expect.coverage` or above and lists up to 20 families no query names.
A scope that matches no family fails the check.

## Running the replay suite

```sh
make evals-replay
```

It boots the isolated `slashobserve-evals` stack in replay mode (http://localhost:28080, Prometheus on :29090) and runs `--suite replay --bootstrap-dex --min-pass 1`.
Stop the stack with `make evals-stack-down`.

## Running the live suite

The live suite runs once per entry of a model matrix.
Copy `evals/matrix.example.yaml` to `evals/matrix.yaml` and point each entry at a compose env file that holds that provider's settings and key.

```yaml
models:
  - name: haiku
    env_file: deploy/compose/.env.haiku
  - name: other
    env_file: deploy/compose/.env.other
    pricing: { input: 1.00, output: 5.00, cache_read: 0.10, cache_write: 1.25 }
```

Boot the stack on the first entry's env file, then run the matrix:

```sh
make evals-stack-up EVALS_STACK_MODE=deploy/compose/.env.haiku
make evals-live EVALS_MATRIX=evals/matrix.yaml EVALS_REPORT_DIR=evals/reports/live
```

Between entries the runner recreates only the server with the next env file.
It writes `<name>.json`, `<name>.md` and `<name>.server.log` per entry and a `matrix.md` that compares them.

Keep every key in a git-ignored env file under `deploy/compose/` and pass only the file's path.
Never print an env file, echo a key, put one on a command line or paste one into a report or issue.
The runner never needs the model key: the server reads it, and `--bootstrap-dex` mints a service-account key for the run, so `OBSERVE_TOKEN` can stay unset.

## Choosing what runs

| Flag | What it does |
| --- | --- |
| `--suite <name>` | Runs every scenario and corpus task of that suite. |
| `--id <id>` | Runs only this scenario, repeatable; the scenario must be in the selected suite. |
| `--tag <tag>` | Runs only scenarios with this tag, repeatable. |
| `--repeat <n>` | Runs each selected scenario `n` times, 1 to 20 (default 1). |
| `--parallel <n>` | Scenarios at a time, 1 to 3 (default 2). |
| `list` | Prints the selection and exits. |

To rerun only what failed, pass each failed id with `--id` against the running stack:

```sh
go run ./evals/runner --server http://localhost:28080 --suite replay --bootstrap-dex --id fault-compaction
```

To measure how often a live scenario succeeds, repeat it:

```sh
go run ./evals/runner --server http://localhost:28080 --suite live --bootstrap-dex --id investigate-checkout-errors-on-one-pod --repeat 10 --out evals/reports/live/errors.json
```

Runs of one scenario never overlap, and each is its own result with a `run` number.
A fault scenario sets, soaks and clears its fault on every run.
The pass rate, latency, cost, token and done-when figures count every run, and the report adds a table with `k of N` runs passed per scenario.
`--min-done-when` then applies to each repeat, so `--repeat 10` needs ten times as many done-when passes.

## Pricing

`evals/pricing.yaml` gives USD prices per million tokens for input, output, cache reads and cache writes.
A run's model is priced by its exact key first, then by the longest `match` prefix any entry lists, so a dated id is priced too.
A model with no entry, or a run that used a token class whose rate is `null`, has no cost rather than a zero one; the report names such a model once.
A matrix entry's `pricing` replaces the lookup for every run of that entry.

## Comparing reports

```sh
make evals-compare EVALS_BASE=evals/reports/base/haiku.json EVALS_NEW=evals/reports/live/haiku.json
```

The comparison matches runs by scenario id and lists, for every scenario in both reports, the status change and the latency, cost and quality with their delta.
A scenario that ran once in both reports shows its verdict change, such as `PASS -> FAIL`; one that ran more than once in either shows its pass rate in each, such as `9 of 10 (90%) -> 7 of 10 (70%)`, with the median latency, mean cost and median quality of its runs.
Its summary has one row per model with the pass count, total cost, p50 time to first panel and median quality of both runs and their delta.
It prints the comparison and writes `compare.md` next to the new report.
A run can compare its own report as it finishes: add `--compare <base.json>` to a run with `--out`.

## Reports

The JSON report holds the provider, the model, the aggregates and one result per run.
A result carries its checks, status, failure code, latency, usage, `cost_usd`, `quality`, `quality_findings`, `coverage`, the build metrics (time to first panel, panels per minute, calls per panel and tokens per call) and, under `--repeat`, its `run` number.
The aggregates carry pass rates per category and kind, failure codes, latency, cost with `unpriced_models`, tokens, `quality_median`, the time to first panel and, under `--repeat`, `runs` with the passed runs per scenario.
The Markdown report shows the same with one row per run and lists the gates that failed: `--min-pass` (default 0.8), `--min-done-when`, `--min-quality` and any failed run without a failure code.
