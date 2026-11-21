---
title: MCP
description: Connect Claude Code, OpenCode, Cursor and other MCP clients to the server.
---

The server exposes its tools to MCP clients over Streamable HTTP at `<public url>/mcp`, so Claude Code, OpenCode, Cursor and other clients can build and edit dashboards with their own model.
`GET /api/v1/instance` returns the exact address as `mcp_url` next to `public_url`; it needs a signed-in session or a key.

Requests must carry `Authorization: Bearer <token>`, where the token is either a person's OAuth access token (the default for people, see [Connect your own agent](#connect-your-own-agent)) or a [service-account key](../operate/service-account-keys.md) (for headless bots, see [Keys for bots](#keys-for-bots)).
Browser sessions are not accepted on `/mcp`, and an OAuth access token is accepted nowhere else.
A request without a token, or with one the server refuses, gets `401` with `WWW-Authenticate: Bearer realm="slashobserve", resource_metadata="<public url>/.well-known/oauth-protected-resource/mcp"`, which is how an MCP client finds the sign-in; a refused token adds `error="invalid_token"`.

The endpoint is stateless, refuses an `Origin` that is not `SLASHOBSERVE_PUBLIC_URL`, answers `413` above 8 MiB, `429` above 8 concurrent requests per key or person and `503` above 32 across all callers.
Those three, and a refused JSON-RPC batch (one request per POST), come back as JSON-RPC error bodies with the short code in `error.data.code`, so a client SDK backs off on `Retry-After` instead of reporting a broken transport.
`tools/list` has no pages: a `cursor` is ignored.
For a key, `initialize` returns an `Mcp-Session-Id` that only carries the client's self-reported name and holds no state.
For an OAuth client, the name it registered under is used and `clientInfo` is ignored.
Each version, agent request, finding and annotation records its origin (`web`, `cli`, `mcp` or `api`) and, for `mcp`, that client name, and the web app, the CLI and the API show the author as `<name> (via <client>)`.

Each tool has a lowest role, which the server checks when a client calls it, using the role of the key's service account, or of the person for an OAuth client:

| Tool | Lowest role |
| ---- | ----------- |
| `add_block` | Viewer |
| `aggregate_audit_events` | Admin |
| `apply_suggested_change` | Viewer |
| `archive_run` | Viewer |
| `arrange_blocks` | Viewer |
| `cancel_run` | Viewer |
| `claim_run` | Viewer |
| `clear_home_dashboard` | Viewer |
| `compare_traces` | Viewer |
| `create_annotation` | Viewer |
| `create_comment` | Viewer |
| `create_dashboard` | Viewer |
| `create_folder` | Viewer |
| `create_observation` | Viewer |
| `diff_dashboard_versions` | Viewer |
| `discard_block` | Viewer |
| `discard_proposal` | Viewer |
| `draft_query` | Viewer |
| `duplicate_dashboard` | Viewer |
| `edit_comment` | Viewer |
| `finish_run` | Viewer |
| `get_audit_event` | Admin |
| `get_audit_histogram` | Admin |
| `get_chat` | Viewer |
| `get_dashboard` | Viewer |
| `get_home_dashboard` | Viewer |
| `get_observation` | Viewer |
| `get_panels` | Viewer |
| `get_proposal` | Viewer |
| `get_rca` | Viewer |
| `get_run` | Viewer |
| `get_run_brief` | Viewer |
| `get_run_build` | Viewer |
| `get_schema` | Viewer |
| `get_spec_guide` | Viewer |
| `get_trace` | Viewer |
| `import_grafana_dashboard` | Viewer |
| `keep_block` | Viewer |
| `lint_query` | Viewer |
| `list_access_requests` | Viewer |
| `list_activity` | Viewer |
| `list_annotations` | Viewer |
| `list_audit_facets` | Admin |
| `list_audit_fields` | Admin |
| `list_chats` | Viewer |
| `list_comments` | Viewer |
| `list_dashboard_health` | Viewer |
| `list_dashboard_versions` | Viewer |
| `list_dashboard_viewers` | Viewer |
| `list_dashboards` | Viewer |
| `list_datasources` | Viewer |
| `list_folders` | Viewer |
| `list_inbox` | Viewer |
| `list_label_values` | Viewer |
| `list_labels` | Viewer |
| `list_mentionable_people` | Viewer |
| `list_metrics` | Viewer |
| `list_observations` | Viewer |
| `list_pinned_dashboards` | Viewer |
| `list_proposals` | Viewer |
| `list_rcas` | Viewer |
| `list_runs` | Viewer |
| `list_saved_searches` | Viewer |
| `list_services` | Viewer |
| `log_context` | Viewer |
| `log_facets` | Viewer |
| `log_patterns` | Viewer |
| `logs_for_trace` | Viewer |
| `make_dashboard` | Viewer |
| `merge_proposal` | Viewer |
| `move_dashboard` | Viewer |
| `move_observation` | Viewer |
| `pin_dashboard` | Viewer |
| `pin_observation` | Viewer |
| `pin_run` | Viewer |
| `publish_rca` | Viewer |
| `query_instant` | Viewer |
| `query_range` | Viewer |
| `recheck_datasource` | Viewer |
| `rename_folder` | Viewer |
| `rename_run` | Viewer |
| `reopen_comment` | Viewer |
| `reopen_observation` | Viewer |
| `reply_to_comment` | Viewer |
| `report_run` | Viewer |
| `request_comment_access` | Viewer |
| `resolve_comment` | Viewer |
| `resolve_observation` | Viewer |
| `restore_dashboard_version` | Viewer |
| `retry_run` | Viewer |
| `run_dashboard_health` | Admin |
| `sample_rows` | Viewer |
| `save_run_answer` | Viewer |
| `save_search` | Viewer |
| `search_audit_events` | Admin |
| `search_workspace` | Viewer |
| `set_home_dashboard` | Viewer |
| `set_inbox_item_done` | Viewer |
| `share_answer` | Viewer |
| `start_run` | Viewer |
| `trace_stats` | Viewer |
| `unpin_dashboard` | Viewer |
| `unpin_run` | Viewer |
| `update_annotation` | Viewer |
| `update_block` | Viewer |
| `update_dashboard` | Viewer |
| `update_observation` | Viewer |
| `update_panels` | Viewer |
| `update_saved_search` | Viewer |

The lowest role is the one the web app needs for the same work, and folder access decides the rest, as in the web app.
A person's client can create and change dashboards in their own My Dashboards, comment where they may comment, and query every datasource, whatever their role.
A key has no My Dashboards and is never on a folder's list, so below Editor it can read and query but not save directly; a save from a Commenter becomes a [proposal](../concepts/proposals.md).

Every refused tool call returns one shape: `code`, a stable word to branch on, and `message`, what to change, plus fields specific to the refusal such as `issues`, `latest_version` or `allowed_actions`.
A call below a tool's role, or a change the folder does not allow, returns the tool error `forbidden_role`; a dashboard or folder the caller cannot see returns `not_found`.
A dashboard the caller can see with a version number that does not exist returns `version_not_found` with a `message` and `latest_version`, so `not_found` always means the dashboard, never the version.

Each tool carries MCP annotations that say what it does: `readOnlyHint` is true for a tool that only reads, and `destructiveHint` is true for one that can overwrite or delete what someone saved, which are `update_dashboard`, `update_panels`, `restore_dashboard_version`, `update_block`, `arrange_blocks`, `update_observation`, `update_annotation`, `update_saved_search`, `edit_comment`, `rename_folder`, `rename_run`, `discard_block`, `discard_proposal`, `merge_proposal`, `set_home_dashboard`, `clear_home_dashboard`, `set_inbox_item_done` and `cancel_run`.
A client that approves only non-destructive tools on its own therefore asks before any of them runs.

`list_datasources`, `list_dashboards`, `get_dashboard`, `query_range` and `query_instant` also publish an `outputSchema`, so a client knows the fields of a result before it calls.
The schema accepts the `{code, message}` body a failed call answers with, because a client that validates results checks a failed one too.
Every other tool publishes none.

A thing's id is named `<thing>_id` and a datasource is named `datasource`, and every tool that takes one also accepts its common alias, so a guess costs no call.
`datasource_uid` is accepted wherever `datasource` is, `id` for `dashboard_id` on a tool named for a dashboard, for `observation_id` on one named for an observation and for `run_id` on one named for a run, and `run_id` for the `id` of `get_run`, `get_run_build`, `cancel_run` and `retry_run`.
`diff_dashboard_versions` also accepts `from_version` and `to_version` for `from` and `to`, and `get_panels` accepts `panel_ids` for `ids`.
A call that sends both names with the same value works, and one that sends them with different values is refused with `invalid_request`.
The names in `tools/list` are the documented ones; a run of the built-in agent takes only those.

