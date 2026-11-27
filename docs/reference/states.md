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

## Thread's agent

A thread's agent is the one a plain reply asks, returned as `agent` with its `state` by `GET /threads`.
No route takes these moves by name today, so the thread's `allowed_actions` do not list them: a message that mentions an agent or is sent to the agent makes `set_agent`, and a message sent to everyone makes `turn_agent_off`.
Each move is recorded as the audit action `thread.agent_set`, not as an activity event.

| State | Meaning | Actions |
| --- | --- | --- |
| `none` | No message has asked an agent yet, or the thread's agent can no longer take runs; a plain reply asks nobody. | `set_agent` |
| `following` | The thread has an agent and a plain reply asks it. | `set_agent`, `turn_agent_off` |
| `off` | The thread keeps its agent, but a message was sent to everyone, so a plain reply asks nobody until the agent is set again. | `set_agent` |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `open` | - | `none` | `thread.opened` | commenter |
| `set_agent` | `none`, `following`, `off` | `following` | `thread.agent_set` | commenter |
| `turn_agent_off` | `following` | `off` | `thread.agent_set` | commenter |
| `clear_agent` | `following`, `off` | `none` | `thread.agent_set` | server |

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

## RCA revision

An RCA revision is one published copy of an observation, returned by `GET /observations/{id}/rcas`; its state is computed on read from the newer revisions and the live public links, never stored.

| State | Meaning | Actions |
| --- | --- | --- |
| `published` | The newest revision of the observation's RCA, with no live public link. | `share`, `withdraw` |
| `superseded` | A newer revision was published after it, and it has no live public link. | `share`, `withdraw` |
| `shared` | At least one public link that has not expired shows it, so it cannot be withdrawn until they are revoked. | `share`, `unshare` |
| `withdrawn` | Gone: the revision and its frozen panels are deleted, so this state is never stored. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `publish` | - | `published` | `observation.published` | editor |
| `supersede` | `published` | `superseded` | `observation.published` | server |
| `share` | `published`, `superseded`, `shared` | `shared` | `public_link.created` | admin |
| `unshare` | `shared` | `published`, `superseded` | `public_link.revoked` | admin |
| `withdraw` | `published`, `superseded` | `withdrawn` | `observation.rca_withdrawn` | editor |

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

## Build

A build is what the agent assembles for a run before it becomes a version; every run read returns its `build_state`, and `build.state` while the run holds one.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `none` | The run holds no build and none of the states below applies; a stopped build deleted after its 7 days also reads `none`. |
| `building` | The run has not finished and the agent may still change the build. |
| `stopped` | The run failed or was canceled and left a build with at least one panel; its requester can keep or continue it until `build.expires_at`. |
| `stopped_empty` | The run failed or was canceled and left a build with no panels, which cannot be kept. |
| `stale` | A stopped build whose dashboard has a newer version than the one it started from: it can no longer be kept, and Continue moves it onto the newer version. |
| `kept` | The requester saved the stopped build as a version (`build_saved_at` is set). |
| `moved` | Another run continues this one and took the build (`continued_by` names it). |
| `committed` | The run finished and its build became the version in `result_version`. |
| `proposed` | The run finished and its build became a proposal. |

## Thread reply

A run asked in a comment thread owes that thread one reply when it ends; `GET /observe/{id}` returns its `reply.state`.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `not_owed` | The run was not asked in a comment thread, so it owes no reply. |
| `owed` | The run was asked in a thread and its reply is not posted yet: the run has not finished, or the server is still trying, for 24 hours after it finished. |
| `replied` | The reply is posted in the thread. |
| `abandoned` | The reply could not be posted within 24 hours after the run finished and the server stopped trying. |

## Dashboard managed by

`GET /dashboards/{id}` and each item of `GET /dashboards` return `managed_by`: what writes the dashboard's content.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `starter` | The server ships the dashboard and keeps it current with each release. Every change to its content answers 409 `provisioned`; duplicate it to change a copy. |
| `file` | A file in the server's provisioning directory is bound to the dashboard. It can be edited, and a change to that file replaces the edit, which stays in the version history. The dashboard keeps this value after the file is removed or provisioning is turned off. |
| `apply` | `observe dashboards apply` last bound a file to the dashboard. It can be edited, and applying that file again after it changes replaces the edit, which stays in the version history. |
| `workspace` | None of the above: people edit the dashboard in the app, and no file and no release writes it. |

## Proposal mergeability

