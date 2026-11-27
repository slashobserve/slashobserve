---
title: Running benchmarks
description: How contributors run the Observe Agent on o11y-bench and ORCA-bench tasks through the Harbor adapter and on SREGym problems as a registered agent, with the replay model or a real one.
---

The `benchmarks/` folder lets you measure the Observe Agent against an external benchmark.
It holds a Harbor agent that runs the Observe Agent on the benchmark's tasks.
The first benchmark is o11y-bench, and ORCA-bench, which asks for a root cause report on recorded telemetry, has its own module and environment.
SREGym, which injects a fault into a live Kubernetes cluster, is not a Harbor benchmark and has its own runner.
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
Every one of the 64 o11y-bench tasks runs, the Grafana API and dashboard tasks included.
For a real model, a subset of tasks, the options, the spend cap and how the Grafana tasks run, read [`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md) in the repository.

## Run ORCA-bench tasks

ORCA-bench runs on Harbor 0.20.0 and a snapshot image of about 50 GB, so it has its own environment in `benchmarks/orca/`, and `make test-benchmarks` tests both.
The adapter runs the Observe Agent beside the task's main container, writes `/app/report.md` from the run's finding, and runs a fixed subset of ten tasks.
Scores from it use a different judge than the leaderboard, so they are not comparable with published ones.
By default each trial gives the agent the task's source code through the repository's read-only code server (`tools/codeserver`), provisioned as an MCP source, because the answer is usually a feature flag defined in that source.
Build its image once with `docker build -f tools/codeserver/Dockerfile -t slashobserve-codeserver:local .` from the repository root, or pass `--ak code_context=off` to run without code.
A server image built before MCP sources were renamed reads the older sources file setting and needs `--ak source_names=context`.

The trial's spans are an OpenSearch index that the datasources file marks `spans: jaeger`, so the trace tools answer on it, and that needs a server image built from `agent-rca-accuracy` or later.
An older image refuses the field and does not start; pass `--ak span_index=false` for it, and the spans are read as plain documents without the trace tools.
The report is built from the finding's ranked causes, which a server image sends from `agent-rca-accuracy` on: each stated cause that follows no other is a root cause with its component, mechanism, trigger key and value, the queries and trace ids behind it and its onset in UTC.
A knock-on is written as a consequence of its root cause, and a cause the server did not confirm goes under the report's unexplored branches, never as a root cause.
An older image's finding fills the report from its single root cause or from its text, as before.
The steps, the disk the snapshot needs and how to point the judge at a Claude model are in the ORCA-bench section of [`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md).

## Run SREGym problems

SREGym runs the adapter as one of its registered agents, on the host beside its kind cluster, without changing the SREGym checkout.
The adapter answers the diagnosis stage only, never mitigates, and runs a fixed subset of ten SREGym-Lite problems.
It needs a server image built from branch `agent-rca-accuracy` or later, because SREGym's kubectl MCP server speaks SSE and marks no tool read-only, and only from that branch does the server speak SSE and approve such a tool when the provisioning file names it.
An older image cannot approve the kubectl tool, and the run then stops before the investigation, or works from telemetry alone with `--without-kubectl`.
The diagnosis is built from the finding's ranked causes: each stated cause that follows no other is an origin with its component, mechanism and changed key and value, and its knock-ons and affected components are its scope, while an unconfirmed cause is left out because an extra suspect costs points.
Read-only on the cluster is an instruction in the playbook and not enforced by the server, which is acceptable only because the benchmark cluster is thrown away after each run.
Its diagnosis success rate compares only loosely with SREGym's published diagnosis numbers, and the adapter has not yet run against a cluster.
The steps, what SREGym records and what a first run must confirm are in the SREGym section of [`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md).

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

[`benchmarks/README.md`](https://github.com/slashobserve/slashobserve/blob/main/benchmarks/README.md) covers the trial step by step, the requirements, the `--ak` options, the cost cap, the Grafana tasks, cleanup and the pinned dependencies.
