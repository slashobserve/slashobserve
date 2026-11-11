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
| `expire` | `queued`, `running` | `failed`, `succeeded` | `run.failed` or `run.succeeded` | server |
| `keep_build` | `failed`, `canceled` | - | `run.kept` | requester |
| `continue` | `failed`, `canceled` | - | - | requester |
| `retry` | `succeeded`, `failed`, `canceled` | - | - | anyone who may start one |
| `pin` | `queued`, `running`, `canceling`, `succeeded`, `failed`, `canceled` | - | - | requester |
| `archive` | `queued`, `running`, `canceling`, `succeeded`, `failed`, `canceled` | - | - | requester |

## Comment thread

A comment thread is a conversation on a dashboard, a finding or an annotation, returned by `GET /threads` and by every route that opens, replies to, resolves or reopens one.

| State | Meaning | Actions |
| --- | --- | --- |
| `open` | Waiting for an answer or a change. | `reply`, `resolve` |
| `resolved` | Someone, or an agent run that saved the change it asked for, marked it done. | `reply`, `reopen` |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `open` | - | `open` | `thread.opened` | commenter |
| `reply` | `open`, `resolved` | - | `thread.replied` | commenter |
| `resolve` | `open` | `resolved` | `thread.resolved` | commenter, server |
| `reopen` | `resolved` | `open` | `thread.reopened` | commenter |

## Dashboard health

`GET /health` returns the `state` of each dashboard that has a problem.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `unchecked` | The health check has not checked this dashboard yet. |
| `outdated` | The last check ran on an older version than the latest one. |
| `skipped` | The last check skipped the dashboard because its queries read the viewer's identity, so no one result applies. |
| `failing` | The last check found a panel query or a variable that fails. |
| `healthy` | None of the above. No data, a duplicate title and not being opened stay problems in the list but do not make a dashboard failing. |

## Health repair

`GET /health` returns the `outcome` of a dashboard's latest repair beside the repair run's own `state`.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `queued` | The repair run waits for a worker. |
| `running` | The repair run is running or stopping. |
| `repaired` | The run saved a version that the health check has not checked yet. |
| `still_failing` | The run saved a version and the check that ran on it still lists the dashboard. |
| `failed` | The run failed or was canceled. |

## Webhook

`GET /webhooks` returns the `state` of each webhook.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `disabled` | An admin turned it off, or the server did because the receiver is gone, deliveries kept failing, its owner is no longer an admin or its secret cannot be read; disabled_reason says which. |
| `retrying` | The last delivery failed and the server retries in order. |
| `sending` | None of the above: deliveries go out as events happen. |

## Service-account key

`GET /service-accounts/{id}/keys` returns the `state` of each key.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `revoked` | An admin revoked it. |
| `expired` | Its expiry time has passed. |
| `expiring` | It expires within 7 days. |
| `active` | None of the above. |

## Connected agent

`GET /me/grants` and `GET /grants` return the `state` of each connected agent.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `revoked` | The person or an admin revoked it; it leaves the list. |
| `active` | It may call /mcp as its person until expires_at. |

## Public link

`GET /dashboards/{id}/public-links` returns the `state` of each link.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `expired` | Its expiry time has passed; the link no longer opens. |
| `active` | None of the above; usable says whether the dashboard's latest version can be shown publicly. |

## Grafana import

A Grafana import is one run of `POST /grafana/{id}/imports`, answered by `GET /grafana/{id}/imports/{sync_id}`; its moves write server log lines with these names, not activity events, because a Grafana connection is Admin only.

| State | Meaning | Actions |
| --- | --- | --- |
| `running` | A server holds its lease and imports the chosen dashboards. | `cancel` |
| `finished` | Every chosen dashboard was handled, each imported, updated, skipped or failed on its own. Final. | - |
| `failed` | It stopped early: Grafana could not be listed, the import could not continue as the person who started it, or its server stopped twice without handing it on. Final. | - |
| `canceled` | An Admin stopped it; the dashboards it already imported stay. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `start` | - | `running` | `grafana sync started` | admin |
| `claim` | `running` | `running` | `grafana sync resumed` | server |
| `release` | `running` | `running` | `grafana sync released` | server |
| `finish` | `running` | `finished` | `grafana sync finished` | server |
| `fail` | `running` | `failed` | `grafana sync failed` | server |
| `exhaust` | `running` | `failed` | `grafana sync lease expired twice` | server |
| `cancel` | `running` | `canceled` | `grafana sync canceled` | admin |