`GET /proposals/{id}` returns the `mergeability` of an open proposal, which says what a merge would meet on the dashboard's latest version, and null once the proposal is merged or discarded.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `current` | It builds on the dashboard's latest version; a merge saves it as it is. |
| `behind_clean` | The dashboard has newer versions that changed other panels, variables and settings; a merge combines them without a choice. |
| `behind_conflicting` | The dashboard has newer versions that changed the same panel, variable or setting; a merge needs a choice for each conflict. |
| `blocked` | Combined with the latest version it is not a valid dashboard (a conflict whose reason is `invalid`); no choice fixes that, so every merge is refused until its author saves a new revision. |

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
| `repaired` | The run saved a version that the health check has not checked yet, or that the check found without a `no_data`, `query_failed` or `variable_failed` problem. |
| `still_failing` | The run saved a version and the check that ran on it, or on a later version, still finds a `no_data`, `query_failed` or `variable_failed` problem. |
| `proposed` | The run succeeded with a proposal, which waits for an Editor to merge it. |
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
| `revoked` | It was ended before its time, and `revoked_reason` says how: by its person, an admin or the client, by a newer connection of the same client, or because a code or refresh token was used twice. |
| `active` | It may call /mcp as its person until expires_at. |
| `expired` | Its refresh token went unused for 30 days; the person approves the client again. |
| `aged_out` | It reached the 90 day limit of a connection; the person approves the client again. |

## Dashboard save

`POST /dashboards/{id}/versions` and `PATCH /dashboards/{id}/panels` return the `outcome` of the save, and so do the `update_dashboard` and `update_panels` tools.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `proposed` | The caller may not save here, so the change became a proposal, or a revision of the caller's open one; no version was saved. |
| `replayed` | The save repeats a version that already exists, so that version is returned and no version was saved. |
| `merged` | The save was stale and the server merged it onto the latest version; the merge is the new version and merged_from names both ends. |
| `created` | None of the above; the save is the new version. |

## Public link

`GET /dashboards/{id}/public-links` returns the `state` of each link.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `expired` | Its expiry time has passed; the link no longer opens. |
| `active` | None of the above; usable says whether the link opens, and unusable_reason says why it does not. |

## Content confined to Admins-only folders

`GET /observations/{id}` returns `confined` with a `reason` while an answer or a run keeps the observation in folders only Admins can read, and leaves it out otherwise; a `restricted_answer` refusal carries the same `reason`.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `permanent` | It holds an answer that read data only Admins can query, such as the audit trail. That does not change, so waiting does not help. |
| `run_in_flight` | It is in a folder only Admins can read and an agent run about it has not ended. Try again once the run ends. |

## Folder

`GET /folders` and `GET /folders/{id}` return the `kind` of each folder, and its `allowed_actions` say what the caller may do with it.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `general` | It is General: everyone sees it by role, and it is never renamed, restricted or deleted. |
| `personal` | It is the caller's My Dashboards: only they see it, it takes new work but no moves, and nobody renames, shares or deletes it. |
| `audit` | It is the Audit folder the server made for the audit starters; `admins_only` and `restricted` say who reads it. |
| `admins_only` | Only Admins read it, and it lists nobody. |
| `restricted` | It lists people or groups, and only they, its creator and Admins see it. |
| `open` | None of the above: everyone sees it, each at the level of their role. |

## Inbox item

`GET /inbox` returns the `state` of each item for the person reading it.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `done` | The reader marked it done; it stays done when the inbox is read, until it is brought back or marked unread. |
| `unread` | It is newer than the position the reader read up to, or the reader marked it unread. |
| `read` | None of the above. |

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

## Grafana connection test

`GET /grafana` returns the `test.state` of each connection.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `untested` | The connection has not been tested since it was added or its URL or token changed; last_test is null. |
| `failing` | The last test could not read Grafana; failure_kind says who can fix it: transient (Grafana did not answer, try again), credentials (the token was refused), configuration (the URL, the outbound allowlist or what answers at the address), unsupported (an answer this server cannot take) or unknown. |
| `ok` | None of the above: the last test read Grafana's health, folders and dashboards. |

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
| `unreachable` | The last discovery failed, or an agent's call could not reach the server since; failure says why, such as the domain not being on the outbound allowlist, the secret being refused or the server not answering. |
| `ready` | None of the above: the last discovery listed the server's tools, or the server has answered an agent's call since it last failed. |

## Built-in agent supervisor

The server starts the built-in agent as the `observe-agent` process and restarts it when it exits; each move writes a server log line with the state under the key `agent_state`, not an activity event.

