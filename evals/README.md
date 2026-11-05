# Evals

The eval runner (`go run ./evals/runner`) asks a running SlashObserve server to build or change dashboards, then checks what the agent saved.
It runs two ways: against scripted model turns (the replay suite, no key, what CI runs) or against a real model (the live suite, which spends tokens).
Every run writes a Markdown report to stdout and, with `--out`, a JSON report.

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

Reports go under `evals/reports/`, which is git-ignored; never commit one.

## Scenarios

A scenario file declares its `id`, `prompt`, `kind` and `suites`; loading fails when `kind` or `suites` is missing.

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
| `id` | Lowercase letters, digits and dashes, 3 to 64 characters, unique across files. |
| `prompt` | The text sent to the agent, at most 4000 characters except for `chat_import` and `size_guard`; it may use `{panel_id}`, `{panel_title}` and `{row_title}`, filled from the seed. |
| `kind` | What the run does, from the table below. |
| `suites` | The suites that run it, each a lowercase name such as `live`, `replay` or `smoke`. |
| `category` | `prometheus`, `clickhouse` or `mixed`; required for `create`. |
| `done_when` | Counts towards `--min-done-when`. |
| `tags` | Free labels for `--tag`; `replay-smoke` also selects the scenario when no `--suite` is given. |
| `seed` | The corpus dashboard a revision starts from: `corpus` (a path under `pkg/grafana/testdata/corpus`), `break_panel`, `mapping`, `target` and `row`. |
| `fault` | A demo shop fault held during the run: `pod`, `type` (`errors` or `latency`), `pct` and `soak_seconds`. |
| `expect` | The checks below. |

Scenario prompt text is part of the measurement: change a prompt only together with a new baseline.

### Kinds

| Kind | The run | Checks it always gets |
| --- | --- | --- |
| `create` | Builds a new dashboard from the prompt. | `validity`, `data`, `source`, `panels`, `latency` |
| `revise` | Changes a seeded dashboard. | `failure_code`, `keeps_panels`, `new_panel_data` (unless `changed_paths`, `only_target_changed` or `changed_panels` is set), `panel_values`, `latency` |
| `fix_unverified` | Fixes a seeded unverified panel. | `failure_code`, `only_target_changed`, `target_data`, `unverified_cleared`, `latency` |
| `suggest` | Proposes one panel without saving a version. | `failure_code`, `proposal`, `no_version_added`, `latency` |
| `chat_import` | Imports pasted Grafana JSON through the chat. | `failure_code`, `imported`, `latency` |
| `size_guard` | Sends a prompt over the size limit, which must fail with `prompt_too_long`. | `failure_code` |

### Suites

| Suite | What declares it | How it runs |
| --- | --- | --- |
| `replay` | Scenarios with a replay fixture, plus every corpus task the replay provider can script. | `make evals-replay`, in CI, with no key. |
| `live` | Scenarios for a real model, plus the corpus tasks of the sample. | `make evals-live` with a model matrix. |
| `smoke` | Twelve live tasks: six `create` (three Prometheus, two ClickHouse, one mixed), two scenario revisions, the demo shop `change`, `fix` and `suggest` corpus tasks, and `chat-import-small`. | `go run ./evals/runner --suite smoke ...` for a quick check of a model or prompt change. |

`--suite` accepts any suite a scenario or corpus task declares, so a new suite needs no runner change.
Without `--suite`, only the `create` scenarios of the live suite and those tagged `replay-smoke` run.
List a selection without running it:

```sh
go run ./evals/runner list --suite smoke
```

Corpus tasks are generated from the dashboards under `pkg/grafana/testdata/corpus`: `paste`, `add`, `fix`, `change`, `suggest`, `rebuild`, `retime` and `rowedit` tasks, named `corpus-<task>-<source>-<dashboard>`.
`--corpus all|sample|none` picks which dashboards they come from; replay defaults to all, every other suite to the sample.

### Checks

Each check is a pure function of what the run saved; a scenario passes when every check passes.

