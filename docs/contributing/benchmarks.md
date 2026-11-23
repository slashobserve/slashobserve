---
title: Running benchmarks
description: How contributors run the Observe Agent on o11y-bench tasks through the Harbor adapter, with the replay model or a real one.
---

The `benchmarks/` folder holds a Harbor agent that runs the Observe Agent on the tasks of an external benchmark.
It starts a throwaway server and Postgres for each task, points the server at the task's stores, asks the task's question as of the time the task's data ends, and writes the answer in the form the benchmark grades.
It also saves the run's full trajectory, token counts and cost beside the result, so grading can be repeated without a model.
The first benchmark is o11y-bench.

## What runs where

`make test-benchmarks` runs the adapter's unit tests, and `make test` and CI include it.
It needs `uv` only, and CI never calls a model from the adapter.
A trial needs Docker, a checkout of o11y-bench and a server image that supports `as_of` on a run.

## Run a task

```sh
benchmarks/run-replay.sh /path/to/o11y-bench promql-error-rate
```

The replay model runs the task with no key, so the answer is a fixed text and the run proves the plumbing.
For a real model, a subset of tasks, the options, the spend cap and the tasks that are skipped, read [`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md) in the repository.
A real run spends tokens, so start with one task and a cost cap.
