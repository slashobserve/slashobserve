---
title: Running the evals
description: How contributors check an agent change with the replay and live eval suites, and compare two runs.
---

The eval runner asks a running SlashObserve server to build and change dashboards, then checks what the agent saved.
Run the replay suite on every agent change; it needs no model key and is what CI runs.
Run the live suite by hand when a change should make a real model better, because it spends tokens.
The full reference, with every scenario field, check and report field, is `evals/README.md` in the repository.

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

```sh
make evals-stack-up EVALS_STACK_MODE=deploy/compose/.env.haiku
make evals-live EVALS_REPORT_DIR=evals/reports/live
```

For a quick check of one model, run the twelve tasks of the smoke suite instead:

```sh
go run ./evals/runner --server http://localhost:28080 --suite smoke --bootstrap-dex --out evals/reports/smoke/run.json
```

`go run ./evals/runner list --suite smoke` prints what a suite would run without running it.

## Reading a report

Each run prints a Markdown report and, with `--out`, writes a JSON report.
A scenario passes when all its checks pass; the report lists the failed checks and the failure code of every run.
Every saved dashboard also gets a quality score from 0 to 1 for titles, units, legends, descriptions, full rows, a headline row of stats and thresholds on the right panel types.
`--min-quality` fails the run when a kind's median score falls below it.
Costs come from `evals/pricing.yaml`; a model it does not price shows no cost rather than zero.

## Comparing two runs

```sh
make evals-compare EVALS_BASE=evals/reports/base/haiku.json EVALS_NEW=evals/reports/live/haiku.json
```

The comparison shows each scenario's status change and its latency, cost and quality deltas, with a summary per model, and writes `compare.md` next to the new report.
Keep the report of the commit you start from, so you can compare your change against it.

## Adding a scenario

Add a file to `evals/scenarios/` with an `id`, a `prompt`, a `kind` and the `suites` that run it; loading fails without `kind` or `suites`.
A scenario in the replay suite also needs a fixture in `evals/fixtures/llm/` whose `prompt` matches the scenario's prompt.
Do not change the prompt of an existing scenario, because its results would no longer compare with earlier reports.