| Check | `expect` fields | Passes when |
| --- | --- | --- |
| `validity` | | The run succeeded and its saved Spec parses. |
| `data` | | Every data panel returned data. |
| `source` | `datasources` | Panels query exactly these datasource types (`prometheus`, `clickhouse`). |
| `metrics` | `metrics`, `rebuild_metric_share`, `rebuild_metric_cap` | A Prometheus query contains each name, or the given share of them. |
| `tables` | `tables` | A ClickHouse query contains each table. |
| `units` | `units` | Some panel uses one of these units. |
| `query` | `query_contains_any` | Each group has one alternative some query contains. |
| `time_range` | `time_range_from` | The dashboard's `time.from` is one of these. |
| `panels` | `min_panels` (default 1), `max_panels` (default 12 for `create`) | The data panel count is in range. |
| `thresholds` | `thresholds`, `threshold_at` | A panel has thresholds, or a threshold at this value for this unit. |
| `coverage` | `scope`, `coverage` (default 0.9) | Queries name the share `coverage` of the metric families in `scope`; see below. |
| `latency` | `max_seconds` (default 60) | The run finished in time. |
| `failure_code` | `failure_code` | The run failed with this code, or succeeded when none is set. |
| `budgets` | `max_model_calls`, `max_output_tokens`, `max_save_output_tokens` | The run stayed within each budget. |
| `usage` | `usage` | Each named counter (`build_saves`, `renewals`, `compactions`, `nudges`, `retries`, `save_now_refusals`, `stalled_turns`, `one_panel_refusals`) has exactly this value. |
| `keeps_panels`, `new_panel_data`, `panel_values`, `changed_paths`, `changed_panels`, `only_target_changed`, `target_data`, `target_type`, `unverified_cleared` | `keeps_panels`, `panel_values`, `changed_paths`, `changed_panels`, `only_target_changed`, `target_returns_data`, `target_type`, `unverified_cleared` | The revision kept, changed or fixed what the field names. |

A failure on `latency` or `budgets` alone is reported as `FAIL (latency only)` and still counts as a failure.

## Quality

Every run that saved a dashboard gets a quality score from 0 to 1, computed from the saved Spec alone with fixed rule weights.

| Rule | Weight | Scored as |
| --- | --- | --- |
| Every data panel has a title. | 0.15 | The share of data panels with one. |
| A stat, gauge, bargauge, timeseries or barchart panel has a unit. | 0.20 | The share of those panels with one. |
| A multi-series timeseries query has a legend. | 0.15 | The share of those queries with one; a query is multi-series when its panel has more than one visible query or it groups `by` or `without`. |
| A stat or gauge has a description. | 0.10 | The share of those panels with one. |
| Every line of panels fills the 24 columns. | 0.15 | The share of full lines, per top level and per row, where a tall panel counts on every line it spans. |
| A dashboard with more than 4 data panels opens a section with a line of stats. | 0.10 | All or nothing. |
| Thresholds appear only on stat, gauge, bargauge and state-timeline panels. | 0.15 | The share of panels with threshold steps that are of those types. |

A rule with nothing to apply to scores full.
The rule against more than 8 series without a table legend is left out, because it needs the panel data.
Each result carries `quality` and `quality_findings`, the aggregates carry `quality_median` per kind, and `--min-quality` (default 0, off) fails the run when any kind's median is below it.

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
Replay fixtures match a run by its prompt and script each model turn: `text`, `tool_calls`, and an optional `fault` (`rate_limited`, `server_error`, `call_timeout`, `max_tokens`, `refusal` or `delay`).
A new fixture is read when the server starts, so restart the stack's server after adding one to a running stack.
Stop the stack with `make evals-stack-down`.

To rerun only what failed, pass each failed id with `--id` against the running stack:

```sh
go run ./evals/runner --server http://localhost:28080 --suite replay --bootstrap-dex --id fault-compaction
```

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

## Pricing

`evals/pricing.yaml` gives USD prices per million tokens for input, output, cache reads and cache writes.
A run's model is priced by its exact key first, then by the longest `match` prefix any entry lists, so a dated id is priced too.
A model with no entry, or a run that used a token class whose rate is `null`, has no cost rather than a zero one; the report names such a model once.
A matrix entry's `pricing` replaces the lookup for every run of that entry.

## Comparing reports

```sh
make evals-compare EVALS_BASE=evals/reports/base/haiku.json EVALS_NEW=evals/reports/live/haiku.json
```

The comparison lists, for every scenario in both reports, the status change and the latency, cost and quality of each run with their delta.
Its summary has one row per model with the pass count, total cost, p50 time to first panel and median quality of both runs and their delta.
It prints the comparison and writes `compare.md` next to the new report.
A run can compare its own report as it finishes: add `--compare <base.json>` to a run with `--out`.

## Reports

The JSON report holds the provider, the model, the aggregates and one result per scenario.
A result carries its checks, status, failure code, latency, usage, `cost_usd`, `quality`, `quality_findings`, `coverage` and the build metrics: time to first panel, panels per minute, calls per panel and tokens per call.
The aggregates carry pass rates per category and kind, failure codes, latency, cost with `unpriced_models`, tokens, `quality_median` and the time to first panel.
The Markdown report shows the same with one row per scenario and lists the gates that failed: `--min-pass` (default 0.8), `--min-done-when`, `--min-quality` and any failed run without a failure code.
