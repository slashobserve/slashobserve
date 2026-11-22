---
title: Activity
description: Read and follow what changed in your workspace, from dashboard saves to agent runs, through the HTTP API and MCP, with each person seeing only the folders they can view.
---

The activity log records every change to a dashboard, annotation, finding, saved search or observation, every agent run, and the changes an Admin makes to who can do what, in the order it happened.
Each change appends one event in the same transaction as the change, so the log never shows something that did not happen and never misses something that did.
You read it as a list with `GET /api/v1/activity` and follow it live as Server-Sent Events with `GET /api/v1/activity/stream`.
Both need the Viewer role or higher, and both work with a session or a service-account key.

## What is recorded

| Target | Event types | Written when |
| --- | --- | --- |
| Dashboard | `dashboard.created`, `dashboard.version_created`, `dashboard.restored` | A version is saved: the first one, a later one, or a restore of an older one. |
| Dashboard | `dashboard.moved`, `dashboard.deleted` | A dashboard moves to another folder or is deleted. |
| Annotation | `annotation.created`, `annotation.updated`, `annotation.deleted` | An [annotation](annotations.md) is added, has its text or tags edited, or is removed; a create and a delete carry a `title`, the first 80 characters of the annotation's text on one line, copied when the event is written, and a delete repeats the annotation's `panel_id`, `tags` and times, so a row still says which annotation went. |
| Public link | `public_link.created`, `public_link.revoked` | Someone who manages a dashboard or an RCA creates or revokes a [public link](public-links.md); the event belongs to that dashboard or observation, and `data` has the `object` (`kind` and `id`), its `title`, the link's `prefix` and `expires_at`, never the token. |
| Access request | `access_request.opened`, `access_request.granted` | A Viewer [asks for comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation, or someone changes a folder's access or a role so that the requester can comment; the event belongs to that dashboard or observation, `data` has the `object` (`kind` and `id`) and its `title`, a grant adds `requested_by` (`kind`, `id` and `name`), and the note is never recorded. |
| Finding | `finding.created`, `finding.pinned` | A finding is saved, or pinned to its dashboard. The answer's observation records its own `observation.created` and block events. A finding whose run is `restricted`, or whose answer is [kept for what it is about](chat.md#answers-that-read-admin-only-data), records `finding.created` on its observation instead of its dashboard, so only that observation's readers see its title. |
| Finding | `finding.moved` | A finding without a dashboard moved to another folder through the finding API removed since; only earlier events carry it. |
| Saved search | `saved_search.created`, `saved_search.updated`, `saved_search.moved`, `saved_search.deleted` | A [saved logs, trace or metrics search](logs-and-traces.md) is saved, renamed, changed (its query, filters or range), moved to another folder or deleted. |
| Observation | `observation.created`, `observation.updated` | An observation is started, or its title or time range changes; a rename also records the title before it. |
| Observation | `observation.moved` | An observation moves to another folder, which shares it with that folder's readers. |
| Observation | `observation.resolved`, `observation.reopened` | An observation is resolved with a summary, or opened again. |
| Observation | `observation.deleted` | An observation is deleted with its blocks, RCA revisions and discussion; the event stays in the log and carries the title the observation had. |
| Observation | `observation.published`, `observation.rca_withdrawn` | An RCA revision is published from an observation, or withdrawn again; both carry the title and the revision number. |
| Observation | `observation.block_added`, `observation.block_updated` | A block is added to an observation's page, or one changes or moves. |
| Observation | `observation.block_kept`, `observation.block_discarded` | An editor keeps a block someone requested, or discards it; its author may discard it too. |
| Observation | `observation.block_removed` | A block is taken off an observation's page. |
| Proposal | `proposal.opened`, `proposal.updated` | A [change is proposed](../concepts/proposals.md), or its author saves again. |
| Proposal | `proposal.merged`, `proposal.discarded` | Someone who can edit merges it, or it is discarded. |
| Run | `run.queued`, `run.started`, `run.released` | An [agent request](chat.md) is queued, a worker starts it, or a stopping server hands it back to the queue. |
| Run | `run.canceling` | The requester canceled it while a worker holds it. |
| Run | `run.succeeded`, `run.failed`, `run.canceled` | A run settles. |
| Run | `run.kept` | The requester kept the build of a stopped run as a version. |
| Thread | `thread.opened`, `thread.replied`, `thread.edited`, `thread.deleted` | A comment thread is started, answered, a message in it is edited, or a message or a whole thread is deleted. |
| Thread | `thread.resolved`, `thread.reopened` | A thread is resolved, by a person or by an agent run that finished the change it asked for, or opened again. |
| Thread | `thread.mentioned` | A message mentions a person or `@observe`, one event for each mention. |

## Admin changes

These events record what an Admin changes about people, keys, integrations and access.
Only an Admin can read them, in the list, in the stream, in a [webhook](webhooks.md) and in [the Audit log](#audit-log) in the web app.

| Target | Event types | Written when |
| --- | --- | --- |
| Person | `user.role_changed` | An Admin sets or clears a person's role override and the person's role changes as a result (pinning the role a person already has records nothing); `data` has the person's `name`, `from_role`, `to_role` and `manual`, which is false when the override was cleared. |
| Person | `user.role_changed` | The sign-in policy changes a person's role, at their login or when the server starts: they joined or left an admin group, or the admin emails or group roles changed. The person is the actor, `manual` is false and `data` has `source`, one of `admin_emails`, `first_user`, `group` or `default`. A person's first login is not a change. |
| Person | `user.signed_out_everywhere` | An Admin signs a person out everywhere; `data` has `name`, `sessions` and `grants`, how many sessions and connected agents ended. |
| Person | `user.invited`, `user.invite_updated`, `user.invite_withdrawn` | An Admin [invites](../teams/people.md) an email, invites it again with another role, or withdraws the invite; `data` has the `email`, the invite has the `role` and an update has `role` as `{from, to}`. The target id is the invite. Inviting an email again with the role it already has records nothing, and a first sign-in that uses the invite is not a change. |
| Person | `user.deactivated`, `user.reactivated`, `user.removed` | An Admin deactivates, reactivates or removes a person; `data` has the person's `name`, and a deactivation also `sessions` and `grants`, how many sessions and connected agents ended. Deactivating someone already deactivated, or reactivating someone active, records nothing. |
| Service account | `service_account.created`, `service_account.updated`, `service_account.deleted` | An Admin creates a service account, changes its role or whether it takes runs, or deletes it with its keys; an update lists each changed field as `{from, to}`. |
| Service account | `service_account.key_created`, `service_account.key_revoked` | An Admin creates or revokes a key; `data` has the key's `key_id` and `key_name`, never the key. |
| Webhook | `webhook.created`, `webhook.updated`, `webhook.secret_rotated`, `webhook.deleted` | An Admin creates, changes, rotates the secret of or deletes a webhook; `data` has `name` and `host`, and an update has `changes` with `{from, to}` for each of `name`, `host`, `enabled` and `event_types` that changed, plus `url_changed` when the address changed. The address path and the secret are never recorded. |
| Outbound domain | `outbound_domain.added`, `outbound_domain.removed` | An Admin allows or removes an outbound domain, including the Grafana host that connecting Grafana allows; `data` has the `domain`. |
| Connected agent | `mcp_client.connected`, `mcp_client.disconnected` | A person connects an agent over MCP, or the connection ends; `data` has the `client`, the person's `person_id` and `person_name` and the grant's `mode`, and a disconnect adds the `reason`: `person`, `admin`, `client` (the agent revoked its own token), `code_reuse` or `refresh_reuse` (the server revoked it after a token was replayed). The actor is the person, or the Admin who disconnected it. Never a token. |
| Grafana connection | `grafana_connection.created`, `grafana_connection.updated`, `grafana_connection.deleted` | An Admin connects, changes or removes a Grafana connection; `data` has `name` and `host`, and an update has `changes` with `{from, to}` for each of `name` and `host` that changed, and `token_changed` when the token was replaced. The token is never recorded. |
| Datasource | `datasource.created`, `datasource.updated`, `datasource.audience_changed`, `datasource.deleted` | An Admin adds, changes or deletes a datasource in the app, or changes who can query it; `data` has `uid`, `type`, `name` and `host`, the address's host and port without its path, an update has `changes` with `{from, to}` for each of `name` and `host` that changed, plus `address_changed`, `settings_changed` and `credentials_changed` when they did, and an audience change has `from` and `to`, each `everyone` or `admins`. A change of the audience and of anything else records both events. A credential is never recorded. Datasources defined in the datasources file are not recorded. |

Each event has an `actor`, the Admin, with the same `label` and origin as any other event, so an event made through a key says which key and an event made over MCP says which client.
`data` never holds a secret: a key, a webhook secret or a Grafana token appears only as the fact that it was created, rotated or replaced.

Every event has a `seq`, a `type`, a `target` (its `kind` and `id`), the `dashboard_id` and `folder_id` it belongs to, an `actor`, a `data` object and `created_at`.
The `actor` is the same object the rest of the API uses, with a `label` that is the one text to show for who made the change.

## What data holds

`data` holds ids, numbers, enums, titles and the version summary, so a saved version reads `{"version": 12, "parent_version": 11, "title": "Checkout", "summary": "Add a latency panel"}`.
A saved search event adds its `kind`, `logs`, `traces` or `metrics`, and a move adds `from_folder_id` and `to_folder_id`.
An observation event adds its `title`, and a block event the `block_id` and `block_kind`, with the block's `state` (`live` or `requested`) on `observation.block_added`; block text, summaries and snapshot data are never in an event.
It never holds a prompt, a comment, a finding's text, a query or Spec JSON, and an annotation event holds only the 80-character `title`, never the full text.
A thread event's `target` is the thread, and its `data` has `object`, the dashboard, annotation, observation or proposal the thread is on, as `{"kind": "dashboard", "id": "..."}`.
`thread.opened` also carries the `title` of the dashboard or observation, copied when the thread opens so the row still names it after the object is deleted.
It adds `version` and `panel_id` when a dashboard thread opens, `message_id` for a reply, an edit or a mention, `resolved_by_version` when a resolve names the version that settled it, and `mentioned` (`kind`, `id`, `name`) on a mention.

A run event adds the run's `intent`, and `run.succeeded` adds `result_version` when it saved a version and `cost_usd` when the cost is known.
The `run.succeeded` of a [health repair](dashboard-health.md) that saved a version also adds `dashboard_title` and `panels_repaired`, the number of panels the repair named that the saved version changed and no longer lists as unverified; panels that were deleted, left unchanged or still fail are not counted.
To see more of the change, read the object through its own endpoint, which checks your access again.

## Audit log

An Admin reads the audit trail in the web app under **Audit** in the sidebar's Workspace group, which other roles do not see.
**Audit** has three pages, listed under it: **Log**, at `/audit`, **Dashboards**, at `/audit/dashboards`, and **Observations**, at `/audit/observations`.
**Dashboards** and **Observations** list what [the Audit folder](#audit-folder) holds, and **Dashboards** keeps the folder's settings button.
The [Logs page](logs-and-traces.md) on the audit datasource is not a page of its own: **Open in Logs** on **Log** opens it, and while it is open **Log** is the highlighted page.
An Editor, Commenter or Viewer who opens `/audit`, `/audit/dashboards` or `/audit/observations` sees a note that only Admins can open the page.

**Log** shows the trail that [Audit queries](../reference/audit-queries.md) describes, not this activity log: every sign-in, read, change, agent tool call and refusal, admin changes among them.
The query box takes the same text as `GET /api/v1/audit/events`, KQL by default, Lucene after `lucene:` and a Query DSL body that starts with `{`, and it names the form it reads and completes field names.
**Search**, or Ctrl+Enter (Cmd+Enter on a Mac), runs it, and a query the server cannot read is shown with the part it stopped at marked and the parser's message under it.
The time range is the last 24 hours unless you pick another, and it spans at most 366 days.

A chart counts the matching events over the range, and the chips under it count the top actors, actions, outcomes, origins and initiators.
Clicking a chip adds its value to a KQL query, or removes it when it is there, and the minus beside it leaves the value out; a Lucene or Query DSL query is edited only as text.
The table lists the matching events newest first, 100 at a time, with **Load older events** for more.
A row opens a panel with every field the event has, the other events of the same request and everything the same agent run did.

**Live**, while the range ends now, adds new matching events at the top as they are recorded.
After a dropped connection it reconnects and continues from the last event it showed; when the server refuses that resume because the event is more than 24 hours old, Live starts over, reloads the list and says so.
After 500 new events the page reloads the list up to now, so the newest of them are in the list and the older ones a click on Load older events away.
It turns itself off with a message when you are no longer an Admin, when you already hold 5 audit streams, or when the query is too slow to follow.
The query, the time range and the open event are in the page's address, so a link opens the same view.
**Open in Logs** opens the same events in the log explorer, the [Logs page](logs-and-traces.md) on the audit datasource, with the page's current query and time range, so the same events can be read there like any OpenSearch logs.
A query longer than the Logs page accepts is left out and Logs opens with the time range alone.

**Export** downloads the matching events as NDJSON or CSV through `GET /api/v1/audit/export`, which records the download as `audit.export` with the query, the window, the format and the row count.
The file downloads in the browser, which shows its progress and can cancel it, so the page never holds the file in memory.
An export stops at 1,000,000 events; when more match, the page says how many before the download starts, and a shorter range exports the rest.
The app's other downloads, such as a dashboard's Spec JSON, a trace's JSON or a panel's CSV, are assembled in the browser from reads that are each recorded, so they have no separate export event.
`observe audit search` and the `search_audit_events` tool read the same trail; see [Audit queries](../reference/audit-queries.md#api-mcp-and-the-cli).

The log used to be Settings, then **Audit log**; `/settings#audit-log` now redirects to `/audit` and keeps its query, time range and open event.

## Audit folder

At first start the server creates the folder Audit, which only Admins can read, with four starter dashboards on the [audit datasource](../reference/audit-queries.md#the-audit-datasource): Audit overview, Sign-ins and access, Agent and MCP activity, and Data access.
The starters are provisioned and read-only in the app, the API, MCP and the CLI, which refuse a change with 409 `provisioned`; comments, pins, annotations and views work as on any dashboard.
The web shows why a starter is read-only and offers Duplicate in the dashboard menu; Ask, annotations and deleting a starter work as on any dashboard, and the agent's changes are refused before anything is queued, a comment that requests a revision included.
To change one, an Admin duplicates it, and the copy is an ordinary dashboard in the Audit folder.
At every start the server compares each starter with the content its release ships, and a starter that differs gets one new version with the summary `Updated to this release`; an unchanged starter gets nothing, and its threads, pins and view counts stay.

A starter that someone changed before it became read-only is kept as a separate dashboard named `<title> (your copy)`, and the starter takes the shipped content.
A starter that was moved out of the Audit folder returns to it, and its copy stays in the folder it had been moved to, or goes to the Audit folder when that folder may not hold its content.
A dashboard of your own that happens to carry a starter's title is never taken over; the starter is added beside it.
A starter an Admin deletes comes back at the next start.
**Audit**, then **Folder**, opens the folder as `/f/<id>` does: its dashboards, then its observations and saved searches.
The page finds the folder by the server's record of it, not by its title, so it keeps working after an Admin renames the folder; `GET /api/v1/folders` marks that folder with `audit: true`.
When a folder named Audit already exists, the server fills it only if it is Admins only and otherwise logs a warning and leaves it alone, and after an Admin deletes the Audit folder it is not created again.
A dashboard, observation or saved search that reads the audit datasource can live only in a folder that only Admins can read.

## Who sees an event

You see an event when any of these holds:

- It belongs to a folder you can view, as [Folders](../teams/folders.md) decides.
- It belongs to no folder and you made the change.
- It belongs to no folder and you are an Admin.

An [admin change](#admin-changes) is the exception to the second rule: only an Admin sees it, even the one who made it once they are no longer an Admin.

An event takes the folder of its dashboard.
A global annotation takes General, which everyone with a role can view, so everyone who can read global annotations sees its events.
An observation, a finding without a dashboard and a saved search belong to their folder, and the discussion of a finding belongs to its observation; a run whose dashboard does not exist yet belongs to no folder.
An unfiled finding no longer exists: the upgrade filed each existing one in the folder of the observation it became.

When a dashboard moves, its earlier events move with it, so only the viewers of the new folder see them.
When a dashboard is deleted, its events stay in the folder it was in.
Access is checked at every read, so a change to your role or to a folder's permissions applies to the next page or the next ping of a stream.

## Order and resuming

Events are ordered by their position in the log, which is not always the order of their `seq`.
An event appears once its own transaction and every older one have committed, so a reader never gets a later event before an earlier one that is still being written, and a slow transaction can hold back newer events for a moment.

The list is newest first, 50 per page by default and at most 100, and `next_cursor` is `null` on the last page.
Pass it back as `cursor` to read the next page.
Narrow the list with `dashboard_id`, `folder_id`, `observation_id`, `actor_id` (a person or a service account), a repeated `type`, `since` and `until` (RFC 3339 times); a parameter that does not parse answers 400.
An Admin sees [admin changes](#admin-changes) in the ordinary list.
`observation_id` returns the events recorded on that observation: its changes, its blocks, its RCA publications and the messages of its discussion.
It narrows what you may see and never widens it, so an observation in a folder you cannot view returns nothing.

```sh
curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
  "$OBSERVE_SERVER/api/v1/activity?limit=20&type=dashboard.version_created&since=2026-11-01T00:00:00Z"
```

## Follow the stream

The stream takes the same filters and sends events oldest first, each as one frame:

```text
id: 412
event: activity
data: {"seq":412,"type":"dashboard.version_created", ...}
```

It starts with `retry: 3000` and `event: ready`, and a `: ping` comment arrives every 15 seconds.
Without `Last-Event-ID` it sends only events appended after it opened, and `Last-Event-ID: 0` starts at the beginning of the log.
`observe activity --follow` sends `0` until it has printed an event, so an event appended between its list and its stream is not missed.
When a connection drops, reconnect with the `id` of the last frame you handled as the `Last-Event-ID` header, and the stream continues with the next event.
If that event is unknown, for example because it was removed, the stream sends `event: reset` after `ready` and continues from the newest event, which tells you some events may have been missed and to read the list again.

At every ping the server checks your credentials, role and folder access again, and it ends the stream when the credential is gone.
It also ends the stream after 30 minutes, so a client reconnects with `Last-Event-ID`.
A person or key may hold 5 streams, and the server 500; beyond that the answer is 429 or 503 `too_many_streams`.

```sh
curl -N -H "Authorization: Bearer $OBSERVE_TOKEN" \
  "$OBSERVE_SERVER/api/v1/activity/stream?type=run.failed"
```

## Retention

The server keeps events for 180 days.
The hourly `activity_prune` job deletes older events in batches, and a busy backlog is cleared over several runs.
Set `SLASHOBSERVE_ACTIVITY_RETENTION` to a Go duration from `24h` to `87600h` to change it; see [Configuration](../reference/configuration.md#activity).
A stream that resumes with the `Last-Event-ID` of a deleted event gets `reset`.

## Read it over MCP

The `list_activity` tool of the [MCP server](mcp.md) reads the same list, newest first, with `dashboard_id`, `folder_id`, `observation_id`, `actor_id`, `types`, `since`, `until`, `limit` and `cursor`, and it needs a Viewer key; an Admin key also gets admin changes.
It lists only what the key may see, by the rules above, and pages with `next_cursor`.
The tool has no stream; an agent calls it again with `since` to see what changed.
`observe activity --observation <observation-id>` takes the same filter in the CLI, beside `--dashboard` and `--folder`, and `--actor <id>` and `--until` match `actor_id` and `until`.

## Send it to another system

An Admin can have the server post events to an address of their choice as signed requests; see [Webhooks](webhooks.md).

## Not recorded

Suggestions, the progress of an agent's build, and a run going back to the queue after a restart are not events.
The dashboard stream behind the web app is a separate stream and still carries comment and revision updates for the dashboard you have open.
