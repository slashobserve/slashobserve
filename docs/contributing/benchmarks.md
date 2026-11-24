---
title: Running benchmarks
description: How contributors run the Observe Agent on o11y-bench tasks through the Harbor adapter, with the replay model or a real one.
---

The `benchmarks/` folder lets you measure the Observe Agent against an external benchmark.
It holds a Harbor agent that runs the Observe Agent on the benchmark's tasks.
The first benchmark is o11y-bench.
Use it to check that the adapter still works end to end, or to compare agent changes on the same tasks.

For each task the adapter starts a throwaway server and Postgres and points the server at the task's stores.
It asks the task's question as of the time the task's data ends, and writes the answer in the form the benchmark grades.
It also saves the run's full trajectory, token counts and cost beside the result, so grading can be repeated without a model.

## Run the adapter's tests

`make test-benchmarks` runs the adapter's unit tests, and `make test` and CI include it.
It needs `uv` only, and CI never calls a model from the adapter.

## Run one task

A trial needs Docker, a checkout of o11y-bench, `mise`, `uv` and a server image that supports `as_of` on a run.
Build the image from a checkout of main or v0.1.5 or later with `docker build -t slashobserve-server:local .`.

1. Run the replay script with the path of your o11y-bench checkout and a task name:

   ```sh
   benchmarks/run-replay.sh /path/to/o11y-bench promql-error-rate
   ```

2. The replay model runs the task with no key, so the answer is a fixed text and the run proves the plumbing.
3. The script prints the path of the trial's `trajectory.json` when it finishes.

Set `SLASHOBSERVE_SERVER_IMAGE` to use another image than `slashobserve-server:local`.

## Run tasks with a real model

A real run spends tokens, so start with one task and a cost cap.
For a real model, a subset of tasks, the options, the spend cap and the tasks that are skipped, read [`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md) in the repository.

## Reference

### What runs where

| Part | Needs |
| --- | --- |
| `make test-benchmarks` | `uv` only. |
| One trial | Docker, a checkout of o11y-bench and a server image that supports `as_of` on a run. |
| Grading | `ANTHROPIC_API_KEY`, because the o11y-bench judge is a model call; the replay script skips it. |

### What a trial does

1. It starts a throwaway Postgres and the server image, with a datasources file that points at the task's stores.
2. It starts a run with the task statement as the prompt and `as_of` set to the time the task's data ends.
3. It polls the run until it finishes, writes the result into the trial's `agent/` folder and removes both containers.

### Where the details are

[`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md) covers the trial step by step, the requirements, the `--ak` options, the cost cap, the skipped tasks, cleanup and the pinned dependencies.
