---
title: Activity
description: Read and follow what changed in your workspace, from dashboard saves to agent runs, through the web app, the CLI, MCP and the HTTP API, with each person seeing only the folders they can view.
---

The activity log lists everything that changed in your workspace, in the order it happened: dashboard saves, comments, agent runs and, for Admins, changes to people and access.
Use it to see who changed what and when, to watch an agent run fail, or to send those events to another system.

A change and its event are written in the same transaction, so the log never shows something that did not happen and never misses something that did.
You see only the events of folders you can view; [Who sees an event](#who-sees-an-event) has the rules.

## See what changed

In the web app, choose **Activity** at the top right of a dashboard to see its comments, agent requests and saved versions.
[The activity timeline](review.md#the-activity-timeline) describes that tab.
An Admin reads the audit trail under **Audit**; see [Audit log](#audit-log).

To read the log itself, use the CLI, MCP or the HTTP API.
They need the Viewer role or higher.
The HTTP API accepts a session or a service-account key, and the CLI and MCP accept a key.

1. Sign in once with a service-account key, as [CLI](../use/cli.md#sign-in) shows.
2. List the newest events:

   ```sh
   observe activity --since 24h
   ```

   You see one line per event, oldest first, with the time, who did it and what happened:

   ```text
   2026-11-01T10:00:00Z  Observe Agent for Sam  run failed (no_progress)
   2026-11-01T10:05:09Z  Sam  saved version 12 of "Checkout"
   ```

3. Narrow the list with a flag, for example `--dashboard <dashboard-id>`, `--type run.failed` or `--actor <person-id>`.
4. For the raw JSON, call the HTTP API:

   ```sh
   curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
     "$OBSERVE_SERVER/api/v1/activity?limit=20&type=dashboard.version_created&since=2026-11-01T00:00:00Z"
   ```

   The answer is `{"activity": [...], "next_cursor": ...}`, newest event first.

`observe activity --json` prints the same response unchanged.
An empty log prints `No activity yet`.

## Read the next page

The HTTP list is newest first, 50 events per page by default and at most 100.
`next_cursor` is `null` on the last page.
Pass a `next_cursor` back as `cursor` to read the next page.
In the CLI, `--limit` takes 1 to 100 (20 by default), `--cursor` continues from a `next_cursor`, and `--all` reads every page as one list.

## Narrow the list

The list takes these query parameters, and a value that does not parse answers 400:

| Parameter | Matches |
| --- | --- |
| `dashboard_id` | Events of one dashboard. |
| `folder_id` | Events in one folder. |
| `observation_id` | Events of one observation. |
| `actor_id` | A person or a service account. |
| `type` | One event type, repeated for several. |
| `since` | An RFC 3339 time. |
| `until` | An RFC 3339 time. |

`observation_id` returns the events recorded on that observation: its changes, its blocks, its RCA publications and the messages of its discussion.
It narrows what you may see and never widens it, so an observation in a folder you cannot view returns nothing.

In the CLI, `--dashboard`, `--folder`, `--observation` and `--actor` take ids, and `--type` repeats.
`--since` and `--until` take a duration such as `24h` or an RFC 3339 time.

## Follow events as they happen

`GET /api/v1/activity/stream` sends new events as Server-Sent Events, oldest first.
It takes the same filters as the list.

1. Open the stream:

   ```sh
   curl -N -H "Authorization: Bearer $OBSERVE_TOKEN" \
     "$OBSERVE_SERVER/api/v1/activity/stream?type=run.failed"
   ```

2. Wait for `retry: 3000` and `event: ready`.
   Each new event then arrives as one frame:

   ```text
   id: 412
   event: activity
   data: {"seq":412,"type":"dashboard.version_created", ...}
   ```

3. Handle each frame, and keep its `id`.
4. When the connection drops, reconnect with that `id` as the `Last-Event-ID` header.
   The stream continues with the next event.

From the CLI, `observe activity --follow --type run.failed` does the same.
It prints the newest events first, then each new one until you press Ctrl-C.
[CLI](../use/cli.md#activity) describes its reconnect behavior.

### What the stream sends

A `: ping` comment arrives every 15 seconds.
Without `Last-Event-ID` the stream sends only events appended after it opened, and `Last-Event-ID: 0` starts at the beginning of the log.
`observe activity --follow` sends `0` until it has printed an event, so an event appended between its list and its stream is not missed.
An event can arrive up to about a second after its change.

If the server does not know the `Last-Event-ID`, for example because it was pruned, the stream sends `event: reset` after `ready`.
It then continues from the newest event.
A reset means some events may have been missed, so read the list again.
A value that is not a non-negative integer answers 400.

### When the stream ends

At every ping the server checks your credentials, role and folder access again.
It ends the stream when the credential is gone.
It also ends the stream after 30 minutes, so a client reconnects with `Last-Event-ID`.
A person or key may hold 5 streams, and the server 500.
The web app holds at most one activity stream per tab: the inbox badge, an open observation and the dashboards list share it.
Beyond that the answer is 429 or 503 `too_many_streams`.

## Read the log over MCP

The `list_activity` tool of the [MCP server](mcp.md) reads the same list, newest first.
It takes `dashboard_id`, `folder_id`, `observation_id`, `actor_id`, `types`, `since`, `until`, `limit` (1 to 100, 50 by default) and `cursor`.
It needs a Viewer key, lists only what the key may see, and pages with `next_cursor`.
An Admin key also gets the [admin changes](#admin-changes).

The tool has no stream.
An agent calls it again with `since` to see what changed.

## Send events to another system

An Admin can have the server post events to an address of their choice as signed requests.
See [Webhooks](webhooks.md).

## Audit log

The audit log is a separate trail from the activity log, and only an Admin reads it.
In the web app, choose **Audit** in the sidebar's Workspace group.
An Editor, Commenter or Viewer who opens one of its pages sees a note that only Admins can open the page.

**Audit** has three pages.

- **Log**, at `/audit`, searches the trail.
- **Dashboards**, at `/audit/dashboards`, lists what [the Audit folder](#audit-folder) holds, including its saved searches, and keeps the folder's settings button.
- **Observations**, at `/audit/observations`, lists the observations in that folder.

**Log** shows the trail that [Audit queries](../reference/audit-queries.md) describes, not this activity log.
It holds every sign-in, read, change, agent tool call and refusal, admin changes among them.

### Search the trail

1. Choose **Audit**, then **Log**.
2. Type a query in the query box, or leave it empty.
   The box takes the same text as `GET /api/v1/audit/events`: KQL by default, Lucene after `lucene:`, and a Query DSL body that starts with `{`.
   It names the form it reads and completes field names.
3. Pick a time range.
   It is the last 24 hours unless you pick another, and it spans at most 366 days.
4. Choose **Search**, or press Ctrl+Enter (Cmd+Enter on a Mac).
   A query the server cannot read is shown with the part it stopped at marked and the parser's message under it.
5. Open a row to see every field of the event, the other events of the same request and everything the same agent run did.

A chart counts the matching events over the range.
The chips under it count the top actors, actions, outcomes, origins and initiators.
Clicking a chip adds its value to a KQL query, or removes it when it is there, and the minus beside it leaves the value out.
A Lucene or Query DSL query is edited only as text.
The table lists the matching events newest first, 100 at a time, with **Load older events** for more.

The query, the time range and the open event are in the page's address, so a link opens the same view.
`observe audit search` and the `search_audit_events` tool read the same trail; see [Audit queries](../reference/audit-queries.md#api-mcp-and-the-cli).

### Watch the trail live

Turn on **Live**, while the range ends now, to add new matching events at the top as they are recorded.
After a dropped connection it reconnects and continues from the last event it showed.
When the server refuses that resume because the event is more than 24 hours old, Live starts over, reloads the list and says so.
After 500 new events the page reloads the list up to now, so the newest of them are in the list and the older ones a click on Load older events away.
Live turns itself off with a message when you are no longer an Admin, when you already hold 5 audit streams, or when the query is too slow to follow.

### Open the same events in Logs

**Open in Logs** opens the same events in the log explorer, the [Logs page](logs-and-traces.md) on the audit datasource.
It carries the page's current query and time range, so you can read the events there like any OpenSearch logs.
A query longer than the Logs page accepts is left out, and Logs opens with the time range alone.
That page is not part of **Audit**: its breadcrumb reads Audit, Log, Explorer, and while it is open **Log** is the highlighted page.

### Export the trail

**Export** downloads the matching events as NDJSON or CSV through `GET /api/v1/audit/export`.
The download records an `audit.export` event with the query, the window, the format and the row count.
The browser shows the download's progress and can cancel it, so the page never holds the file in memory.
An export stops at 1,000,000 events.
When more match, the page says how many before the download starts, and a shorter range exports the rest.

The app's other downloads, such as a dashboard's Spec JSON, a trace's JSON or a panel's CSV, are assembled in the browser from reads that are each recorded.
They have no separate export event.

The log used to live in Settings, under **Audit log**.
`/settings#audit-log` now redirects to `/audit` and keeps its query, time range and open event.

## Audit folder

The server creates a folder named Audit at its first start.
Only Admins can read it.
It holds four starter dashboards on the [audit datasource](../reference/audit-queries.md#the-audit-datasource): Audit overview, Sign-ins and access, Agent and MCP activity, and Data access.

To open the folder, choose **Audit**, then **Folder**.
It opens as `/f/<id>` does, with its dashboards, then its observations and saved searches.

### Change a starter dashboard

The starters are provisioned, so they are read-only in the app, the API, MCP and the CLI.
Those refuse a change with 409 `provisioned`.

1. Open the starter.
   The web app says why it is read-only.
2. Choose **Duplicate** in the dashboard menu.
3. Edit the copy.
   It is an ordinary dashboard in the Audit folder.

Comments, pins, annotations, views, Ask and deleting a starter work as on any dashboard.
The agent's changes to a starter are refused before anything is queued, a comment that requests a revision included.

### How starters stay current

At every start the server compares each starter with the content its release ships.
A starter that differs gets one new version with the summary `Updated to this release`.
An unchanged starter gets nothing, and its threads, pins and view counts stay.

- A starter that someone changed before it became read-only is kept as a separate dashboard named `<title> (your copy)`, and the starter takes the shipped content.
- A starter that was moved out of the Audit folder returns to it.
  Its copy stays in the folder it had been moved to, or goes to the Audit folder when that folder may not hold its content.
- A dashboard of your own that happens to carry a starter's title is never taken over, and the starter is added beside it.
- A starter an Admin deletes comes back at the next start.

### Rules for the folder

The page finds the folder by the server's record of it, not by its title, so it keeps working after an Admin renames the folder.
`GET /api/v1/folders` marks that folder with `audit: true`.
When a folder named Audit already exists, the server fills it only if it is Admins only, and otherwise logs a warning and leaves it alone.
After an Admin deletes the Audit folder, the server does not create it again.
A dashboard, observation or saved search that reads the audit datasource can live only in a folder that only Admins can read.

## Reference

### Event fields

Every event has these fields.

| Field | Holds |
| --- | --- |
| `seq` | The event's id. |
| `type` | The event type. |
| `target` | The `kind` and `id` of what changed. |
| `dashboard_id` | The dashboard it belongs to, or null. |
| `folder_id` | The folder it belongs to, or null. |
| `actor` | Who made the change. |
| `data` | Details of the change. |
| `created_at` | When it was recorded. |

The `actor` is the same object the rest of the API uses.
Its `label` is the one text to show for who made the change.
An event made through a key says which key, and an event made over MCP says which client.

### What is recorded

| Target | Event types | Written when |
| --- | --- | --- |
| Dashboard | `dashboard.created`, `dashboard.version_created`, `dashboard.restored` | A version is saved, or an older one is restored. |
| Dashboard | `dashboard.moved`, `dashboard.deleted` | A dashboard moves folder or is deleted. |
| Annotation | `annotation.created`, `annotation.updated`, `annotation.deleted` | An [annotation](annotations.md) is added, edited or removed. |
| Public link | `public_link.created`, `public_link.revoked` | A [public link](public-links.md) is created or revoked. |
| Access request | `access_request.opened`, `access_request.granted` | A Viewer asks to comment, or a change lets them. |
| Finding | `finding.created`, `finding.pinned` | A finding is saved, or pinned to its dashboard. |
| Finding | `finding.moved` | Only old events; the API that moved findings is removed. |
| Saved search | `saved_search.created`, `saved_search.updated`, `saved_search.moved`, `saved_search.deleted` | A [saved search](logs-and-traces.md) is saved, changed, moved or deleted. |
| Observation | `observation.created`, `observation.updated` | An observation starts, or its title or time range changes. |
| Observation | `observation.moved` | An observation moves to another folder. |
| Observation | `observation.resolved`, `observation.reopened` | An observation is resolved with a summary, or opened again. |
| Observation | `observation.deleted` | An observation is deleted. |
| Observation | `observation.published`, `observation.rca_withdrawn` | An RCA revision is published or withdrawn. |
| Observation | `observation.block_added`, `observation.block_updated` | A block is added, or one changes or moves. |
| Observation | `observation.block_kept`, `observation.block_discarded` | A requested block is kept or discarded. |
| Observation | `observation.block_removed` | A block is taken off the page. |
| Proposal | `proposal.opened`, `proposal.updated` | A [change is proposed](../concepts/proposals.md), or its author saves again. |
| Proposal | `proposal.merged`, `proposal.discarded` | A proposal is merged or discarded. |
| Run | `run.queued`, `run.started`, `run.released` | An [agent request](chat.md) is queued, started or handed back. |
| Run | `run.canceling` | The requester canceled a run a worker holds. |
| Run | `run.succeeded`, `run.failed`, `run.canceled` | A run settles. |
| Run | `run.kept` | The requester kept the build of a stopped run. |
| Thread | `thread.opened`, `thread.replied`, `thread.edited`, `thread.deleted` | A comment thread is opened, answered, edited or deleted. |
| Thread | `thread.resolved`, `thread.reopened` | A thread is resolved or opened again. |
| Thread | `thread.mentioned` | A message mentions a person or `@observe`. |

#### Details of some events

- **Annotation.**
  An annotation event holds only a `title`, the first 80 characters of the annotation's text on one line.
  A delete repeats the annotation's `panel_id`, `tags` and times, so a row still says which annotation went.
- **Public link.**
  The event belongs to the dashboard or observation, and it is written when someone who manages it creates or revokes a link.
- **Access request.**
  The event belongs to the dashboard or observation.
  `access_request.opened` is written when a Viewer [asks for comment access](../teams/folders.md#ask-for-comment-access).
  `access_request.granted` is written when someone changes a folder's access or a role so that the requester can comment.
- **Finding.**
  The answer's observation records its own `observation.created` and block events.
  A finding whose run is `restricted`, or whose answer is [kept for what it is about](chat.md#answers-that-read-admin-only-data), records `finding.created` on its observation instead of its dashboard.
  Only that observation's readers then see its title.
- **Moved finding.**
  `finding.moved` was written when a finding without a dashboard moved to another folder through the finding API.
  That API is removed, and only earlier events carry the type.
- **Saved search.**
  `saved_search.updated` covers a rename or a change to the query, filters or range.
- **Observation.**
  `observation.moved` shares the observation with the new folder's readers.
  `observation.deleted` removes the observation with its blocks, RCA revisions and discussion, and the event stays in the log with the title the observation had.
  An editor keeps or discards a block someone requested, and the block's author may discard it too.
- **Proposal.**
  Someone who can edit the dashboard merges a proposal.
- **Run.**
  `run.released` is written when a stopping server hands a run back to the queue.
  `run.canceling` is written while a worker still holds the run.
  `run.kept` is written when the requester keeps the build of a stopped run as a version.
- **Thread.**
  A person or an agent run that finished the change the thread asked for can resolve it.
  `thread.mentioned` is one event for each mention.

### Admin changes

These events record what an Admin changes about people, keys, integrations and access.
Only an Admin can read them, in the list, in the stream, in a [webhook](webhooks.md) and in [the Audit log](#audit-log) in the web app.
Each has an `actor`, the Admin, with the same `label` and origin as any other event.

| Target | Event types | Written when |
| --- | --- | --- |
| Person | `user.role_changed` | A person's role changes. |
| Person | `user.signed_out_everywhere` | An Admin signs a person out everywhere. |
| Person | `user.invited`, `user.invite_updated`, `user.invite_withdrawn` | An Admin [invites](../teams/people.md) an email, changes the invite or withdraws it. |
| Person | `user.deactivated`, `user.reactivated`, `user.removed` | An Admin deactivates, reactivates or removes a person. |
| Service account | `service_account.created`, `service_account.updated`, `service_account.deleted` | An Admin creates, changes or deletes a service account. |
| Service account | `service_account.key_created`, `service_account.key_revoked` | An Admin creates or revokes a key. |
| Webhook | `webhook.created`, `webhook.updated`, `webhook.secret_rotated`, `webhook.deleted` | An Admin creates, changes, rotates or deletes a webhook. |
| Outbound domain | `outbound_domain.added`, `outbound_domain.removed` | An Admin allows or removes an outbound domain. |
| Connected agent | `mcp_client.connected`, `mcp_client.disconnected` | A person connects an agent over MCP, or the connection ends. |
| Grafana connection | `grafana_connection.created`, `grafana_connection.updated`, `grafana_connection.deleted` | An Admin connects, changes or removes a connection. |
| Datasource | `datasource.created`, `datasource.updated`, `datasource.audience_changed`, `datasource.deleted` | An Admin adds, changes or deletes a datasource, or changes who can query it. |

`data` never holds a secret.
A key, a webhook secret or a Grafana token appears only as the fact that it was created, rotated or replaced.

#### Details of admin events

- **Role changed by an Admin.**
  An Admin sets or clears a person's role override, and the person's role changes as a result.
  `data` has the person's `name`, `from_role`, `to_role` and `manual`, which is false when the override was cleared.
  Pinning the role a person already has records nothing.
- **Role changed by the sign-in policy.**
  The policy changes a role at a person's login or when the server starts, because they joined or left an admin group, or the admin emails or group roles changed.
  The person is the actor and `manual` is false.
  `data` has `source`, one of `admin_emails`, `first_user`, `group` or `default`.
  A person's first login is not a change.
- **Signed out everywhere.**
  `data` has `name`, `sessions` and `grants`, how many sessions and connected agents ended.
- **Invites.**
  `data` has the `email`, the invite has the `role`, and an update has `role` as `{from, to}`.
  The target id is the invite.
  Inviting an email again with the role it already has records nothing, and a first sign-in that uses the invite is not a change.
- **Deactivated, reactivated, removed.**
  `data` has the person's `name`, and a deactivation also has `sessions` and `grants`.
  Deactivating someone already deactivated, or reactivating someone active, records nothing.
- **Service accounts.**
  A create has `name`, `role` and `takes_runs`.
  An update has `name` and lists each changed field, `role` or `takes_runs`, as `{from, to}`.
  A delete has `name` and removes the account's keys.
- **Service account keys.**
  `data` has the key's `key_id` and `key_name`, and a create adds `expires_at`.
  It never has the key.
- **Webhooks.**
  `data` has `name` and `host`, and a create adds `event_types`.
  An update has `changes` with `{from, to}` for each of `name`, `host`, `enabled` and `event_types` that changed, plus `url_changed` when the address changed.
  The address path and the secret are never recorded.
- **Outbound domains.**
  `data` has the `domain`, and the target id is a UUID derived from it.
  Connecting Grafana adds its host as an allowed domain, and that is recorded too.
- **Connected agents.**
  `data` has the `client`, the person's `person_id` and `person_name`, and the grant's `mode`.
  A disconnect adds the `reason`: `person`, `admin`, `client` (the agent revoked its own token), `code_reuse` or `refresh_reuse` (the server revoked it after a token was replayed).
  The actor is the person, or the Admin who disconnected it.
  It never holds a token.
- **Grafana connections.**
  `data` has `name` and `host`.
  An update has `changes` with `{from, to}` for each of `name` and `host` that changed, and `token_changed` when the token was replaced.
  The token is never recorded.
- **Datasources.**
  `data` has `uid`, `type`, `name` and `host`, the address's host and port without its path.
  An update has `changes` with `{from, to}` for each of `name` and `host` that changed.
  It adds `address_changed`, `settings_changed` and `credentials_changed` when they did.
  An audience change has `from` and `to`, each `everyone` or `admins`, and a change of the audience and of anything else records both events.
  A credential is never recorded, and datasources defined in the datasources file are not recorded.

### What data holds

`data` holds ids, numbers, enums, titles and the version summary.
A saved version reads `{"version": 12, "parent_version": 11, "title": "Checkout", "summary": "Add a latency panel"}`.
It never holds a prompt, a comment, a finding's text, a query or Spec JSON.
To see more of the change, read the object through its own endpoint, which checks your access again.

- **Dashboard versions.**
  `dashboard.created`, `dashboard.version_created` and `dashboard.restored` have `version`, `title` and `summary`.
  They also have `parent_version`, except on a create, and `restored_from` on a restore.
  A version that a [provisioning file](../operate/provisioning.md#edits-made-in-the-app) saved over an edit made in the app has `replaced_version`, the version that holds the edit.
- **Dashboard moves and deletes.**
  `dashboard.moved` has `title`, `from_folder_id` and `to_folder_id`, and `dashboard.deleted` has `title`.
- **Annotations.**
  `annotation.created` and `annotation.deleted` have the 80-character `title`, `tags`, `time_from`, and `panel_id` and `time_to` when set.
  The `title` is empty when the text has no characters to show.
  `annotation.updated` has `tags`.
- **Public links.**
  They have the `object` (`kind` and `id`), its `title`, the link's `prefix` and `expires_at`, never the token.
- **Access requests.**
  They have the `object` (`kind` and `id`) and its `title`, and the note is never recorded.
  A grant adds `requested_by` (`kind`, `id` and `name`).
- **Findings.**
  `finding.created` has `title`, and `agent_request_id` when the agent saved it.
  `finding.pinned` has `annotation_id`.
  `finding.moved` has `title`, `from_folder_id`, which is null when the finding was in no folder, and `to_folder_id`.
- **Saved searches.**
  They have `title` and `kind`, which is `logs`, `traces` or `metrics`.
  `saved_search.updated` adds `changed`, the parts that changed out of `title`, `query`, `filters` and `range`.
  `saved_search.moved` adds `from_folder_id` and `to_folder_id`.
- **Observations.**
  Every observation event has its `title`, and `observation.updated` adds `from_title` when the title changed.
  `observation.moved` adds `from_folder_id` and `to_folder_id`.
  `observation.published` and `observation.rca_withdrawn` add `revision`.
- **Blocks.**
  A block event has `block_id` and `block_kind`, and `observation.block_added` adds the block's `state`, `live` or `requested`.
  Block text, summaries and snapshot data are never in an event.
- **Proposals.**
  `proposal.opened` and `proposal.updated` have `title`, `revision` and `base_version`.
  `proposal.merged` has `title`, `revision` and `version`, which is null when the change was already in the dashboard.
  `proposal.discarded` has `title` and `revision`.
- **Threads.**
  A thread event's `target` is the thread.
  Its `data` has `object`, the dashboard, annotation, observation or proposal the thread is on, as `{"kind": "dashboard", "id": "..."}`.
  `thread.opened` adds the `title` of the dashboard or observation, copied when the thread opens so the row still names it after the object is deleted.
  It adds `version` and `panel_id` when a dashboard thread opens.
  A reply, an edit, a delete and a mention add `message_id`, and `thread.deleted` names the thread's first message when the whole thread was deleted.
  `thread.resolved` adds `resolved_by_version` when a resolve names the version that settled it.
  A mention adds `mentioned` (`kind`, `id`, `name`).
- **Runs.**
  Every run event has the run's `intent`: `ask`, `apply` or `suggest`.
  `run.queued` adds `continues_id` when the run continues another, and `run.started` and `run.released` add `attempt`.
  `run.failed` adds `failure_code`, and `run.succeeded` and `run.kept` add `result_version` when a version was saved.
  `run.succeeded`, `run.failed` and `run.canceled` add `cost_usd` when the cost is known.
- **Health repairs.**
  The `run.succeeded` of a [health repair](dashboard-health.md) that saved a version also has `dashboard_title` and `panels_repaired`.
  `panels_repaired` counts the panels the repair named that the saved version changed and no longer lists as unverified.
  Panels that were deleted, left unchanged or still fail are not counted.

### Who sees an event

You see an event when any of these holds:

- It belongs to a folder you can view, as [Folders](../teams/folders.md) decides.
- It belongs to no folder and you made the change.
- It belongs to no folder and you are an Admin.

An [admin change](#admin-changes) is the exception to the second rule.
Only an Admin sees it, even the one who made it once they are no longer an Admin.

An event takes the folder of its dashboard.
A global annotation takes General, which everyone with a role can view, so everyone who can read global annotations sees its events.
An observation, a finding without a dashboard and a saved search belong to their folder.
The discussion of a finding belongs to its observation.
A run belongs to no folder when its dashboard does not exist yet, or when it answers a chat question that is not filed in an observation.
An unfiled finding no longer exists: the upgrade filed each existing one in the folder of the observation it became.

When a dashboard moves, its earlier events move with it, so only the viewers of the new folder see them.
When a dashboard is deleted, its events stay in the folder it was in.
Access is checked at every read, so a change to your role or to a folder's permissions applies to the next page or the next ping of a stream.

### Order and resuming

Events are ordered by their position in the log, which is not always the order of their `seq`.
An event appears once its own transaction and every older one have committed.
A reader therefore never gets a later event before an earlier one that is still being written, and a slow transaction can hold back newer events for a moment.

### Retention

The server keeps events for 180 days.
The hourly `activity_prune` job deletes older events in batches of 10,000.
Set `SLASHOBSERVE_ACTIVITY_RETENTION` to a Go duration from `24h` to `87600h` to change the period; see [Configuration](../reference/configuration.md#activity).
A stream that resumes with the `Last-Event-ID` of a deleted event gets `reset`.

### Errors

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_request` | A filter, `limit`, `cursor` or `Last-Event-ID` is not valid. |
| 403 | `forbidden_role` | Your role does not allow reading the log. |
| 429 | `too_many_streams` | You hold 5 streams already. |
| 503 | `too_many_streams` | The server holds 500 streams already. |
| 503 | `shutting_down` | The server is stopping. |

### Not recorded

Suggestions and the progress of an agent's build are not events.
The dashboard stream behind the web app is a separate stream.
It still carries comment and revision updates for the dashboard you have open.
