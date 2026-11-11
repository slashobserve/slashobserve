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
For a key, `initialize` returns an `Mcp-Session-Id` that only carries the client's self-reported name and holds no state.
For an OAuth client, the name it registered under is used and `clientInfo` is ignored.
Each version, agent request, finding and annotation records its origin (`web`, `cli`, `mcp` or `api`) and, for `mcp`, that client name, and the web app, the CLI and the API show the author as `<name> (via <client>)`.

Each tool has a lowest role, which the server checks when a client calls it, using the role of the key's service account, or of the person for an OAuth client:

| Tool | Lowest role |
| ---- | ----------- |
| `add_block` | Viewer |
| `claim_run` | Viewer |
| `compare_traces` | Viewer |
| `create_annotation` | Viewer |
| `create_comment` | Viewer |
| `create_dashboard` | Viewer |
| `create_finding` | Editor |
| `create_observation` | Viewer |
| `discard_block` | Viewer |
| `draft_query` | Viewer |
| `edit_comment` | Viewer |
| `finish_run` | Viewer |
| `get_dashboard` | Viewer |
| `get_observation` | Viewer |
| `get_panels` | Viewer |
| `get_run` | Viewer |
| `get_schema` | Viewer |
| `get_spec_guide` | Viewer |
| `get_trace` | Viewer |
| `import_grafana_dashboard` | Viewer |
| `keep_block` | Viewer |
| `lint_query` | Viewer |
| `list_activity` | Viewer |
| `list_annotations` | Viewer |
| `list_comments` | Viewer |
| `list_dashboard_health` | Viewer |
| `list_dashboards` | Viewer |
| `list_datasources` | Viewer |
| `list_findings` | Viewer |
| `list_folders` | Viewer |
| `list_label_values` | Viewer |
| `list_labels` | Viewer |
| `list_metrics` | Viewer |
| `list_observations` | Viewer |
| `list_saved_searches` | Viewer |
| `log_context` | Viewer |
| `log_facets` | Viewer |
| `log_patterns` | Viewer |
| `logs_for_trace` | Viewer |
| `move_finding` | Viewer |
| `publish_rca` | Viewer |
| `query_instant` | Viewer |
| `query_range` | Viewer |
| `reopen_comment` | Viewer |
| `reply_to_comment` | Viewer |
| `report_run` | Viewer |
| `resolve_comment` | Viewer |
| `resolve_observation` | Viewer |
| `sample_rows` | Viewer |
| `save_search` | Viewer |
| `search_workspace` | Viewer |
| `trace_stats` | Viewer |
| `update_block` | Viewer |
| `update_dashboard` | Viewer |
| `update_panels` | Viewer |

The lowest role is the one the web app needs for the same work, and folder access decides the rest, as in the web app.
A person's client can create and change dashboards in their own My Dashboards, comment where they may comment, and query every datasource, whatever their role.
A key has no My Dashboards and is never on a folder's list, so below Editor it can read and query but not save.
`create_finding` needs Editor, like Investigate.

A call below a tool's role, or a change the folder does not allow, returns the tool error `forbidden_role`; a dashboard or folder the caller cannot see returns `not_found`.
A call with a field the tool does not take is refused with `invalid_request` and the list of fields it takes, and a field of the wrong JSON type is refused with the type it needs.
On `initialize` the server sends instructions with the order of work: read `get_spec_guide` once before the first save, find real names with the discovery tools, check each query with `lint_query` and `query_range` or `query_instant`, then save, sending changes with `update_panels`.
Each tool's description and input schema are the text the built-in agent reads, except that MCP clients get the full guide from `get_spec_guide`, `folder_id` on `create_dashboard`, and `dashboard_id` and `parent_version` on `update_dashboard` and `update_panels`.
`get_spec_guide` returns the core of the authoring guide plus one section per datasource type the server has configured, the text the built-in agent reads.

