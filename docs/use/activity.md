---
title: Activity
description: Read and follow what changed in your workspace, from dashboard saves to agent runs, through the HTTP API and MCP, with each person seeing only the folders they can view.
---

The activity log records every change to a dashboard, annotation, finding, saved search or observation, and every agent run, in the order it happened.
Each change appends one event in the same transaction as the change, so the log never shows something that did not happen and never misses something that did.
You read it as a list with `GET /api/v1/activity` and follow it live as Server-Sent Events with `GET /api/v1/activity/stream`.
Both need the Viewer role or higher, and both work with a session or a service-account key.

## What is recorded

| Target | Event types | Written when |
| --- | --- | --- |
| Dashboard | `dashboard.created`, `dashboard.version_created`, `dashboard.restored` | A version is saved: the first one, a later one, or a restore of an older one. |
| Dashboard | `dashboard.moved`, `dashboard.deleted` | A dashboard moves to another folder or is deleted. |
| Annotation | `annotation.created`, `annotation.deleted` | An [annotation](annotations.md) is added or removed. |
| Finding | `finding.created`, `finding.pinned` | A finding is saved, or pinned to its dashboard. |
| Finding | `finding.moved` | A finding without a dashboard moves to another folder, which shares it with that folder's readers. |
| Saved search | `saved_search.created`, `saved_search.updated`, `saved_search.moved`, `saved_search.deleted` | A [saved logs or trace search](logs-and-traces.md) is saved, renamed, moved to another folder or deleted. |
| Observation | `observation.created`, `observation.updated` | An observation is started, or its title or time range changes. |
| Observation | `observation.moved` | An observation moves to another folder, which shares it with that folder's readers. |
| Observation | `observation.resolved`, `observation.reopened` | An observation is resolved with a summary, or opened again. |
| Observation | `observation.published` | An RCA is published from an observation. |
| Observation | `observation.block_added`, `observation.block_updated` | A block is added to an observation's page, or one changes or moves. |
| Observation | `observation.block_kept`, `observation.block_discarded` | An editor keeps a block someone requested, or discards it; its author may discard it too. |
| Observation | `observation.block_removed` | A block is taken off an observation's page. |
| Run | `run.queued`, `run.started`, `run.released` | An [agent request](chat.md) is queued, a worker starts it, or a stopping server hands it back to the queue. |
| Run | `run.canceling` | The requester canceled it while a worker holds it. |
| Run | `run.succeeded`, `run.failed`, `run.canceled` | A run settles. |
| Run | `run.kept` | The requester kept the build of a stopped run as a version. |
| Thread | `thread.opened`, `thread.replied`, `thread.edited` | A comment thread is started, answered or a message in it is edited. |
| Thread | `thread.resolved`, `thread.reopened` | A thread is resolved, by a person or by an agent run that finished the change it asked for, or opened again. |
| Thread | `thread.mentioned` | A message mentions a person or `@observe`, one event for each mention. |

Every event has a `seq`, a `type`, a `target` (its `kind` and `id`), the `dashboard_id` and `folder_id` it belongs to, an `actor`, a `data` object and `created_at`.
The `actor` is the same object the rest of the API uses, with a `label` that is the one text to show for who made the change.

## What data holds

`data` holds ids, numbers, enums, titles and the version summary, so a saved version reads `{"version": 12, "parent_version": 11, "title": "Checkout", "summary": "Add a latency panel"}`.
A saved search event adds its `kind`, `logs` or `traces`, and a move adds `from_folder_id` and `to_folder_id`.
An observation event adds its `title`, and a block event the `block_id` and `block_kind`; block text, summaries and snapshot data are never in an event.
It never holds a prompt, a comment, annotation or finding text, a query or Spec JSON.
A thread event's `target` is the thread, and its `data` has `object`, the dashboard, finding, annotation or observation the thread is on, as `{"kind": "dashboard", "id": "..."}`.
It adds `version` and `panel_id` when a dashboard thread opens, `message_id` for a reply, an edit or a mention, `resolved_by_version` when a resolve names the version that settled it, and `mentioned` (`kind`, `id`, `name`) on a mention.
To see more of the change, read the object through its own endpoint, which checks your access again.

## Who sees an event

You see an event when any of these holds:

- It belongs to a folder you can view, as [Folders](../teams/folders.md) decides.
- It belongs to no folder and you made the change.
- It belongs to no folder and you are an Admin.

An event takes the folder of its dashboard.
A global annotation takes General, which everyone with a role can view, so everyone who can read global annotations sees its events.
A finding without a dashboard and a saved search belong to their folder; an unfiled finding and a run whose dashboard does not exist yet belong to no folder.

When a dashboard moves, its earlier events move with it, so only the viewers of the new folder see them.
When a dashboard is deleted, its events stay in the folder it was in.
Access is checked at every read, so a change to your role or to a folder's permissions applies to the next page or the next ping of a stream.

## Order and resuming

Events are ordered by their position in the log, which is not always the order of their `seq`.
An event appears once its own transaction and every older one have committed, so a reader never gets a later event before an earlier one that is still being written, and a slow transaction can hold back newer events for a moment.

The list is newest first, 50 per page by default and at most 100, and `next_cursor` is `null` on the last page.
Pass it back as `cursor` to read the next page.
Narrow the list with `dashboard_id`, `folder_id`, a repeated `type` and `since` (an RFC 3339 time); a parameter that does not parse answers 400.

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

The `list_activity` tool of the [MCP server](mcp.md) reads the same list, newest first, with `dashboard_id`, `folder_id`, `types`, `since`, `limit` and `cursor`, and it needs a Viewer key.
It lists only what the key may see, by the rules above, and pages with `next_cursor`.
The tool has no stream; an agent calls it again with `since` to see what changed.

## Send it to another system

An Admin can have the server post events to an address of their choice as signed requests; see [Webhooks](webhooks.md).

## Not recorded

Suggestions, the progress of an agent's build, and a run going back to the queue after a restart are not events.
The dashboard stream behind the web app is a separate stream and still carries comment and revision updates for the dashboard you have open.
