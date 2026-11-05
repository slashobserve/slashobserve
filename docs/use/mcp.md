---
title: MCP
description: Connect Claude Code, OpenCode, Cursor and other MCP clients to the server.
---

The server exposes its tools to MCP clients over Streamable HTTP at `<public url>/mcp`, so Claude Code, OpenCode, Cursor and other clients can build and edit dashboards with their own model.
`GET /api/v1/instance` returns the exact address as `mcp_url` next to `public_url`; it needs a signed-in session or a key.

Requests must carry `Authorization: Bearer <key>` with a [service-account key](../operate/service-account-keys.md).
Browser sessions are not accepted on `/mcp`.
The endpoint is stateless, refuses an `Origin` that is not `SLASHOBSERVE_PUBLIC_URL`, answers `413` above 8 MiB, `429` above 8 concurrent requests per key and `503` above 32 across all keys.
On `initialize` the server returns an `Mcp-Session-Id` that only carries the client's self-reported name and holds no state.
Each version and agent request records its origin (`web`, `cli`, `mcp` or `api`) and, for `mcp`, that client name.

The server offers 21 tools.
Each has a lowest role, which the server checks when a client calls it, using the role of the key's service account:

| Role      | Tools                                                                                                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Viewer    | `list_dashboards`, `get_dashboard`, `get_panels`, `get_spec_guide`, `list_comments`, `list_folders`                                                           |
| Commenter | `reply_to_comment`, `resolve_comment`                                                                                                                         |
| Editor    | `create_dashboard`, `update_dashboard`, `update_panels`, `import_grafana_dashboard`, `list_datasources`, `list_metrics`, `list_labels`, `list_label_values`, `get_schema`, `sample_rows`, `lint_query`, `query_range`, `query_instant` |

A call below the key's role returns the tool error `forbidden_role`.
Folder access applies on top: a tool that names a dashboard or folder the key cannot see returns `not_found`, and changing a dashboard needs Edit on its folder.
A call with a field the tool does not take is refused with `invalid_request` and the list of fields it takes, and a field of the wrong JSON type is refused with the type it needs.
On `initialize` the server sends instructions with the order of work: read `get_spec_guide` once before the first save, find real names with the discovery tools, check each query with `lint_query` and `query_range` or `query_instant`, then save, sending changes with `update_panels`.
Each tool's description and input schema are the text the built-in agent reads, except that MCP clients get the full guide from `get_spec_guide`, `folder_id` on `create_dashboard`, and `dashboard_id` and `parent_version` on `update_dashboard` and `update_panels`.

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

`list_metrics` also takes `scope`: a series selector such as `{job="checkout"}` on Prometheus, or a table name on ClickHouse.
With `scope` it answers every metric family in that scope in one result: the name, type, unit, label keys and series count of each family, `families_total`, and `grouped`, the families that share a name prefix such as `http_server`.
Label keys that every family has are listed once in `common_labels`, and a scope too large for one result pages with `next_offset` like the other lists.
Runtime families, `up` and the names that start with `go_`, `process_`, `promhttp_` or `scrape_`, are left out of the list and counted in `runtime_families`, unless the scope names `__name__`, as `{__name__=~"go_.+"}` or `go_goroutines{job="checkout"}` do.
`search` and `limit` are refused together with `scope`.

The server keeps the last scoped inventory of each service account.
After one, the results of `create_dashboard`, `update_dashboard` and `update_panels` carry `uncovered`, up to 50 families of that inventory that no panel query names, and `uncovered_total`, how many there are in all; `uncovered` is `[]` once every family is covered.
Runtime families left out of the inventory are never in `uncovered`.
A histogram, summary or counter counts as covered when a query names any of its series, such as `<name>_bucket` or `<name>_total`.
Save results also carry `notes`, up to 20 hints that never reject a save: a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, and a stat with no `description`.

`list_folders` is available over MCP only.
It lists the folders the key can see, with `can_create` for each, and sets `truncated` when there are more than fit in the result.
`create_dashboard` takes an optional `folder_id` from that list; without it, the dashboard goes to General.
A `folder_id` the key cannot create in returns `forbidden_role`, and an unknown one `not_found`.
No tool deletes, moves or duplicates a dashboard: people do that in the web app or through the HTTP API, and delete also with `observe dashboards delete`.

MCP clients can read open comments with `list_comments`, reply with `reply_to_comment` and resolve with `resolve_comment`, passing the version that fixed the comment.
A dashboard made by Claude Code can then be reviewed by the team without a model on the server.

MCP clients can also hand the server a Grafana dashboard with `import_grafana_dashboard`: pass the dashboard JSON text, optionally a `folder` title, and a `mapping` from Grafana datasource keys to your datasource uids when the result says they could not be matched; panels whose queries fail are saved as unverified and listed in the result so the client can fix them with `update_dashboard`.
The result names the folder the dashboard went to in `folder`, and `folder_note` says why when it is not the one requested.
See [Import Grafana dashboards](import-grafana.md).

Export the key in the shell that starts your client, so the key never lands in a config file:

```sh
export OBSERVE_TOKEN=<your service-account key>
```

## Claude Code

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

## OpenCode

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

## Cursor

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