`create_dashboard` and `update_dashboard` run every panel query before they save and reject a panel whose query fails or returns no data, as described in [What a save checks](../concepts/agent.md#what-a-save-checks).
`update_dashboard` needs the `dashboard_id` and the `parent_version` you read, and sends the complete document; if a newer version has been saved since, it returns the tool error `version_conflict` with the latest version.
`get_dashboard` returns the Spec at the latest version or at `version`, and returns `too_large` for a Spec over 128 KiB.

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
`logs_for_trace` reads the log lines that carry a trace id, or one span id, from a Loki, ClickHouse, Elasticsearch or OpenSearch datasource over a window, the same lines the trace view opens from a span, and answers the table summary of `query_instant`.

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
  `list_metrics` answers an empty list there, with or without `scope`.

On Loki each family of a scoped `list_metrics` is a stream label key, and its `series` counts the streams that carry it.
That inventory, and the empty one of Elasticsearch and OpenSearch, never replaces the one `uncovered` reads.

The server keeps the last scoped inventory of each service account.
After one, the results of `create_dashboard`, `update_dashboard` and `update_panels` carry `uncovered`, up to 50 families of that inventory that no panel query names, and `uncovered_total`, how many there are in all; `uncovered` is `[]` once every family is covered.
Runtime families left out of the inventory are never in `uncovered`.
A histogram, summary or counter counts as covered when a query names any of its series, such as `<name>_bucket` or `<name>_total`.
Save results also carry `notes`, up to 20 hints that never reject a save: a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, and a stat with no `description`.

`list_folders` is available over MCP only.
It lists the folders the key can see, with `can_create` for each, and sets `truncated` when there are more than fit in the result.
`create_dashboard` takes an optional `folder_id` from that list; without it, a person's dashboard goes to their My Dashboards and a key's to General.
A `folder_id` the key cannot create in returns `forbidden_role`, and an unknown one `not_found`.
No tool deletes, moves or duplicates a dashboard: people do that in the web app or through the HTTP API, and delete also with `observe dashboards delete`.
`move_finding` is the one move tool, and only for a finding without a dashboard.

MCP clients can read open comments with `list_comments`, reply with `reply_to_comment` and resolve with `resolve_comment`, passing the version that fixed the comment.
A dashboard made by Claude Code can then be reviewed by the team without a model on the server.

Comments can sit on a dashboard, a finding or an annotation.
`list_comments` takes exactly one of `target` and `dashboard_id`: `target` is written `kind:uuid`, for example `finding:7f0c1d2e-5a4b-4c3d-8e9f-0a1b2c3d4e5f`, and `dashboard_id` means the same as `dashboard:<id>`.
Each comment has its `version`, which is null off a dashboard, `edited_at` and its `mentions`; a result listed by `target` also names that `target`.
`create_comment` opens a thread on a `target`; on a dashboard it takes an optional `panel_id` and `version`, and defaults to the whole dashboard on the latest version.
`edit_comment` changes the text of a comment the same key wrote, `reopen_comment` reopens a resolved thread, and `reply_to_comment` and `resolve_comment` work on any thread, `resolve_comment` taking `version` only on a dashboard.
A comment id is the id of any message of the thread, and the first message's id is the thread's.

Write `@observe` to address the built-in agent: a mention is recorded and appears in the activity log as `thread.mentioned`, and it does not start a run.
Every opened, replied, edited, resolved and reopened thread appends to the activity log, which `list_activity` reads.
A key needs the Comment ability on the target to write; a target the key cannot see answers `not_found`, and only the author can edit a message (`not_author`).

`create_finding` saves a finding outside an agent run: a Markdown answer of at most 400 words with up to four evidence panels whose queries must return data.
A finding without a `dashboard_id` is filed in `folder_id`, by default the key's My Dashboards when it has one, else General; `dashboard_id` and `folder_id` cannot go together.
`list_findings` lists findings newest first, those on one dashboard with `dashboard_id`, those filed in one folder with `folder_id`, else the key's own.
`move_finding` moves a finding without a dashboard into another folder, which shares it with that folder's readers.
It takes `finding_id` and `folder_id` and returns the finding with the title of its folder; it needs Edit on the finding and on the folder, and a finding or folder the key cannot see answers `not_found`.
A finding on a dashboard answers `invalid_request`, because it follows the dashboard, and so does a move into My Dashboards.

`save_search` saves a logs or trace search with a title, outside an agent run, so people can open it from the logs or traces page.
It takes `kind` (`logs` or `traces`), `title` and `datasource_uid`, and optionally `query`, `filters`, `from`, `to` and `folder_id`, and returns the saved search with its `url`.
A logs search needs a `query` and a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, and its `filters` takes only `columns`; a traces search needs a Tempo or ClickHouse datasource, takes an optional TraceQL `query` on Tempo and none on ClickHouse, and its `filters` takes `service`, `span`, `min`, `max` and `errors`.
`from` and `to` default to `now-1h` and `now`, and are saved as written.
A search is filed in `folder_id`, by default the key's My Dashboards when it has one, else General, and the folder's readers can open it.
A `folder_id` the key cannot save in returns `forbidden_role`, and an unknown one `not_found`; a datasource that does not exist or does not fit the kind returns `invalid_request`.
Check the query with `lint_query` or `draft_query` first, because saving runs no query.

`list_saved_searches` lists the saved searches in the folders the key can see, most recently changed first, optionally of one `kind` or in one `folder_id`, with each one's datasource, query, filters, range, folder and `url`.
No tool renames, moves or deletes a saved search: people do that on the web or with `observe saved-searches`.

An observation is a page of blocks about one question: Markdown text, panels that run live, and snapshots that freeze a panel's data.
`create_observation` takes a `title`, and optionally a `folder_id` (by default the key's My Dashboards when it has one, else General), a range with `from` and `to`, and up to 10 `blocks`; the blocks are added in order after the observation exists, and the first refused one stops the rest and answers `block_refused` with its `block_index` and the `observation_id` that was created.
`get_observation` returns the observation with `allowed_actions` and its blocks in order, each with its `kind`, `state`, `position`, `width`, `version` and a short view: Markdown clipped to 2,000 characters (`markdown_truncated`), a panel's `title`, `type` and datasources, a snapshot's `snapshot_id`; `blocks_truncated` says the rest did not fit.
`list_observations` lists the observations in the folders the key can see, most recently changed first, optionally in one `folder_id` or one `state` (`open` or `resolved`), and pages with `next_cursor`.
`add_block` takes `observation_id` and a `kind`: `text` takes `markdown` of at most 4,000 characters, `panel` takes `panel` and optionally its own `from` and `to` (otherwise the range of the observation, else the last hour), and `snapshot` takes `from_block_id`, the panel block to freeze; `width` (`full` or `half`) and `position` apply to every kind, and a field of another kind is refused with `invalid_request`.
`update_block` takes `block_id`, the `version` you read and the new `markdown`, or the new `panel` with its range, and optionally `width` or `position`.
`resolve_observation` closes an observation with a one-line `summary`.
Every write answers `code: ok` with the ids, the `state` of what it changed, the block `version` and the `url`.

`publish_rca` takes `observation_id` and publishes it as the next immutable RCA revision (see [Observations](observations.md#publish-as-rca)), answering `code: ok` with the `revision`, the number of `blocks` and `snapshots` it holds and the `observation_url`; a failing panel answers `panel_failed` with the block it names, and the other refusals are `rca_limit`, `observation_changed`, `snapshot_too_large` and `personal_folder`.
It never creates a public link: a person does that, and no tool lists or revokes one either.

A block written by someone who can only comment arrives as `requested` and says so in `note`; a person who can edit keeps it with `keep_block` or discards it with `discard_block`, and the one who asked for it may discard it too.
A block's `version` is checked by `update_block` and `keep_block`: a stale one answers `version_conflict`, so read the block with `get_observation` and apply the change to the version it names.
A transition the state does not allow, such as keeping a block that is already live, answers `invalid_transition` with the actions allowed now.
Like every agent write, a text block with HTML is refused with `html_not_allowed`, and a panel is checked as `create_finding` checks its evidence: a panel whose query fails or returns no data in its range is refused with its issues.
No tool removes a block, or moves, renames or reopens an observation: people do that in the web app or through the HTTP API.

No tool queues an investigation on the server: an MCP client investigates with its own model, using the discovery and query tools, and saves the answer with `create_finding`.

`list_dashboard_health` is available over MCP only.
It lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see, most recently updated first, 20 per page by default and at most 50: each with its `problems` (`kind`, and `panel_id`, `panel_title`, `variable`, `code` and `message` where they apply), `checked_at` and `checked_version`, plus the state of the daily check in `job`.
`folder_id` and `kind` narrow the list, and `next_cursor` pages on.
A client repairs a failing panel itself: it reads the panel with `get_panels` and saves the fix with `update_panels`.

`draft_query` is available over MCP only.
It turns a question into a checked LogQL, ClickHouse SQL, KQL or TraceQL query, or trace search filters, for one Loki, ClickHouse, Elasticsearch, OpenSearch or Tempo datasource, as [Ask](logs-and-traces.md#ask-in-plain-language) does.
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
Narrow it with `dashboard_id`, `folder_id`, `types` and `since` (RFC 3339), 50 per page by default and at most 100, and page with `next_cursor`.
It lists only the events of folders the key may see, and it is over MCP only, so the built-in agent does not have it.

`get_run` reads one run of the built-in agent by the id of a `run.*` event from `list_activity`.
The answer is the object `GET /api/v1/observe/{id}` returns: its `state`, `result_kind`, `allowed_actions`, `attempt`, the last 20 steps and its usage.
It is over MCP only, and a run is visible to the same people as through the API.

`search_workspace` runs the [search](search.md) of the workspace: pass `q`, and optionally `kinds` (`dashboard`, `finding`, `annotation`, `observation`, `comment`, `saved_search`), `limit` (20 by default, at most 50) and `cursor`.
Each result has its `kind`, `id`, `title`, a plain-text `snippet`, the `url`, the `folder`, `updated_at` and, for a comment, the `target` it is on and its `thread_id`.
It finds only what the key may read, and it is over MCP only, so the built-in agent does not have it.

`list_annotations` lists [annotations](annotations.md), such as deploy markers, newest first: the global ones, plus one dashboard's own with `dashboard_id`, narrowed by `from` and `to` (RFC 3339) and `tags`, 50 per page by default and at most 100.
It is the one annotation tool the built-in agent also has, and only in investigate runs, so an investigation can see a deploy beside a spike.
`create_annotation` is available over MCP only and adds one, with the fields of `POST /api/v1/annotations`: `text`, `tags`, `time_from` and `time_to`, and `dashboard_id` and `panel_id` to place it on one dashboard or panel.
Without `dashboard_id` the annotation shows on every dashboard.
It needs Edit on the dashboard's folder, and an annotation with no `dashboard_id` needs the Editor role.
No tool deletes an annotation; people do that in the web app or through the HTTP API.

MCP clients can also hand the server a Grafana dashboard with `import_grafana_dashboard`: pass the dashboard JSON text, optionally a `folder` title, and a `mapping` from Grafana datasource keys to your datasource uids when the result says they could not be matched; panels whose queries fail are saved as unverified and listed in the result so the client can fix them with `update_dashboard`.
The result names the folder the dashboard went to in `folder`, and `folder_note` says why when it is not the one requested.
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

Disconnect a client in Settings, Connected agents; an Admin sees and disconnects everyone's there too.

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
It returns the guide that outside agents follow to build and change dashboards, investigate a question, search logs and traces and answer comments, and appends the `ask` as its last section.
The prompt also takes an optional `kind`, `build` or `investigate`, that returns the system text SlashObserve's own agent works from for that kind of run, which an agent that claims runs reads before it starts one.
Without `kind` it returns the general guide as before.
Any key that can call `/mcp` can list and get it, and the text holds nothing secret.
How a client shows a prompt is up to the client; in Claude Code it appears as an `mcp__` command under the server's name.
To get the same guide as `/observe` in Claude Code and other agents that read Agent Skills, run `observe skill install` (see [CLI](cli.md#agent-skill)).

## Take runs as an agent

A connected agent, signed in over OAuth, can claim the runs assigned to it with `claim_run`.
A service account can claim runs only after an Admin turns on `takes_runs`, or after `takesRuns: true` in the [service-accounts file](../operate/service-account-keys.md).
Nothing assigns runs to outside agents yet, so SlashObserve's own agent runs every run until assignment arrives.

The loop of an agent that takes runs:

1. Call `claim_run`, which waits up to `wait_seconds` (30 by default) and answers `run: null` when nothing is waiting.
   Call it again.
2. A claimed run holds its `id`, `attempt`, `kind`, `prompt` and target, a `session` with a `url` and a `token`, and the `observe` prompt to read with the run's `kind`.
3. Read the `observe` prompt with that `kind`; it is the system text SlashObserve's own agent works from for that kind of run.
4. Open a second MCP connection to the session `url` with the token as the Bearer, and work with the tools it lists.
5. Call `report_run` at least every 30 seconds with the `run_id` and `attempt`; the lease ends 60 seconds after the last call, and `stop: true` in the answer means the run was canceled or has ended.
6. Call `finish_run` with `outcome` `succeeded` or `failed`.

The session lists the tools SlashObserve's own agent has for that kind of run, with the same text, and none of the claim tools.
It acts for the person who asked and records every call in the run's steps.
Its prompt list is empty, because the `observe` prompt is read on the first connection.

The token stops working when the attempt changes, when the run is canceled or ends, when the lease lapses, or when the run passes its time limit.
A report that arrives after the time limit ends the run as failed.
A run that saved a dashboard version or a finding ends as succeeded whatever `finish_run` says, and one that saved nothing ends as failed.
A run that no agent held for 10 minutes fails as `agent_unavailable`.

The `usage` of `finish_run` is shown on the run as the agent reports it, and a reported cost does not stop a run.
A connected agent or service account shows its presence, `working` while it holds a run, `listening` when it asked for work in the last 40 seconds and `offline` otherwise, with `last_seen_at`; see [Agent presence](../reference/states.md#agent-presence).
A connection's `mode` is `direct` by default.
Its person can switch it to `propose` with `PATCH /api/v1/me/grants/{id}` and the body `{"mode": "propose"}`, and back with `direct`; it applies from the agent's next request, and no key or agent can change it.
In propose mode the agent's dashboard saves become proposals and it cannot merge them.
