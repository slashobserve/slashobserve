---
title: States
description: Every state a run, comment thread and workspace object can be in, the actions each state allows and the activity event each move records.
---

This page is generated from the server's state tables by `make generate`, and a test fails when it falls behind them.
An action listed for a state is offered in that object's `allowed_actions` to a caller who may take it.

## Run

A run is one agent request, started by `POST /observe`, a comment, a prompt, an investigation or a suggestion.

| State | Meaning | Actions |
| --- | --- | --- |
| `queued` | Waiting for a worker. | `cancel`, `pin`, `archive` |
| `running` | A worker holds its lease and runs the agent. | `cancel`, `pin`, `archive` |
| `canceling` | The requester canceled it and its worker is stopping. | `cancel`, `pin`, `archive` |
| `succeeded` | It saved its result: a version, a suggestion or a finding. Final. | `retry`, `pin`, `archive` |
| `failed` | It stopped without a result; a build it left can be kept or continued. Final. | `keep_build`, `continue`, `retry`, `pin`, `archive` |
| `canceled` | The requester canceled it; a build it left can be kept or continued. Final. | `keep_build`, `continue`, `retry`, `pin`, `archive` |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `enqueue` | - | `queued` | `run.queued` | requester |
| `claim` | `queued`, `running` | `running` | `run.started` | server |
| `release` | `running` | `queued` | `run.released` | server |
| `cancel` | `queued`, `running`, `canceling` | `canceling`, `canceled`, `succeeded` | `run.canceling` or `run.canceled` or `run.succeeded` | requester |
| `succeed` | `running`, `canceling` | `succeeded` | `run.succeeded` | server |
| `fail` | `running` | `failed` | `run.failed` | server |
| `settle` | `queued`, `canceling` | `canceled`, `succeeded` | `run.canceled` or `run.succeeded` | server |
| `exhaust` | `running` | `failed`, `succeeded` | `run.failed` or `run.succeeded` | server |
| `keep_build` | `failed`, `canceled` | - | `run.kept` | requester |
| `continue` | `failed`, `canceled` | - | - | requester |
| `retry` | `succeeded`, `failed`, `canceled` | - | - | anyone who may start one |
| `pin` | `queued`, `running`, `canceling`, `succeeded`, `failed`, `canceled` | - | - | requester |
| `archive` | `queued`, `running`, `canceling`, `succeeded`, `failed`, `canceled` | - | - | requester |