A call with a field the tool does not take is refused with `invalid_request` and the list of fields it takes, and a field of the wrong JSON type is refused with the type it needs.
On `initialize` the server sends instructions with the order of work: read `get_spec_guide` once before the first save, find real names with the discovery tools, check each query with `lint_query` and `query_range` or `query_instant`, then save, sending changes with `update_panels`.
Each tool's description and input schema are the text the built-in agent reads, except that MCP clients get the full guide from `get_spec_guide`, `folder_id` on `create_dashboard`, and `dashboard_id` and `parent_version` on `update_dashboard` and `update_panels`.
`get_spec_guide` returns the core of the authoring guide plus one section per datasource type the server has configured, the text the built-in agent reads; an Admin also gets the section on the built-in audit datasource.

`create_dashboard` and `update_dashboard` run every panel query before they save and reject a panel whose query fails or returns no data, as described in [What a save checks](../concepts/agent.md#what-a-save-checks).
`update_dashboard` needs the `dashboard_id` and the `parent_version` you read, and sends the complete document.
If a newer version has been saved since and your save changed none of the panels, variables and settings that version changed, the server merges the two and the result carries `merged_from`, with the `base_version` you read and the `onto_version` it merged onto.
If both changed the same panel, variable or setting, it returns the tool error `version_conflict` with `latest_version` and `conflicts`, one entry for each unit with its `kind`, `id`, `reason` and a `message`, so the client reads the latest version and applies its change again.
`update_dashboard` and `update_panels` save a [proposal](../concepts/proposals.md) instead when the caller can comment but not edit, or when its connection is in propose mode.
The answer is a success, not a tool error: its code is `proposed` and it carries `proposal_id`, `revision`, `opened` (false when the save added a revision to the caller's open proposal) and the dashboard's `url`.
A Viewer still gets `forbidden_role`.
`get_dashboard` returns the Spec at the latest version or at `version`, with `saved_by`, `origin`, `saved_at` and `summary` of that version, and returns `too_large` for a Spec over 128 KiB.

`get_dashboard` also returns `view_count`, the people-days on which the dashboard was opened in the last 30 days, with `window_days`.
`list_dashboards` returns the same `view_count` on each dashboard and takes `order`, `updated` (the default), `views` for the most viewed first or `name` for A to Z by title; pass a `next_cursor` back with the order that produced it.

`update_panels` changes an existing dashboard by sending only what changed, so a one-panel fix costs one panel and not the whole document.
It takes `dashboard_id`, `parent_version` and a `summary`, and a patch with any of these:

- `upsert`: up to 50 complete panels, each replacing the panel with its id or, with a new id, added.
  A panel that leaves out `gridPos` keeps its position, and a row keeps its children, which are upserted by their own ids.
- `delete`: up to 50 panel ids; a row only together with all of its children.
- `layout`: up to 200 moves of existing panels to a `gridPos` or into a row, with `row` set to `""` for the top level.
- `variables`: the complete variable list, which replaces the existing one.
- `dashboard`: the default `time`, the `title`, the `tags` and the `links`.

A key present in `dashboard` replaces that value whole, so `tags` and `links` are not merged item by item and `[]` clears them, and a key left out keeps the current value.
The server merges the patch into the version named by `parent_version` and validates the whole result, so a bad tag or time is rejected with its path in the document.
This call changes only the default range:

```json
{ "dashboard": { "time": { "from": "now-6h", "to": "now" } }, "summary": "Default to the last 6 hours" }
```

`update_panels` runs only the queries it changed, and every query when `time` changes to a different range, as described in [What a save checks](../concepts/agent.md#what-a-save-checks).
A rejection lists `failing_panels`, so the client resends only those panels.
An overlap is reported at the panel the patch upserted or moved, with both panels' positions in the message.
`get_panels` reads the full JSON of up to 20 panels by `ids`, at the latest version or at `version`, together with the variables; ids it did not return are in `missing`, and `truncated` is true when the result was cut to fit.
It returns at least one panel, and a panel too large to return is listed in `too_large`.
`list_metrics`, `list_labels`, `list_label_values` and `get_schema` take an `offset` and answer `truncated`, `truncated_by` (`limit` or `bytes`) and `next_offset`, so a client pages through a long list by passing `next_offset` back as `offset`.
`list_labels` and `list_label_values` also take `from` and `to`, both or neither, so Loki and Tempo answer only the names seen in that window instead of their own default one.

`list_services` lists the services a log or trace datasource has seen over a recent window, and finds which datasources hold a service.
With `datasource` it reads that one source; without it, it asks every Loki, Tempo, ClickHouse, Elasticsearch and OpenSearch source, and ClickHouse answers once for its logs table and once for its traces table.
`signal` (`logs` or `traces`) picks one signal, `window` (such as `15m`, default `1h`, at most `6h`) sets how far back the window ends now, and `service` keeps only the sources that hold a service whose name contains it, case-insensitive.
Each source answers its `field`, the label, column or attribute that holds the service, so a query can filter on it, and a source that does not answer in time says why under `unavailable` while the others still answer.
Answers are cached for about a minute and a source lists at most 200 services, with `truncated` set when there were more.

`recheck_datasource` runs a fresh health check on one datasource instead of reading the cached one and answers its status, and calls within 2 seconds of the last check return that check.

`get_trace` reads one trace by id from a Tempo or ClickHouse datasource: its services, root span, error spans with the message each set, and slowest spans.
A trace over 20,000 spans is cut to its earliest spans and says so.
`trace_stats` counts spans and errors per time bucket (at most 60) and gives p50, p95 and p99 durations and a duration histogram over a TraceQL filter on Tempo or a SQL condition over `otel_traces` on ClickHouse.
`compare_traces` takes up to 50 `selected` trace ids and up to 150 `baseline` ids from a Tempo or ClickHouse datasource and returns the attribute values whose share of traces differs most between the two sets, to explain why some traces are slow or failing.
`log_facets` counts the top values of each label, field or column that a logs query matches on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, with exact counts where the datasource can give them and counts from the newest sampled lines where it cannot.
The built-in agent has these four, and `recheck_datasource` is over MCP only.

`logs_for_trace` reads the log lines that carry a trace id, or one span id, from a Loki, ClickHouse, Elasticsearch or OpenSearch datasource over a window, the same lines the trace view opens from a span, and answers the table summary of `query_instant`.

On a ClickHouse trace datasource `query_instant` also runs your own SQL over `otel_traces`, the query the SQL view of the trace search runs, with the same read-only, time range and row checks; `page_size`, `page` and `sort` page it.
A paged query that selects `Timestamp` comes back as log lines, newest first, with a `TimeNs` column; pass `as_written` with `page_size` to get the rows as the SQL wrote them instead, with its own `ORDER BY`, `LIMIT` and columns.
On any ClickHouse table, `query_instant` without `page_size` and `query_range` refuse a query that does not read the time range through a macro such as `$__timeFilter(Timestamp)`, even one that selects `Timestamp`, because only a paged query is cut to the range by the server; a macro inside a comment does not count.

`query_instant` and `query_range` return `executed`, the text each query ran as after variables and macros were expanded, when it was sent to the datasource, so you can see what a `$job` or a `$__rate_interval` became.

`list_proposals` takes a `dashboard_id`, a `state` (`open` by default, or `merged`, `discarded` or `all`) and a `cursor`, and pages with `next_cursor`.
`get_proposal` returns a proposal with its `spec`, its `changes`, the `conflicts` a merge would hit while it is open and behind the dashboard, the `allowed_actions` of the caller and the field-by-field `diff`, which is left out, with the step saying so, when it would take the result over 128 KiB; a result over 128 KiB without it answers `too_large`.
`merge_proposal` takes the `proposal_id`, the `revision` read with `get_proposal` and, for each conflict, a choice `{ "kind": "panel", "id": "p1", "take": "theirs" }` where `ours` keeps the dashboard's unit and `theirs` takes the proposal's; it needs Edit on the dashboard, runs the queries of the panels it changes like any save, and saves a new version.
It answers `proposal_changed` with the newest `revision` when the author added one, `version_conflict` with `conflicts` when a conflict has no choice, and `invalid_transition` with the `state` and `allowed_actions` when the proposal is already closed.

`discard_proposal` takes the `proposal_id` and an optional `reason` of up to 500 characters that the proposal shows, and its author or someone who may merge it can discard it.
A caller without the access gets `forbidden_role`, and an unknown proposal or one on a dashboard the caller cannot see gets `not_found`.

`list_metrics` also takes `scope`: a series selector such as `{job="checkout"}` on Prometheus, or a table name on ClickHouse.
With `scope` it answers every metric family in that scope in one result: the name, type, unit, label keys and series count of each family, `families_total`, and `grouped`, the families that share a name prefix such as `http_server`.
Label keys that every family has are listed once in `common_labels`, and a scope too large for one result pages with `next_offset` like the other lists.
Runtime families, `up` and the names that start with `go_`, `process_`, `promhttp_` or `scrape_`, are left out of the list and counted in `runtime_families`, unless the scope names `__name__`, as `{__name__=~"go_.+"}` or `go_goroutines{job="checkout"}` do.
`search` and `limit` are refused together with `scope`.

Each datasource type answers these discovery tools:

- Prometheus: `list_metrics`, `list_labels` and `list_label_values`.
- ClickHouse: the same on the OpenTelemetry tables, plus `get_schema` and `sample_rows`.
- Loki: `list_labels` and `list_label_values` for stream labels, narrowed by a stream selector in `metric`; `sample_rows` for up to 20 lines of the last 15 minutes, from every stream or from a stream selector in `table`; and `list_metrics` with a stream selector as `scope`.
- Tempo: `list_labels` and `list_label_values` for TraceQL attributes, and `get_schema` for every service with its span names as `columns` and `rows`, plus `attributes`.
- Elasticsearch and OpenSearch: `list_labels` for the fields you can group or filter by, `list_label_values` for a field's top values over the range, `get_schema` for the concrete indices of the index pattern, or with `table` set to an index name every mapped field with its kind, and `sample_rows` for up to 20 of the newest documents of the index pattern with no `table`.
  `list_metrics` with a `scope` answers an empty inventory there.

`list_metrics` without a `scope` on Loki, Tempo, Elasticsearch or OpenSearch, and `get_schema` on Prometheus or Loki, answer `invalid_request` and name the datasource types they read, instead of an empty list or the datasource's own wording.

On Loki each family of a scoped `list_metrics` is a stream label key, and its `series` counts the streams that carry it.
That inventory, and the empty one of Elasticsearch and OpenSearch, never replaces the one `uncovered` reads.

The server keeps the last scoped inventory of each service account.
After one, the results of `create_dashboard`, `update_dashboard` and `update_panels` carry `uncovered`, up to 50 families of that inventory that no panel query names, and `uncovered_total`, how many there are in all; `uncovered` is `[]` once every family is covered.
Runtime families left out of the inventory are never in `uncovered`.
A histogram, summary or counter counts as covered when a query names any of its series, such as `<name>_bucket` or `<name>_total`.
Save results also carry `notes`, up to 20 hints that never reject a save: a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, and a stat with no `description`.

`list_folders` is also offered to the built-in agent, so a request that names a folder can place the dashboard there.
It lists the folders the key can see, with `can_create` for each, and sets `truncated` when there are more than fit in the result.
For an Admin key each folder also carries `admins_only`, true for a folder only Admins can read; a save that reads an Admins-only datasource, and is refused with `restricted_content`, goes in one of those.
A key below Admin never sees the field, and no tool changes a folder's access: an Admin sets it with `observe folders permissions admins-only`.
`create_folder` takes a `title` (1 to 100 characters, unique without regard to case); a Viewer is refused by the folder rules, an Editor or Admin may create, and the caller manages the folder they create.

`rename_folder` takes a `folder_id` and a `title` and needs the Manage permission on the folder.
Both answer `folder_exists` for a taken title and `rename_folder` answers `general_folder` for the General folder; no tool deletes a folder or changes who may see one, because people do that in the web app or with `observe folders`.
`create_dashboard` takes an optional `folder_id` from that list; without it, a person's dashboard goes to their My Dashboards and a key's to General.
A retry is safe: when the same caller already saved a dashboard with that title and an identical spec in that folder, `create_dashboard` returns it with `replayed` true instead of making a second one.
A `folder_id` the key cannot create in returns `forbidden_role`, and an unknown one `not_found`.
`list_dashboard_versions` and `diff_dashboard_versions` read a dashboard's history, `restore_dashboard_version` saves an earlier version as a new one with the same `parent_version` check as a save, `move_dashboard` moves a dashboard to another folder and `duplicate_dashboard` copies it into `folder_id`, by default the dashboard's own folder when you may create there, else your My Dashboards or General, as the web dialog does.
No tool deletes a dashboard: people do that in the web app, through the HTTP API or with `observe dashboards delete`.
A connection in propose mode cannot restore or move a dashboard directly and gets `proposal_required`; it can save the change with `update_dashboard`, which becomes a proposal.

`pin_dashboard`, `unpin_dashboard` and `list_pinned_dashboards` manage the person's pins, the dashboards in their sidebar; the HTTP API calls them stars (`/dashboards/{id}/star`, `/stars`).
They work only over a person's OAuth connection: a service-account key does not see them in `tools/list`, as it does not see `list_inbox` or `set_inbox_item_done`, and answers `user_session_required` if it calls one.

`get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard` manage the dashboard that opens for the person after sign-in in place of Home; the HTTP API calls it `/me/home-dashboard`.
`get_home_dashboard` returns `status` (`none`, `ok`, `gone` or `unreadable`) and, when it is `ok`, the dashboard.
`set_home_dashboard` needs a dashboard the person can view and answers `not_found` otherwise; setting the same dashboard again changes nothing.
They work only over a person's OAuth connection: a service-account key answers `user_session_required`.

MCP clients can read open comments with `list_comments`, reply with `reply_to_comment` and resolve with `resolve_comment`, passing the version that fixed the comment.
A dashboard made by Claude Code can then be reviewed by the team without a model on the server.

Comments can sit on a dashboard, an annotation, an observation or a proposal.
A `proposal:<uuid>` target is read with View and written with Comment on the proposal's dashboard, and a comment on it has no `panel_id` or `version`.
`list_comments` takes exactly one of `target` and `dashboard_id`: `target` is written `kind:uuid`, for example `observation:7f0c1d2e-5a4b-4c3d-8e9f-0a1b2c3d4e5f`, and `dashboard_id` means the same as `dashboard:<id>`.
Each comment has its `version`, which is null off a dashboard, `edited_at` and its `mentions`, and an `actor` that is the same object the HTTP API returns (`kind`, `id`, `name`, `origin`, `client`, `run_id` and `label`), whose `run_id` is the run that wrote an agent reply and null for a person; `author` stays the plain name, and a top-level `run_id` appears only on an agent reply; a result listed by `target` also names that `target`.
`create_comment` opens a thread on a `target`; on a dashboard it takes an optional `panel_id` and `version`, and defaults to the whole dashboard on the latest version.
`edit_comment` changes the text of a comment the same key wrote, `reopen_comment` reopens a resolved thread, and `reply_to_comment` and `resolve_comment` work on any thread, `resolve_comment` taking `version` only on a dashboard.
A caller with only Comment resolves only a thread they started and gets `not_starter` on any other; a caller who can edit resolves any.
A comment id is the id of any message of the thread, and the first message's id is the thread's.

On an observation, `create_comment` and `reply_to_comment` take `block_id`, the block the message is about from `get_observation`, and omit it for the whole observation; a block of another observation, or a `block_id` on any other target, answers `invalid_request`.
`reply_to_comment` also takes `reply_to_id`, a message of the same thread that the reply answers, which the web app shows quoted.
Every comment the comment tools return carries `block_id` (null for the whole observation and on every other target) and `reply_to` (null when it quotes nothing, else the quoted message's `id`, its `author` as the web app labels it, and an `excerpt` of its first 200 characters).

`list_mentionable_people` takes a `target` and an optional `query` and returns the people who can see it, with the `id` to put in `@[Name](user:<id>)`; it returns at most 20, and `more` is true when others match, so narrow with `query`.
Mentioning a person who does not exist or cannot see the target answers `invalid_mention`, and a mentioned person gets it in their Inbox.
An observation has one discussion, so `create_comment` on an observation that has one adds the comment to it; with `anchor` it opens a thread of its own on that spot instead.
`create_comment` takes `anchor`, the spot a dashboard or observation thread points at (with `block_id` on an observation, a spot in that block), and every comment it returns carries `at`, the spot in words.
Both tools take `client_id`, a UUID you make for the message: sending the same call again returns the message already posted with `replayed` true, and starts nothing.

Write `@observe` to address the built-in agent, `@[Name](grant:<id>)` for one of your own connected agents and `@[Name](service_account:<id>)` for a service account that takes runs: a mention is recorded and appears in the activity log as `thread.mentioned`.
A comment is for everyone and asks no agent unless the call sets `to` to `agent`; `start_runs` is the deprecated spelling of the same.
With `to: agent` the comment asks the one agent it mentions, else the thread's agent, else `@observe`, in the same transaction as the comment, and that agent becomes the thread's agent; a comment for everyone turns the thread's agent off.
The result carries `to`, and `run` (`id`, `status`, `assigned_kind`) or `run_refusal` (`code`, `message`) when the comment was stored without a run.

A dashboard or an observation thread starts a read-only ask, which needs Comment on the dashboard or observation and answers in the thread without changing it, and an annotation or a proposal thread starts none.
A run started this way is checked like one the same person starts in the web app.

An answer on a dashboard may carry `suggested_change`, one sentence on a change that would help, and `apply_suggested_change` (`comment_id` and the answer's `finding_id` from `get_run`) posts "Revise the dashboard: <suggested change>" in that thread as you and starts a run that makes the change: it saves a version when you can edit the dashboard and opens a proposal when you can only comment.
The run is yours and goes to the agent that answered; a repeat call while it is queued or running, or after it succeeded, posts nothing and returns that run with `replayed` true.
An id that is not an answer given in this thread answers `not_found`, an answer without a suggested change `invalid_request`, and the run's own refusals are `too_many_revisions`, `agent_unavailable`, `assignee_unavailable` and `provisioned` on a starter dashboard.
Two agents in one comment answer `too_many_agents` and write nothing, and a grant that is not yours, revoked or expired, a service account that takes no runs, or a grant mention from a key answer `invalid_mention`.
The refusal codes are `mention_target_unsupported`, `agent_unavailable` (also when the thread's agent was disconnected, which clears it), `agent_not_yours` (the thread's agent is someone else's connected agent), `forbidden_role` and `too_many_runs`.
Every opened, replied, edited, resolved and reopened thread appends to the activity log, which `list_activity` reads.
A key needs the Comment ability on the target to write; a target the key cannot see answers `not_found`, and only the author can edit a message (`not_author`).

When a run a comment started ends, whatever the outcome, the server posts one reply in the thread, by the actor the run's agent writes as: "Observe Agent for <person>" for the built-in agent, "<person> (via <client>)" for a connected agent and the service account's own name for a service account; the reply is a status line the server writes, it does not resolve the thread, and `list_comments` shows it like any other message, with `run_id` set to the run that wrote it when the built-in agent did; a reply that could not be posted when the run ended is tried again every minute for up to a day, and a reply a person deleted is never posted again.

`save_search` saves a logs, trace or metrics search with a title, outside an agent run, so people can open it from the logs, traces or metrics page.
It takes `kind` (`logs`, `traces` or `metrics`), `title` and `datasource`, and optionally `query`, `filters`, `from`, `to` and `folder_id`, and returns the saved search with its `url`.
The query is linted as `lint_query` does it, and one with a lint error is refused with `invalid_request` and the lint message, so a saved search never opens on a query that cannot run.
When the lint check itself cannot run, for example because the datasource is unreachable, the search is saved anyway.
A repeat of a search you already saved, with the same title, datasource, query, filters, range and folder, returns that search with `replayed` true and saves no second copy.
`POST /api/v1/saved-searches` and `observe saved-searches save` apply the same two checks to a service-account key, and refuse a lint error with 422 `query_invalid` and answer a repeat with 200 and `replayed` true; a person's browser session saves as before.
A logs search needs a `query` and a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, and its `filters` takes only `columns`; a traces search needs a Tempo or ClickHouse datasource, takes an optional `query`, TraceQL on Tempo and SQL on ClickHouse, which reopens the search in the SQL view, and its `filters` takes `service`, `span`, `min`, `max` and `errors`; a metrics search needs a PromQL `query` and a Prometheus datasource, and its `filters` takes only `mode`, `graph` (the default) or `table`.
`from` and `to` default to `now-1h` and `now`, and are saved as written.

A search is filed in `folder_id`, by default the key's My Dashboards when it has one, else General, and the folder's readers can open it.
A `folder_id` the key cannot save in returns `forbidden_role`, and an unknown one `not_found`; a datasource that does not exist or does not fit the kind returns `invalid_request`.
Check the query with `lint_query` or `draft_query` first, because saving runs no query.
Saving the same search again as the same caller (same kind, title, datasource, query, filters and range) returns the one already saved with `replayed: true`, so a retry makes no duplicate.
No MCP tool renames or deletes a saved search, so people do that in the web app or with the CLI.

`list_saved_searches` lists the saved searches in the folders the key can see, most recently changed first, optionally of one `kind` or in one `folder_id`, with each one's datasource, query, filters, range, folder and `url`.
`update_saved_search` changes what a saved search runs, outside an agent run: pass its `id` from `list_saved_searches` and any of `query`, `filters`, `from` and `to`.
Each one replaces the saved value, and `filters` replaces all the filters, so send every filter to keep; `null` removes them all, and an empty `query` removes the query of a trace search and is refused for a logs search.
The result is checked as a save is, so a rule it breaks returns `invalid_request`; the kind, datasource, title and folder stay, and the tool returns the search with its `url`.
It needs Edit on the search's folder, which a Viewer key lacks (`forbidden_role`), and a search the key cannot see returns `not_found`.
No tool renames, moves or deletes a saved search: people do that on the web or with `observe saved-searches`.

An observation is a page of blocks about one question: Markdown text, panels that run live, and snapshots that freeze a panel's data.
`create_observation` takes a `title`, and optionally a `folder_id` (by default the key's My Dashboards when it has one, else General), a range with `from` and `to`, and up to 10 `blocks`; the observation and its blocks are created together, so when a block is refused nothing is created and the call answers `block_refused` with the `block_index` and the reason.
`get_observation` returns the observation with `allowed_actions` and its blocks in order, each with its `kind`, `state`, `position`, `width`, `height`, `version` and a short view: Markdown clipped to 2,000 characters (`markdown_truncated`), a panel block's `title`, `type`, datasources and its full `panel`, in the shape `update_block` takes, a snapshot's `snapshot_id`; `blocks_truncated` says the rest did not fit, and a block's `allowed_actions` never lists `remove`, because no MCP tool removes a block.
`list_observations` lists the observations in the folders the key can see, most recently changed first, optionally in one `folder_id`, one `state` (`open` or `resolved`) or a `title` that contains the given text, and pages with `next_cursor`.

`add_block` takes `observation_id` and a `kind`: `text` takes `markdown` of at most 4,000 characters, `panel` takes `panel`, optionally `variables` (an object of variable name to a list of values, for each variable the panel's queries use) and its own `from` and `to` (otherwise the range of the observation, else the last hour), and `snapshot` takes `from_block_id`, the panel block to freeze; `width` (`third`, `half`, `two_thirds` or `full`, by default `full`), `height` (120 to 960 pixels, by default the block's own) and `position` apply to every kind, and a field of another kind is refused with `invalid_request`.
A panel that uses a variable with no value in `variables` is refused with `variable_missing`, and the message names the variable.
`get_observation` shows a panel block's `variables` as the values it runs with.
`update_block` takes `block_id`, the `version` you read and the new `markdown`, or the new `panel` with its range, and optionally `width`, `height` (a number, or `null` for the automatic height) or `position`.
A `height` applies to panel and snapshot blocks only, and a number for a text block is refused with `invalid_block`.
`follow_page: true` drops a panel block's own `from` and `to`, so it shows the observation's range again; it goes without `panel`, `markdown`, `from` or `to`.

`arrange_blocks` takes `observation_id` and `blocks`, a list of `block_id` in the order the page should show them, each with an optional `width` and `height` (panel and snapshot blocks only); a block left out keeps its place after the block it followed, and an id that is not a block of the observation refuses the call and names the id.
It needs no `version`, and a layout change never makes a text or panel edit stale, because a block's `version` counts content only.
The last arrangement wins, and it answers with the blocks in their new order.

`resolve_observation` closes an observation with a one-line `summary`.
Every write answers with the ids, the `state` of what it changed, the block `version` and the `url`.

`publish_rca` takes `observation_id` and publishes it as the next immutable RCA revision (see [Observations](observations.md#publish-as-rca)), answering with the `revision`, the number of `blocks` and `snapshots` it holds and the `observation_url`; a failing panel answers `panel_failed` with the block it names, and the other refusals are `rca_limit`, `rca_unchanged`, `observation_changed`, `snapshot_too_large` and `personal_folder`.
It never creates a public link: a person does that, and no tool lists or revokes one either.
`list_rcas` lists an observation's published revisions and `get_rca` reads one, as `GET /api/v1/observations/{id}/rcas` does.
No tool withdraws a revision: people do that in the web app, through the HTTP API or with `observe o withdraw`.

A block written by someone who can only comment arrives as `requested` and says so in `note`; a person who can edit keeps it with `keep_block` or discards it with `discard_block`, and the one who asked for it may discard it too.

`get_observation` and `list_observations` are also offered to the built-in agent, which only reads with them.

A block's `version` is checked by `update_block` and `keep_block`: a stale one answers `version_conflict`, so read the block with `get_observation` and apply the change to the version it names.
A transition the state does not allow, such as keeping a block that is already live, answers `invalid_transition` with the actions allowed now.
Like every agent write, a text block with HTML is refused with `html_not_allowed`, and a panel is checked like the evidence of an investigation's answer: a panel whose query fails or returns no data in its range is refused with its issues.
`update_observation` changes the title or range, `move_observation` moves it to another folder and `reopen_observation` reopens a resolved one.
A new title from `update_observation` also renames the chat the observation shares its title with when you started that chat, and `chat_rename` in its result is `renamed`, or `kept` when the chat's title was left as it is; `rename_run` on that chat's first run renames the observation and answers `observation_rename` the same way ([Chat](chat.md#one-title-for-a-chat-and-its-observation)).

`pin_observation` pins an observation to a dashboard as an annotation, to `dashboard_id` or by default to the dashboard its run was about, and needs edit access to that dashboard; pinning again answers with the same annotation and `created: false`.
`make_dashboard` makes a dashboard in My Dashboards from an observation's live panel blocks, or from one block with `block_id`, with the variables each block carries; making it again returns the dashboard you made first (`replayed: true`) unless `new` is true.
When the observation came from your own chat and that chat is about nothing else, the chat becomes about the new dashboard.
An observation with no dashboard to pin to answers `observation_has_no_dashboard`, and one with no live panel block answers `observation_has_no_panels`.
No tool removes a block: people do that in the web app or through the HTTP API.

No tool queues an investigation on the server: an MCP client investigates with its own model, using the discovery and query tools, and saves the answer into an observation with `create_observation` and `add_block`.

`list_dashboard_viewers` is available over MCP only.
It lists, by name, the other people who have the dashboard open in the web app now, so a client can see who is looking before it changes the dashboard; it needs View, and the list is empty when nobody is looking.

`list_dashboard_health` is available over MCP only.
It lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see, ranked as in [Dashboard health](dashboard-health.md) (failing first, then no data, then duplicate title, then unopened, and within each the most recently opened first), 20 per page by default and at most 50: each with its `problems` (`kind`, and `panel_id`, `panel_title`, `variable`, `code` and `message` where they apply), `checked_at` and `checked_version`, plus the state of the daily check in `job`.
`folder_id` and `kind` narrow the list, and `next_cursor` pages on.
A client repairs a failing panel itself: it reads the panel with `get_panels` and saves the fix with `update_panels`.

`draft_query` is available over MCP only.
It turns a question into a checked LogQL, ClickHouse SQL, KQL, PromQL or TraceQL query, or trace search filters, for one Loki, ClickHouse, Elasticsearch, OpenSearch, Prometheus or Tempo datasource, as [Ask](logs-and-traces.md#ask-in-plain-language) does.
`signal` is `logs`, `traces` or `metrics`; it defaults to the one the datasource holds, and a ClickHouse datasource needs it.
A Prometheus datasource answers with `language` `promql`, evaluated at the end of the range.
It spends at most two of this server's model calls per question, checks the query with the lint and a short sample, and never runs the full query; run the answer with `query_instant`.
A question it cannot turn into a query returns the tool error `cannot_translate` with the reason, and a server with no model returns `agent_unavailable`.

`log_context` reads the log lines just before and just after one line on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, as [Show context](logs-and-traces.md#context-around-a-line) does.
On an Elasticsearch or OpenSearch time field of type `date`, context is exact to the millisecond.
Pass the datasource, the query and the line's `time_ns` from a `query_instant` result as `anchor`; `before` and `after` default to 20 lines each and take at most 100.
The answer holds each side's lines with `time`, `time_ns`, `body` and `labels`, the before side newest first and the after side oldest first, and says when more lines exist within an hour on either side.
The built-in agent has the same tool.

`log_patterns` groups the lines a logs query matches into patterns with counts on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, as [Patterns](logs-and-traces.md#pattern-templates) does: from Loki's pattern index or Elasticsearch's `categorize_text` when it has one, otherwise from the newest 2,000 lines.
OpenSearch has no such aggregation, so the server clusters the newest 2,000 lines there and says `source: server`.
Pass the datasource, the query and optionally `from`, `to` and `limit`, 10 patterns by default and at most 30.
The answer says `source` (`loki`, `elasticsearch` or `server`), `lines_sampled`, `lines_matched` and each pattern's `count`, `sample` cut to 200 bytes, `first`, `last`, `level` and `trend`, the counts of 30 equal buckets over the range, whose width in milliseconds is `trend_step_ms`.
The built-in agent has the same tool.

`list_activity` lists the [activity](activity.md) of the workspace, newest first: dashboard saves, restores, moves and deletes, annotations, findings and agent runs, each with its `actor` and the actor's `label`.
Narrow it with `dashboard_id`, `folder_id`, `observation_id`, `actor_id`, `types`, `since` and `until` (RFC 3339), 50 per page by default and at most 100, and page with `next_cursor`.
It lists only the events of folders the key may see, plus the [admin changes](activity.md#admin-changes) when the key is an Admin's, and it is over MCP only, so the built-in agent does not have it.

The six audit tools read the [audit trail](../reference/audit-queries.md) and need an Admin: `search_audit_events` (20 events a call by default, at most 100, paged with `next_cursor`), `get_audit_event` (one event with the events of its request and agent run), `list_audit_facets`, `get_audit_histogram`, `aggregate_audit_events` and `list_audit_fields`.
They take `query` in KQL, `lucene:` or a Query DSL body, and `since` and `until`, the last 24 hours by default and at most 366 days.
An answer is at most 16 KiB, so a page can hold fewer events than `limit`; it then says `truncated: true` with a note, and `next_cursor` continues it, so an agent that counts events must follow the cursor until it is null.
A `search_audit_events`, `get_audit_event`, `list_audit_facets`, `get_audit_histogram` or `aggregate_audit_events` call is recorded in the trail as `audit.search` with the query and its view (once per key, view and query text in 10 minutes, see [Audit queries](../reference/audit-queries.md)), `list_audit_fields` records nothing, and none of them changes or deletes an event; the export is in the CLI and the API only, because a tool answer holds at most 16 KiB.

`list_inbox` lists the [inbox](inbox.md) of the person who signed in to the connected agent: what asks something of them, with the `reason` and whether it is `unread`, and `unread_count`.
Pass `view` (`for_you`, `proposals`, `mentions` or `everything`, `for_you` by default), `filter` (`open` by default, `unread` for the open items that are unread, `done` for the items marked done), `limit` (50 by default, at most 100) and `cursor` from `next_cursor`.
It answers what `GET /api/v1/inbox` answers, it does not mark anything read, a service-account key gets `user_session_required`, and it is over MCP only, so the built-in agent does not have it.

`set_inbox_item_done` marks one inbox item done by its `seq` from `list_inbox`, or brings it back with `done: false`.
A done item leaves the default list, counts as read and stays done when the inbox is read, as the done button in the web app does; it answers like `PUT /api/v1/inbox/items/{seq}`, and a service-account key gets `user_session_required`.

`request_comment_access` asks for [comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation the caller can view but not comment on, with `object_kind` (`dashboard` or `observation`), `object_id` and an optional one-line `note` of at most 200 characters.
It changes no access and its answer never names who received the request; it answers what `POST /api/v1/access-requests` answers, including `request_open` with `requested_at`, also named in its message, while a request is open, `already_can_comment` and `not_found`.
`list_access_requests` lists the open requests on the folders the caller manages, as `GET /api/v1/access-requests` does, with each request's `folder`; when `folder.general` is true the object is in General, which takes no folder entry, so only a role change or moving the object to another folder first grants it.
Both are over MCP only, so the built-in agent does not have them, and a service-account key may call them.

`get_run` reads one run of the built-in agent by the id of a `run.*` event from `list_activity`.
The answer is the object `GET /api/v1/observe/{id}` returns: its `state`, `result_kind`, `allowed_actions`, `attempt`, the last 20 steps, its usage and, while the run is running, `live_text`, the text the agent is writing now, which is never stored and is `null` once the run ends.
It is over MCP only, and a run is visible to the same people as through the API.
An Admin can read any run; the answer then has `read_only: true` and no allowed actions.
A run from someone else's private chat, other than an investigation or an answer shared into a thread you can read, has `redacted: true`, `read_only: true`, no allowed actions, and its `title`, `prompt`, step messages, `live_text`, `continue_note` and `error` empty, while `failure_code` stays.
`subject_observation_id` names the observation an ask is about, such as one asked from an observation's chat, and stays set after its answer is saved into another observation; it is null when `redacted` or when you cannot open that observation.

When the run ended with an answer to a question, the object carries it as `finding`, whose `observation_id` and `url` are null until the answer is saved as an observation.
Only the person who asked and an Admin can read such an answer.
When a request made on Home asked to change one existing dashboard, the run's `finding` carries `ask_on`, the `dashboard_id` and `title` of that dashboard, and no `ask_on` means the finding names none or several.
To make that change, use `update_panels` on that dashboard yourself, or ask the built-in agent there with `start_run`, sending `kind` `revise`, the finding's `ask_on.dashboard_id` as `dashboard_id` and the run's prompt, or with `observe --revise --dashboard <id>` or `POST /api/v1/observe` with `kind` `revise` and `dashboard_id`.

`get_run_build` reads the dashboard a run has saved so far, the object `GET /api/v1/observe/{id}/build` returns, for the person who started the run and for an Admin.
`start_run`, `cancel_run` and `retry_run` control the built-in agent's runs as the person who connected the client, with their role and folder access, and are over MCP only.
`start_run` takes a `prompt` (1 to 4000 characters) and optionally a `kind`, `ask` (the default) or `revise`, a `dashboard_id`, with `base_version` for a revise, a `chat_id`, one of your chats from `list_chats` whose next turn the run is, and an `anchor`, the spot on the dashboard the prompt is about, shaped as `list_comments` shows it; it queues a run and answers with it at once, and you follow it with `get_run`.
Without a `dashboard_id` an `ask` builds a new dashboard or answers in the chat, and `kind: revise` with a `dashboard_id` changes that dashboard and saves a version.
With `kind: revise`, `applies_finding_id` in place of `prompt` makes the change an answer about that dashboard suggests: the answer's `suggested_change` from `get_run` is the prompt, and while a run applying it is queued or running, or after it succeeded, the call returns your run with `replayed: true` or answers `already_applied` for someone else's.
An `ask` with one of `dashboard_id` or `observation_id` is a [private ask](../concepts/agent.md#private-asks): it only answers, changes nothing, and is read only by you and Admins; `kind: investigate` is refused, because an investigation is a person's decision.

Every run is a turn of a chat: without `chat_id` a new chat starts, and a revise joins your latest chat on that dashboard unless `new_chat` is true, which always starts a new one and is refused with `chat_id` or `follows_id`; `follows_id`, an earlier run of yours with no dashboard, is the deprecated way to name a chat.
Send a new `client_id` (a UUID) with each question: a resend with the same one answers with the run it started and `replayed: true`, and one with another prompt answers `client_id_reused`.
Without `client_id` every call queues another run, so read `get_run` or `list_activity` before asking again; the limits (3 new-dashboard runs per caller, 5 revisions per dashboard, an instance-wide cap) answer `too_many_requests`, `too_many_revisions` and `agent_busy`, and `agent_unavailable` means the server has no model.
`cancel_run` stops a run the caller started and is safe to repeat: a run that is already canceled, succeeded or failed answers with its current state.
`retry_run` starts a succeeded, failed or canceled run again with the same prompt as a new run; a queued or running run, a failed or canceled run that was already retried, and a run you read `redacted`, answers `invalid_transition` with `allowed_actions`.
Investigations are a person's decision, so `start_run` has no way to ask for one and `retry_run` answers `investigation_person_only` for one.
A private ask about a dashboard or an observation is not an investigation, also once its answer is saved as an observation: `retry_run` starts it again as a private ask about the same dashboard or observation, in the same chat.
The run's `investigation` field says which one a run is.

`list_chats` lists your chats, pinned ones first and then the one most recently asked in, 20 per page by default and at most 50, with `scope`, `dashboard_id`, `observation_id`, `pinned` and `q` as `GET /api/v1/chats` takes them, and pages with `next_cursor`; `scope: all` lists every person's chats by their last turn only and needs an Admin.
Each chat carries `dashboard_title`, null when you cannot see its dashboard, and its `observation_id` and its last turn's `subject_observation_id` are null when you cannot open that observation.
`get_chat` reads one chat with its turns oldest first, each the run as `get_run` returns it without steps; when they do not all fit it answers `truncated: true` and a `next_offset` to pass as `offset`.
A chat's owner and Admins read it and nobody else sees it; an Admin reads another person's chat read only, with `owner` and `read_only` saying so; archiving a chat is left to people, with `observe chats archive` or `DELETE /api/v1/chats/{id}`, and an Admin who passes its `chat_id` to `start_run` gets `chat_read_only`, not `not_found`.

`list_runs`, `rename_run`, `pin_run`, `unpin_run` and `archive_run` work the runs the web app lists as chats, with the same rules as `GET /api/v1/observe`, `PATCH /api/v1/observe/{id}`, `DELETE /api/v1/observe/{id}` and `observe requests`, and are over MCP only.
`list_runs` lists the caller's runs newest first with pinned ones on top, 20 per page by default and at most 50, each with its `id`, `title`, `state`, `kind` (`build` or `investigate`), `intent` (`ask`, `apply` or `suggest`), `investigation` (true for an investigation that writes into an observation, false for an ask), `result_kind`, dashboard, observation, `anchor` and `block_id` (the spot your own question points at), `pinned_at`, `created_at` and `allowed_actions`; it does not carry the prompt, which `get_run` returns.
It takes `state` (a list), `dashboard_id`, `pinned`, `scope` (`all` lists everyone's runs and needs the Admin role) and `q`, a word of the title, the prompt or an answer the caller may read, and pages with `next_cursor`.

`observation_id` keeps the runs asked about that observation and the runs that wrote into it.
`anchored: true` keeps the runs that point at a spot, and works only with the default `scope` of `mine`: with `scope: all` it answers `invalid_request`, even for an Admin.

`anchor` and `block_id` are the spot a run points at, shaped as `start_run` takes it, and are filled only for the person who asked the run.
They are `null` for anyone else, such as a reader of a shared answer or an Admin reading a private chat, and for a run that points at nothing.
`get_run` and the turns of `get_chat` carry the same two fields under the same rule.
The first page also carries `summary.total`, the number of runs that match, which counts a chat once per message.
`rename_run` takes a `title` of 1 to 120 characters on one line, or `null` to take it back to the start of the prompt, `pin_run` and `unpin_run` move a run to the top of the list and back, and `archive_run` removes it from the list while `get_run` still reads it and everything it saved stays.
An observation the run saved stays too, because no MCP tool deletes anything; a person deletes it with the chat in the web app, or with `observe requests archive <id> --delete-observation --yes`.
Only the person or key that started a run can rename, pin, unpin or archive it, and anyone else gets `forbidden` on a run they can see on a dashboard and `not_found` on any other, so a chat that only its requester can read never shows it exists; each is safe to repeat.
`run_dashboard_health` is for Admins: it starts the health run as `POST /api/v1/health/run` does, answers at once, starts nothing more while a run is going, and the `job` field of `list_dashboard_health` shows how it went.

`save_run_answer` takes `run_id` and files the answer of that run as an observation, in the folder of the dashboard the chat was about, else in the requester's default folder (a person's My Dashboards, a key's General).
It answers `observation_id`, `url` and `replayed`, true when the answer was already saved.
Until then a run's `result_kind` is `answer` and its `observation_id` is null, and it reads `observation` afterwards, in `get_run`, `list_runs`, `observe requests list --json` and `GET /api/v1/observe`.
Only the one who asked can save it: an Admin gets `forbidden_role`, anyone else and a run without an answer get `no_answer`.

`share_answer` takes `run_id` and shows the answer of a private question (`start_run` with `kind` `ask`) to the people who can read the dashboard or observation it was about.
It opens a comment thread there, at the same panel or block, with the question as your message and the agent's answer as its reply, and on an observation it also writes the answer onto the page; with `thread_id` it adds both to that thread instead, which must be on the same dashboard or observation.
It answers `thread_id`, `message_id` and `replayed`, true when the run was already shared, and a run is shared once.
Only the one who asked can share it, and only when you can comment there: anyone else gets `not_found`, a run that is not a private question gets `not_shareable` and one without an answer gets `no_answer`.
An answer whose evidence reads a datasource only Admins can query is refused with `restricted_content` unless the dashboard or observation is in a folder only Admins can read.
The run and the rest of the chat stay readable only by you and Admins.
Withdrawing a shared answer deletes the shared question and its answer, or the thread the share opened, so no MCP tool does it; the person who asked uses `observe comments withdraw` or `DELETE /threads/{id}/share`.

`search_workspace` runs the [search](search.md) of the workspace: pass `q`, and optionally `kinds` (`dashboard`, `annotation`, `observation`, `comment`, `saved_search`), `limit` (20 by default, at most 50) and `cursor`.
Each result has its `kind`, `id`, `title`, a plain-text `snippet`, the `url`, the `folder`, `updated_at` and, for a comment, the `target` it is on and its `thread_id`.
It finds only what the key may read, and it is over MCP only, so the built-in agent does not have it.

`list_annotations` lists [annotations](annotations.md), such as deploy markers, newest first: the global ones, plus one dashboard's own with `dashboard_id`, narrowed by `from` and `to` (RFC 3339) and `tags`, 50 per page by default and at most 100.
It is the one annotation tool the built-in agent also has, in every run and as the person who asked, so it can see a deploy beside a spike.
`create_annotation` is available over MCP only and adds one, with the fields of `POST /api/v1/annotations`: `text`, `tags`, `time_from` and `time_to`, and `dashboard_id` and `panel_id` to place it on one dashboard or panel.
Without `dashboard_id` the annotation shows on every dashboard.
It needs Edit on the dashboard's folder, and an annotation with no `dashboard_id` needs the Editor role.
`update_annotation` changes the `text`, the `tags` or both of an annotation by `id`; only the caller who added it, or an Admin, may, and its time and dashboard never change.
No tool deletes an annotation; people do that in the web app or through the HTTP API.

MCP clients can also hand the server a Grafana dashboard with `import_grafana_dashboard`: pass the dashboard JSON text, optionally a `folder` title, and a `mapping` from Grafana datasource keys to your datasource uids when the result says they could not be matched; panels whose queries fail are saved as unverified and listed in the result so the client can fix them with `update_dashboard`.
The tool refuses JSON that converts to an HTML text panel, a placeholder panel or a panel action with `import_content_not_allowed`, the same content `update_dashboard` and `create_dashboard` refuse; the message names the panels and asks the agent to remove them or to have a person import the JSON in the web app, where a person can also use `observe import`.
The result names the folder the dashboard went to in `folder`, and `folder_note` says why when it is not the one requested.
A file whose Grafana uid an earlier import already holds, in a dashboard you can see, is not imported twice: the call fails with `already_imported` and `existing` names that dashboard with its `dashboard_id`, `title`, `version`, `url` and `can_update`.
Pass its `dashboard_id` as the tool's `update_dashboard` argument, which is not the `update_dashboard` tool, to save the JSON as its next version (it needs Edit, keeps the folder, and answers `updated: true`), or set `import_as_new` to make another dashboard on purpose.
An update with a file that has another Grafana uid than the dashboard was imported from fails with `grafana_uid_mismatch` and changes nothing.
A retried call of the same agent request still answers with the dashboard it made.
The built-in agent in a run that creates a dashboard imports as before and is not refused, because the person asked it to import that JSON; it may not use `update_dashboard` on this tool, because that run may not save a version of an existing dashboard.

To repair an imported dashboard, read it with `get_dashboard`, fix the panels and variables in its `unverified` list with `update_panels`, and leave placeholder panels to people; the web repair queue does the same through `POST /api/v1/repairs`.
See [Import Grafana dashboards](import-grafana.md).

## Connect your own agent

Add `<public url>/mcp` to an MCP client, approve it once in your browser, and the client acts as you.
The client registers itself, then opens a page in your browser, signed in through your identity provider as for the web app.
That page names the client, shows your name, email and role, shows the host the browser returns to, and asks you to approve or deny.
When the client runs on your own computer the page warns that any program on this computer can ask under any name, so approve only a connection you just started.

After you approve, the client acts with your role and your folder access, re-read on every call, so a role change takes effect at once.
Everything it writes shows as `<your name> (via <client>)`.
The client name is chosen by the client and SlashObserve cannot verify it; it is a label and never changes what the client may do.
Access tokens last 1 hour, and a refresh token lasts 30 days from when it was issued and is replaced each time it is used, so a client that stays in use keeps working.
A connection lasts at most 90 days, after which you approve it again.

You can have at most 20 connected agents, and approving a client you already connected replaces that connection.
An approval the client has not yet finished, which lasts 60 seconds, counts toward the 20.
You can list and disconnect your connected agents over the API with `GET /api/v1/me/grants` and `DELETE /api/v1/me/grants/{id}` (any signed-in person), and an Admin can list and disconnect everyone's with `GET /api/v1/grants` and `DELETE /api/v1/grants/{id}`.
Each connected agent in the list has a `state`, always `active` there because the list leaves out revoked and expired connections; see [States](../reference/states.md).
Service-account keys cannot call any of them.

Disconnect a client in Settings, Connected agents; an Admin sees and disconnects everyone's in Settings, Agent access.
Disconnecting a client also ends a run it already claimed: the run's token stops working at once, and the run ends when its lease lapses.
The [Agents page](agents.md) lists the same connections with their mode and presence, and its **Connect an agent** button shows the Claude Code command.

Each client's setup:

### Claude Code

```sh
claude mcp add --transport http slashobserve https://observe.example.com/mcp
```

Then run `/mcp` inside Claude Code and choose Authenticate, or run `claude mcp login slashobserve`.
`claude mcp logout slashobserve` disconnects it.

### VS Code

Add this to `.vscode/mcp.json`, choose Start on the server, and allow the sign-in:

```json
{
  "servers": {
    "slashobserve": {
      "type": "http",
      "url": "https://observe.example.com/mcp"
    }
  }
}
```

### Cursor

Add a `url`-only entry to `.cursor/mcp.json` and sign in when Cursor asks:

```json
{
  "mcpServers": {
    "slashobserve": {
      "url": "https://observe.example.com/mcp"
    }
  }
}
```

### Claude Desktop and claude.ai

These add the server from their connector settings and reach it from Anthropic's servers, so `SLASHOBSERVE_PUBLIC_URL` must be an `https` address that is reachable from the internet.
A server on `localhost` cannot be added.

## Keys for bots

A service-account key suits a headless bot that has no person to approve it.
Its writes show as the service account, and its role decides what it may do.

Export the key in the shell that starts your client, so the key never lands in a config file:

```sh
export OBSERVE_TOKEN=<your service-account key>
```

### Claude Code

Add this to `.mcp.json` in your project, with your server's address:

```json
{
  "mcpServers": {
    "slashobserve": {
      "type": "http",
      "url": "https://observe.example.com/mcp",
      "headers": { "Authorization": "Bearer ${OBSERVE_TOKEN}" }
    }
  }
}
```

Claude Code expands `${OBSERVE_TOKEN}` from its environment when it starts.
`.mcp.json` is the documented place for that expansion.

`claude mcp add` can create the same entry.
Use single quotes so your shell does not expand the variable and write the key into the config:

```sh
claude mcp add --transport http --scope project slashobserve https://observe.example.com/mcp --header 'Authorization: Bearer ${OBSERVE_TOKEN}'
```

Prefer `.mcp.json`.

The **API keys** page's Claude Code snippet also appends one line to your project's `CLAUDE.md` (only if the file does not already contain `For SlashObserve dashboards`) so Claude Code reads open comments with `list_comments`, fixes them with `update_dashboard`, then replies to each comment with `reply_to_comment` and resolves it with `resolve_comment`, passing the version that fixed it.

### OpenCode

Add this to `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "slashobserve": {
      "type": "remote",
      "url": "https://observe.example.com/mcp",
      "enabled": true,
      "oauth": false,
      "headers": { "Authorization": "Bearer {env:OBSERVE_TOKEN}" }
    }
  }
}
```

`"oauth": false` stops OpenCode from trying an OAuth flow; SlashObserve takes only the bearer key.

### Cursor

Cursor reads remote MCP servers from `.cursor/mcp.json` (per project) or `~/.cursor/mcp.json`.
The following is a generic example that this project has not tested with Cursor.
The `url` and `headers` fields apply to any client that supports remote MCP servers with custom headers.
How Cursor expands environment variables inside `headers` is defined by Cursor, so check its current MCP documentation before relying on the `${env:OBSERVE_TOKEN}` form below:

```json
{
  "mcpServers": {
    "slashobserve": {
      "url": "https://observe.example.com/mcp",
      "headers": { "Authorization": "Bearer ${env:OBSERVE_TOKEN}" }
    }
  }
}
```

If your Cursor version cannot expand variables there, put the key in the file only for a user-level `~/.cursor/mcp.json` that is never committed.

## The /observe prompt

The server offers one MCP prompt, `observe`, with an optional `ask`.
It returns the guide that outside agents follow to build and change dashboards, investigate a question, search logs and traces, answer comments, keep observations, review proposals and take assigned runs, and appends the `ask` as its last section.
The prompt also takes an optional `kind`, `build` or `investigate`, that returns the system text SlashObserve's own agent works from for that kind of run, which an agent that claims runs reads before it starts one.
Without `kind` it returns the general guide as before.
Any key that can call `/mcp` can list and get it, and the text holds nothing secret.
How a client shows a prompt is up to the client; in Claude Code it appears as an `mcp__` command under the server's name.
To get the same guide as `/observe` in Claude Code and other agents that read Agent Skills, run `observe skill install` (see [CLI](cli.md#agent-skill)).

## Take runs as an agent

An outside agent can do the runs that people hand to it, with its own model and under the guards of SlashObserve's own agent.
A run reaches it when a comment mentions it and starts a run: `@[Name](grant:<id>)` for a connected agent and `@[Name](service_account:<id>)` for a service account (see [Give an agent a run](agents.md#give-an-agent-a-run)).
The run waits in the queue, assigned to that one agent, and only that agent can claim it.
Only a mention assigns a run to an outside agent, and the built-in agent never takes one.

Two kinds of credential can claim:

- A connected agent, signed in over OAuth, claims the runs assigned to its own connection.
  It works only for the person who connected it, and the run's writes carry that connection's `direct` or `propose` mode.
- A service account claims the runs assigned to it with its key, after an Admin turns on **Takes agent runs** for it in the Create key panel or sets `takes_runs` through the API, or after `takesRuns: true` in the [service-accounts file](../operate/service-account-keys.md).
  Its runs always use the direct mode.
  With **Takes agent runs** off, or `takes_runs` false, `claim_run` answers `not_a_worker`.

The claim tools are `claim_run`, `report_run`, `finish_run` and `get_run_brief`; MCP lists them like any other tool.
Connect as in [Connect your own agent](#connect-your-own-agent) or [Keys for bots](#keys-for-bots), and poll with `claim_run` while the agent is idle.
An agent that polls shows as `listening` on the [Agents page](agents.md), and as `working` while it holds a run.

The loop of an agent that takes runs:

1. Call `claim_run`, which waits up to `wait_seconds` (0 to 30, 30 by default) and answers `run: null` when nothing is waiting.
   Call it again.
   An agent may keep 2 waiting `claim_run` calls at a time; past that, and when `wait_seconds` is 0, the answer is `run: null` with `retry_after_seconds` of 5, so wait that long before the next call.
2. A claimed run holds its `id`, `attempt`, `kind` (`build` or `investigate`), its `intent` (`ask`, `apply` or `suggest`), its `prompt`, its target (`dashboard_id`, `base_version`, `panel_id`, `from` and `to`, null when the run has none), `continue_note`, and `requested_by`, the person it acts for.
   The answer also has `lease_seconds` (60), a `session` with a `url`, a `token` (starting `sot_`) and `expires_at`, and `system_prompt`, the `observe` prompt to read with the run's `kind`.
   When a message in a comment thread started the run, `thread_context` holds the earlier messages of that thread, oldest first; read it before you answer, and it is `null` for any other run.
   When the question points at a spot on a panel or an observation block, `anchor` holds it, shaped as `start_run` takes it, and `block_id` the observation block it sits on; both are `null` for a run that points at nothing.
   `get_run_brief` names the same spot in one sentence, so an agent that reads only the claim can use `anchor`, and one that reads the brief gets the sentence.
   The server answers in the thread when the run ends, so do not reply there yourself.
3. Read the `observe` prompt with that `kind`; it is the system text SlashObserve's own agent works from for that kind of run.
   `get_run_brief` returns the same text with the first message SlashObserve's own agent starts each model session with, and is optional.
4. Open a second MCP connection to the session `url` with the token as the Bearer, and work with the tools it lists.
5. Call `report_run` at least every 30 seconds with the `run_id` and `attempt`; the lease ends 60 seconds after the last call, and `stop: true` in the answer means the run was canceled or has ended.
   `note`, one line of at most 500 characters, shows in the run's steps.
   When the server has no run time limit, the session's `expires_at` is a minute out and every `report_run` moves it a minute on, so a run lasts as long as it keeps reporting.
6. Call `finish_run` with `outcome` `succeeded` or `failed`.

A claim tool called with a run that is not yours at that attempt answers `not_your_run`.

The session lists the tools SlashObserve's own agent has for that kind of run, with the same text, and none of the claim tools.
It acts for the person who asked and records every call in the run's steps.
Its prompt list is empty, because the `observe` prompt is read on the first connection.

The token stops working when the attempt changes, when the run is canceled or ends, when the lease lapses, or when the run passes its time limit.
A report that arrives after the time limit ends the run as failed.
A run that saved a dashboard version, a [proposal](../concepts/proposals.md) or a finding ends as succeeded whatever `finish_run` says, and one that saved nothing ends as failed.
A run that no outside agent took within 10 minutes fails as `agent_unavailable`.

### The run session enforces the guards

The server enforces the guards of [SlashObserve's own agent](../concepts/agent.md#guards) on every run session, whatever agent works in it:

- The save-now window: after 6 turns without a save, 12 in an investigate or suggest run, only a save runs and a read in a turn without a save is refused; the call before the limit gets a note.
- Identical reads: a third identical read is refused, and an errored read or a query result runs again.
- One save per turn: a second save in one turn is refused.
- Repeated saves: a save identical to an earlier one is refused.
- Rejected saves in a row: the third ends the run as `validation_exhausted`.
- Stalled turns: two turns in a row in which every call was refused end the run as `no_progress`.
- Churn: a save that changes no panel, or changes a panel for the fifth time, gets a note, and the fourth such save in a row ends the run as `no_progress`.
- The cost limit: when `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` is set and `report_run` or `finish_run` reports `usage` whose cost reaches it, the answer has `stop: true` and the run ends as `budget_exhausted`; without `usage` the server cannot know the cost and never stops a run for it.
- Conflicts: a save that finds a newer version first rebases the build onto it when the panels do not overlap, and otherwise the run restarts once from the latest version; a second conflict ends the run as `conflict`.

Every call may carry `_meta["slashobserve.com/turn"]` with `{"id", "calls", "saves"}`: the id of one model turn, how many calls the turn made and how many of them save.
The server treats the calls with one id as one turn.
Without the key, each call is its own turn, so the stalled-turn and one-save-per-turn guards judge one call at a time.
A key the server cannot trust (an empty or over-long id, more than 64 calls, or more saves than calls) is ignored.
The key may also carry `usage`, the same cumulative object as `report_run`, counting the model call that made the turn, so a run the server ends on the result of that turn stores the usage including it.
The cost limit is checked only on `report_run` and `finish_run`.

Every result of a run session carries `_meta["slashobserve.com/run"]` with `{"settled", "build"}`.
`settled` is `saved`, `proposed`, `finding` or `suggested` once the run has its result (`proposed` when the save became a proposal, which ends the run as succeeded with the result kind `proposal`), `restarted` when a conflict restarted the run from the latest version, `ended` once the run has ended, and `null` otherwise.
`build` is the saved panel progress, `{"saves", "panels", "planned_panels"}`, or `null` when the run has no build.
Some results also carry a note after the content, which an agent reads like any other tool text.

### Briefs, usage and live text

`get_run_brief` takes `run_id`, `attempt` and an optional `reason` (`restart`, `output_limit`, `pending`, `conflict`, `continue`, `renewed`, `call_timeout`, `no_finding` or `no_answer`) and answers `system`, `message`, `reason` and `build`.
`message` is the first message of a model session.
Calling it starts a session: it resets the read guard and pins the parent version the run's saves are checked against, and the server may replace `reason` (with `conflict` after a rebase, with the resume reason when the run has a build).
An agent that never calls it works from the claim answer and the `observe` prompt.
It answers `run_ended` for a run that has ended.
A request on the run's token after the run ended is answered with HTTP 409 and `run_ended`, not a server error.

`report_run` also takes, all optional:

- `text`, at most 4,000 characters: the end of what the model is writing, shown on the run as live text while the run is running and never stored.
- `usage`: the agent's model use so far, cumulative, with `provider`, `model`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost_usd`, `model_calls`, `failed_calls`, `retries`, `compactions`, `renewals`, `model_ms` and `max_save_output_tokens`.

Its answer has, next to `state`, `stop` and `lease_until`, the run's `build` progress.

`finish_run` also takes `failure_code`, one of `call_timeout`, `rate_limited`, `provider_error`, `output_limit`, `no_tool_call`, `no_finding`, `refused`, `no_fixture` and `internal`, with `outcome` `failed`: the server stores its own text for the code, so no `message` is needed.
`commit: true`, with `outcome` `succeeded`, saves the finished build as a version first.
`usage` takes the fields above.
The answer has `restarted`, true when the commit found a newer version and the run restarted from it: the run stays claimed, so call `get_run_brief` and go on.
`finish_run` on a run that was canceled settles it as canceled at once.
A `failure_code` with another outcome, a code outside the list, or `commit` with `outcome` `failed` answers `invalid_request`.

The `usage` of `finish_run` is shown on the run as the agent reports it.

### Presence and modes

A connected agent or service account shows its presence, `working` while it holds a run, `listening` when it asked for work in the last 40 seconds and `offline` otherwise, with `last_seen_at`; see [Agent presence](../reference/states.md#agent-presence).
A connection's `mode` is `direct` by default.
Its person can switch it to `propose` with `PATCH /api/v1/me/grants/{id}` and the body `{"mode": "propose"}`, and back with `direct`, or with the **Mode** menu on the [Agents page](agents.md#direct-and-propose-mode); it applies from the agent's next request, and no key or agent can change it.
In propose mode `update_dashboard` and `update_panels` save a proposal and answer `proposed` even when the person is an Editor, `merge_proposal` answers `propose_mode`, and `discard_proposal` works only on a proposal the agent itself opened.
A save that removes panels or variables answers `proposal_required` and names them, so the agent asks its person to remove them; the same holds when a run ends with `finish` on a build that removed any, and when `finish_run` with `commit` saves such a build, which stays so the agent can put the panels back and finish again.
`create_dashboard` is not changed by the mode.
