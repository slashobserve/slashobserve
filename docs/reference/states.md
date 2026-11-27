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
| `succeeded` | It saved its result: a version, a suggestion, or a finding written into the investigation's observation or kept on the run until its requester saves it. Final. | `retry`, `pin`, `archive` |
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

A comment thread is a conversation on a dashboard, an observation, an annotation or a proposal, returned by `GET /threads` and by every route that opens, replies to, resolves or reopens one.

| State | Meaning | Actions |
| --- | --- | --- |
| `open` | Waiting for an answer or a change. | `reply`, `resolve` |
| `resolved` | Someone, or an agent run that saved the change it asked for, marked it done. | `reply`, `reopen` |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `open` | - | `open` | `thread.opened` | commenter |
| `reply` | `open`, `resolved` | - | `thread.replied` | commenter |
| `resolve` | `open` | `resolved` | `thread.resolved` | requester, editor, server |
| `reopen` | `resolved` | `open` | `thread.reopened` | commenter |

## Observation

An observation is a page of blocks in a folder, started by POST /observations or by an investigation.

| State | Meaning | Actions |
| --- | --- | --- |
| `open` | Still being worked on: blocks are added and the discussion goes on. | `update`, `move`, `resolve`, `publish`, `delete` |
| `resolved` | Closed with a one-line summary of what was found; it can be reopened. | `update`, `move`, `reopen`, `publish`, `delete` |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `create` | - | `open` | `observation.created` | editor |
| `update` | `open`, `resolved` | - | `observation.updated` | editor |
| `move` | `open`, `resolved` | - | `observation.moved` | editor |
| `resolve` | `open` | `resolved` | `observation.resolved` | editor |
| `reopen` | `resolved` | `open` | `observation.reopened` | editor |
| `publish` | `open`, `resolved` | - | `observation.published` | editor |
| `delete` | `open`, `resolved` | - | `observation.deleted` | admin, requester |

## Observation block

A block is one item on an observation's page: Markdown, a live panel, a frozen snapshot or an agent run.

| State | Meaning | Actions |
| --- | --- | --- |
| `live` | On the page for everyone who can see the observation. | `update`, `remove` |
| `requested` | Asked for by someone who can comment but not edit; an editor keeps it or discards it. | `update`, `keep`, `discard` |
| `deleted` | Gone: the row is deleted, so this state is never stored. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `add` | - | `live` | `observation.block_added` | editor |
| `add` | - | `requested` | `observation.block_added` | commenter |
| `update` | `live`, `requested` | - | `observation.block_updated` | editor |
| `keep` | `requested` | `live` | `observation.block_kept` | editor |
| `discard` | `requested` | `deleted` | `observation.block_discarded` | editor, requester |
| `remove` | `live` | `deleted` | `observation.block_removed` | editor |

## Proposal

A proposal is a change to a dashboard that someone who can edit merges or discards, returned by `GET /api/v1/proposals/{id}`.

| State | Meaning | Actions |
| --- | --- | --- |
| `open` | Waiting for someone who can edit to merge or discard it. | `update`, `merge`, `discard` |
| `merged` | Its change was saved as a new version of the dashboard. Final. | - |
| `discarded` | Closed without saving its change. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `propose` | - | `open` | `proposal.opened` | commenter |
| `update` | `open` | - | `proposal.updated` | requester |
| `merge` | `open` | `merged` | `proposal.merged` | editor |
| `discard` | `open` | `discarded` | `proposal.discarded` | requester, editor |

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

## MCP source tool

An MCP source tool is one tool of an MCP server an Admin registered, returned by `GET /api/v1/mcp-sources/{uid}/tools`; discovery moves it between `offered`, `changed`, `gone` and `refused`, and an Admin approves or revokes it.

| State | Meaning | Actions |
| --- | --- | --- |
| `offered` | An Admin has not approved the tool; the agent cannot call it. read_only says whether the server marks it as only reading. | `approve` |
| `approved` | An Admin approved the tool as pinned (its name, description and input schema, by hash); the agent may call it. | `revoke` |
| `changed` | The server now describes an approved tool differently from what was pinned; it is not served until an Admin approves the new description. | `approve`, `revoke` |
| `gone` | The server no longer lists an approved tool; it is not served, and it returns to approved if the server lists it again unchanged. | - |
| `refused` | The server marks the tool destructive, or its description or schema is too large; refusal says why and nobody can approve it. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `list` | - | `offered` | `mcp_source.discover` | server |
| `reject` | - | `refused` | `mcp_source.discover` | server |
| `offer` | `refused` | `offered` | `mcp_source.discover` | server |
| `refuse` | `offered`, `approved`, `changed`, `gone` | `refused` | `mcp_source.discover` | server |
| `approve` | `offered`, `changed` | `approved` | `mcp_source.approve` | admin |
| `revoke` | `approved`, `changed` | `offered` | `mcp_source.revoke` | admin |
| `change` | `approved`, `gone` | `changed` | `mcp_source.discover` | server |
| `drop` | `approved`, `changed` | `gone` | `mcp_source.discover` | server |
| `restore` | `changed`, `gone` | `approved` | `mcp_source.discover` | server |

## MCP source

`GET /api/v1/mcp-sources` returns the `state` of each MCP source.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `unchecked` | No discovery has run since the source was added or its address changed. |
| `unreachable` | The last discovery failed; failure says why, such as the domain not being on the outbound allowlist, the secret being refused or the server not answering. |
| `ready` | None of the above: the last discovery listed the server's tools. |

## Agent presence

Whether an outside agent is taking runs: derived from when it last asked for work and the runs it holds.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `working` | It holds the lease of a run it claimed. |
| `listening` | It asked for work in the last 40 seconds. |
| `offline` | It has not asked for work in 40 seconds and holds no run. |

## Server process

A server process moves through these phases from start to exit: `GET /readyz` returns the current one as `phase`, the `slashobserve_server_phase` series is 1 for it, and each move writes one server log line with the name in the Event column and the new phase as its `phase` attribute, not an activity event.

| State | Meaning | Actions |
| --- | --- | --- |
| `booting` | It reads its settings and connects to Postgres. | - |
| `migrating` | It applies the pending database migrations. | - |
| `provisioning` | It reconciles roles, applies the service-account and MCP source files and builds its services; it does not listen yet. | - |
| `ready` | It listens and serves requests, and `/readyz` answers 200 while its checks pass. | - |
| `stopping` | It got the shutdown signal: `/readyz` answers 503 while it keeps serving for 5 seconds, then it stops accepting requests and finishes the open ones. The 503 code `shutting_down` of the event streams is this phase. | - |
| `stopped` | It serves nothing more and exits. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `boot` | - | `booting` | `starting slashobserve-server` | server |
| `migrate` | `booting` | `migrating` | `migrating database` | server |
| `provision` | `migrating` | `provisioning` | `provisioning` | server |
| `serve` | `provisioning` | `ready` | `listening` | server |
| `stop` | `ready` | `stopping` | `shutting down` | server |
| `exit` | `booting`, `migrating`, `provisioning`, `ready`, `stopping` | `stopped` | `server stopped` | server |
