---
title: Running the evals
description: How contributors check an agent change with the replay and live eval suites, repeat a scenario, and compare two runs.
---

The eval runner asks a running SlashObserve server to build, change and investigate, then checks what the agent saved.
Run the replay suite on every agent change; it needs no model key and is what CI runs.
Run the live suite by hand when a change should make a real model better, because it spends tokens.
The contributor's guide, with every scenario field, fault, check, quality rule and report field, is [`evals/README.md`](https://github.com/slashobserve/slashobserve/blob/main/evals/README.md) in the repository.

## Suites

| Suite | What it is for |
| --- | --- |
| `replay` | Proving the runner, the server and the agent loop still work, with scripted model turns and no key. |
| `live` | Measuring a real model: pass rate, quality, latency and cost, including investigations of injected shop faults. |
| `smoke` | Thirteen live tasks for a quick check of a model, prompt or agent change before a full live run. |

`go run ./evals/runner list --suite smoke` prints what a suite would run without running it.

## Replay suite

```sh
make evals-replay
make evals-stack-down
```

`make evals-replay` boots the isolated `slashobserve-evals` stack on http://localhost:28080 with scripted model turns and fails unless every scenario passes.
To rerun one failed scenario against the running stack, pass its id:

```sh
go run ./evals/runner --server http://localhost:28080 --suite replay --bootstrap-dex --id fault-compaction
```

## Live suite

Copy `evals/matrix.example.yaml` to `evals/matrix.yaml` and point each entry at a compose env file with that provider's settings and key; both files stay out of git.
Pass only the env file's path: never print the file, echo a key or put one on a command line.

```sh
make evals-stack-up EVALS_STACK_MODE=deploy/compose/.env.haiku
make evals-live EVALS_REPORT_DIR=evals/reports/live
```

For a quick check of one model, run the thirteen tasks of the smoke suite instead:

```sh
go run ./evals/runner --server http://localhost:28080 --suite smoke --bootstrap-dex --out evals/reports/smoke/run.json
```

`--bootstrap-dex` mints a service-account key for the run, so the runner never needs the model key.

## Repeating a scenario

A live model does not answer the same way every time, so measure a rate rather than one run:

```sh
go run ./evals/runner --server http://localhost:28080 --suite live --bootstrap-dex --id investigate-checkout-errors-on-one-pod --repeat 10 --out evals/reports/live/errors.json
```

`--repeat` takes 1 to 20; runs of one scenario never overlap, a fault is set and cleared on every run, and the report shows `k of N` runs passed per scenario.
The pass rate, latency and cost figures count every run.

## Reading a report

Each run prints a Markdown report and, with `--out`, writes a JSON report.
A scenario passes when all its checks pass; the report lists the failed checks and the failure code of every run.
Every saved dashboard also gets a quality score from 0 to 1 from the rules in `internal/taste`, the same rules the shipped reference dashboards pass: titles, units, legends, descriptions, thresholds, full lines, a headline of stats, rows that ask questions and lone panels that stand alone.
`--min-quality` fails the run when a kind's median score falls below it.
Costs come from `evals/pricing.yaml`; a model it does not price shows no cost rather than zero.

## Comparing two runs

```sh
make evals-compare EVALS_BASE=evals/reports/base/haiku.json EVALS_NEW=evals/reports/live/haiku.json
```

The comparison matches runs by scenario id and shows each scenario's status change, or its pass rate when it ran more than once, with latency, cost and quality deltas, a summary per model, and writes `compare.md` next to the new report.
Keep the report of the commit you start from, so you can compare your change against it.

## Adding a scenario

Add a file to `evals/scenarios/` with an `id`, a `prompt`, a `kind` and the `suites` that run it; loading fails without `kind` or `suites`.
A `revise` or `investigate` scenario can hold a demo shop `fault` (`errors`, `latency`, `queue` or `slow_db`) on `checkout-0` or `checkout-1` while it runs, and belongs in the live suite only.
A scenario in the replay suite also needs a fixture in `evals/fixtures/llm/` whose `prompt` matches the scenario's prompt.
Do not change the prompt of an existing scenario, because its results would no longer compare with earlier reports.