| State | Meaning | Actions |
| --- | --- | --- |
| `starting` | The server is starting the `observe-agent` process. | - |
| `running` | The process runs and can claim runs. | - |
| `backoff` | The process exited or could not be started, and the server waits 1 to 30 seconds before it starts it again. | - |
| `stopping` | The server is shutting down and stops the process. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `start` | - | `starting` | `observe-agent starting` | server |
| `run` | `starting` | `running` | `observe-agent running` | server |
| `back_off` | `starting`, `running` | `backoff` | `observe-agent waiting to restart` | server |
| `restart` | `backoff` | `starting` | `observe-agent starting` | server |
| `stop` | `starting`, `running`, `backoff` | `stopping` | `observe-agent supervisor stopping` | server |

## Panel and variable verification

`GET /dashboards/{id}` returns the panels and variables of a version that are not verified in `unverified`, each with the `reason` the save kept it.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `unverified` | The version lists it in `unverified`: its query failed or returned nothing when the version was saved and the save kept it, with a `reason` of `empty` (it returned no data), `timeout` (it timed out and the save did not change it), `carried` (the parent version already listed it and it is unchanged), `restored` (the restored version listed it) or `imported` (a Grafana import keeps every failing panel). |
| `verified` | None of the above; its query ran when the version was saved, or when the last version that changed it was. |

## Agent presence

Whether an outside agent is taking runs: derived from when it last asked for work and the runs it holds.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `working` | It holds the lease of a run it claimed. |
| `listening` | It asked for work in the last 40 seconds. |
| `offline` | It has not asked for work in 40 seconds and holds no run. |

## Person

A person is someone who signs in, returned by `GET /users` with `state` and `allowed_actions`; an invited email is listed by `GET /invites` until its first sign-in.

| State | Meaning | Actions |
| --- | --- | --- |
| `invited` | An Admin invited the email and nobody has signed in with it; listed by `GET /invites`, not by `GET /users`. | `invite`, `withdraw` |
| `active` | Can sign in and work. | `set_role`, `sign_out`, `deactivate` |
| `deactivated` | Cannot sign in; their sessions and connected agents are gone and their work is kept. | `set_role`, `reactivate`, `remove` |
| `removed` | The invite was withdrawn, or the deactivated person and their My Dashboards were deleted. Final. | - |

| Action | From | To | Event | By |
| --- | --- | --- | --- | --- |
| `invite` | - | `invited` | `user.invited` | admin |
| `invite` | `invited` | - | `user.invite_updated` | admin |
| `withdraw` | `invited` | `removed` | `user.invite_withdrawn` | admin |
| `join` | - | `active` | `auth.login` | server |
| `join` | `invited` | `active` | `auth.login` | server |
| `set_role` | `active`, `deactivated` | - | `user.role_changed` | admin |
| `sign_out` | `active` | - | `user.signed_out_everywhere` | admin |
| `deactivate` | `active` | `deactivated` | `user.deactivated` | admin |
| `reactivate` | `deactivated` | `active` | `user.reactivated` | admin |
| `remove` | `deactivated` | `removed` | `user.removed` | admin |

## Scheduled job

`GET /jobs` returns the `state` of each scheduled job to an Admin.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `running` | A server holds its lease and is running it. |
| `interrupted` | Its last run did not finish: the server crashed and the lease ran out, or the server shut down and gave the lease back. The next server that finds it due runs it again. |
| `due` | Its next run time has passed and no server has claimed it yet. |
| `waiting` | Its last run finished, or it never ran, and the next run time is ahead. |

## Access request

`GET /access-requests` returns the `state` of each request for Comment access; the list of requests you can grant holds only open ones, and `scope=mine` returns your own in every state.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `open` | Asked in the last 30 days, not granted, nobody who can grant it marked it done in their inbox, and the requester has not asked again for the same object since. |
| `granted` | The requester got Comment access after asking. |
| `dismissed` | Someone who can grant it marked it done in their inbox without granting it, or it was marked done and the requester has asked again since; the requester may ask again. |
| `expired` | Nobody granted or dismissed it within 30 days; the requester may ask again. |

## Datasource health

`GET /datasources` and `POST /datasources/{uid}/recheck` return the `health_state` of each datasource, from the last check the answering server ran.
This state is computed from stored fields on every read; nothing stores it.

| State | When |
| --- | --- |
| `unchecked` | No check has answered: the caller left before it did, or the check was cancelled. `healthy` is `false`, and nothing says the datasource is failing. |
| `healthy` | The last check passed, or the datasource answered a query after a check it did not answer. |
| `failing` | The last check failed, and `failure_kind` says whether to wait or to change something. |
| `rechecking` | The last check failed 2 seconds ago or more and a new one is running; `message`, `detail` and `failure_kind` are still those of the failed check. |

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
