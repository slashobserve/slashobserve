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

The server offers 19 tools.
Each has a lowest role, which the server checks when a client calls it, using the role of the key's service account:

| Role      | Tools                                                                                                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Viewer    | `list_dashboards`, `get_dashboard`, `get_spec_guide`, `list_comments`, `list_folders`                                                                         |
| Commenter | `reply_to_comment`, `resolve_comment`                                                                                                                         |
| Editor    | `create_dashboard`, `update_dashboard`, `import_grafana_dashboard`, `list_datasources`, `list_metrics`, `list_labels`, `list_label_values`, `get_schema`, `sample_rows`, `lint_query`, `query_range`, `query_instant` |

A call below the key's role returns the tool error `forbidden_role`.
Folder access applies on top: a tool that names a dashboard or folder the key cannot see returns `not_found`, and changing a dashboard needs Edit on its folder.

`list_folders` is available over MCP only.
It lists the folders the key can see, with `can_create` for each, and sets `truncated` when there are more than fit in the result.
`create_dashboard` takes an optional `folder_id` from that list; without it, the dashboard goes to the folder the key last created a dashboard in, else General.
A `folder_id` the key cannot create in returns `forbidden_role`, and an unknown one `not_found`.

MCP clients can read open comments with `list_comments`, reply with `reply_to_comment` and resolve with `resolve_comment`, passing the version that fixed the comment.
A dashboard made by Claude Code can then be reviewed by the team without a model on the server.

MCP clients can also hand the server a Grafana dashboard with `import_grafana_dashboard`: pass the dashboard JSON text, optionally a `folder` title, and a `mapping` from Grafana datasource keys to your datasource uids when the result says they could not be matched; panels whose queries fail are saved as unverified and listed in the result so the client can fix them with `update_dashboard`.
The result names the folder the dashboard went to in `folder`, and `folder_note` says why when it is not the one requested.
See [Import Grafana dashboards](import-grafana.md).

Export the key in the shell that starts your client, so the key never lands in a config file:

    export OBSERVE_TOKEN=<your service-account key>

## Claude Code

Add this to `.mcp.json` in your project, with your server's address:

    {
      "mcpServers": {
        "slashobserve": {
          "type": "http",
          "url": "https://observe.example.com/mcp",
          "headers": { "Authorization": "Bearer ${OBSERVE_TOKEN}" }
        }
      }
    }

Claude Code expands `${OBSERVE_TOKEN}` from its environment when it starts.
`.mcp.json` is the documented place for that expansion.

`claude mcp add` can create the same entry.
Use single quotes so your shell does not expand the variable and write the key into the config:

    claude mcp add --transport http --scope project slashobserve https://observe.example.com/mcp --header 'Authorization: Bearer ${OBSERVE_TOKEN}'

Prefer `.mcp.json`.

The **API keys** page's Claude Code snippet also appends one line to your project's `CLAUDE.md` (only if the file does not already contain `For SlashObserve dashboards`) so Claude Code reads open comments with `list_comments`, fixes them with `update_dashboard`, then replies to each comment with `reply_to_comment` and resolves it with `resolve_comment`, passing the version that fixed it.

## OpenCode

Add this to `opencode.json`:

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

`"oauth": false` stops OpenCode from trying an OAuth flow; SlashObserve takes only the bearer key.

## Cursor

Cursor reads remote MCP servers from `.cursor/mcp.json` (per project) or `~/.cursor/mcp.json`.
The following is a generic example that this project has not tested with Cursor.
The `url` and `headers` fields apply to any client that supports remote MCP servers with custom headers.
How Cursor expands environment variables inside `headers` is defined by Cursor, so check its current MCP documentation before relying on the `${env:OBSERVE_TOKEN}` form below:

    {
      "mcpServers": {
        "slashobserve": {
          "url": "https://observe.example.com/mcp",
          "headers": { "Authorization": "Bearer ${env:OBSERVE_TOKEN}" }
        }
      }
    }

If your Cursor version cannot expand variables there, put the key in the file only for a user-level `~/.cursor/mcp.json` that is never committed.
