---
title: Audit queries
description: The fields of an audit event, the three query forms the audit trail accepts, the window and the limits, and how to read the trail over the API, MCP and the CLI.
---

The audit trail records every sign-in, read, change, agent tool call and refusal, with who did it, through what, to what and how it ended.
Only Admins read it, and an Admin service-account key can too.
Every read of the trail is itself recorded, as `audit.search` or `audit.export`, with the query form, the window and the row count, never the query text.

## Fields

Every event has these fields; `GET /api/v1/audit/fields`, `list_audit_fields` and `observe audit fields` list them with their descriptions.
A field with an alias takes either name in a query.

| Field | Alias | Type | What it holds |
| ----- | ----- | ---- | ------------- |
| `occurred_at` | | date | When the event happened |
| `id` | | uuid | Event id |
| `category` | | enum | `auth`, `access`, `content`, `conversation`, `agent`, `query`, `admin` or `system` |
| `action` | | text | What was attempted, such as `dashboard.update`; `dashboard.*` matches a prefix |
| `outcome` | | enum | `success`, `denied` or `failed`; an event whose commit the server could not confirm reads `unknown`, which a query cannot name |
| `reason` | | text | The API error code of a refusal or failure, such as `forbidden_role` |
| `status` | | int | HTTP status of the request |
| `duration_ms` | `duration` | int | How long the action took, in milliseconds |
| `actor_kind` | | enum | Whose permissions were used: `user`, `service_account`, `anonymous` or `system` |
| `actor_id` | | uuid | The user or service account |
| `actor_name` | `actor` | text | The actor's name when the event happened |
| `actor_role` | | text | The actor's role when the event happened |
| `initiator` | | enum | Who started it: `person`, `agent` or `system` |
| `agent_id`, `agent_name` | | uuid, text | The agent that acted |
| `run_id` | `run` | uuid | The agent run that made the event |
| `auth_method` | | enum | `session`, `api_key`, `oauth`, `public_link` or `none` |
| `credential_id` | | text | The session, key, grant or public link used, never the secret |
| `origin` | | enum | `web`, `cli`, `mcp`, `api`, `agent`, `job` or `provision` |
| `origin_client` | | text | The connected client's name |
| `tool` | | text | The MCP or agent tool |
| `client_ip` | `ip` | ip | The caller's address; a query takes an address or a CIDR such as `10.4.0.0/16` |
| `user_agent` | | text | The request's user agent |
| `request_id` | `request` | uuid | The request id, also the `X-Request-Id` response header |
| `trace_id` | | text | The request's trace id, when one was sent |
| `http_method`, `route` | | text | The request's method and route template |
| `target` | | target | What the event was about as `kind/id`, such as `dashboard/8f3c...`; `dashboard` alone and `dashboard/8f*` work too |
| `target_kind`, `target_id`, `target_name` | | text | The parts of the target, and its name when the event happened |
| `folder_id`, `dashboard_id`, `observation_id`, `thread_id`, `chat_id` | | uuid | The objects the event touched |
| `datasource_uid` | `datasource` | text | The datasource a query ran on |
| `server_id`, `server_version` | | text | The server replica and build that recorded the event |

Events also carry `seq` and `data`, the action's own fields as the audit catalog declares them, which a query cannot name.
Free text, a word with no field, matches the action, the actor's and the target's names, the reason, the tool, the route and a query's text.

## Query forms

A query is KQL unless it starts with `lucene:` or `{`, and an empty query matches every event in the window.

KQL, the default, joins `field:value` terms with `and`, `or` and `not`, and groups them with parentheses:

```text
outcome:denied and origin:mcp
action:dashboard.* and not actor_name:"Sam Lee"
status >= 500 or reason:query_timeout
target:dashboard/8f3c2a1e-0d6f-4b7e-9d39-5a7f0f0c4a11
client_ip:10.4.0.0/16 and initiator:agent
```

Lucene, after a `lucene:` prefix, uses `AND`, `OR`, `NOT`, `+` and `-`, ranges in brackets and wildcards at the end of a word; fuzzy and proximity searches are refused:

```text
lucene: outcome:denied AND origin:(mcp OR cli)
lucene: status:[500 TO 599] -action:auth.*
lucene: occurred_at:[2026-11-01 TO 2026-11-08}
```

The Query DSL is a JSON body that starts with `{`, with an optional `query`, `size` (the page size of a search) and `aggs`.
`query` takes `bool` (`must`, `filter`, `should`, `must_not`), `term`, `terms`, `match`, `match_phrase`, `range`, `wildcard`, `exists`, `multi_match`, `query_string`, `match_all` and `match_none`:

```json
{"query": {"bool": {"filter": [{"term": {"outcome": "denied"}}, {"range": {"status": {"gte": 400}}}]}}, "size": 50}
```

`aggs` runs only through the aggregate endpoint: `date_histogram` and `terms` buckets, up to three levels with one bucket aggregation per level, and `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles` metrics.
An aggregation whose buckets would make more than 100,000 rows is refused with the reason.

```json
{"aggs": {"by_origin": {"terms": {"field": "origin"}, "aggs": {"people": {"cardinality": {"field": "actor_id"}}}}}}
```

A query that cannot be read answers 422 `query_invalid` with the parser's message and `start` and `end`, the byte offsets in the query to underline; over MCP the message names the same offsets.

## Window and limits

Every read covers a window from `since`, inclusive, to `until`, exclusive.
Each takes an RFC 3339 time, epoch milliseconds or a relative time such as `now-7d`; `until` is now by default and `since` 24 hours before `until`.
A window longer than 31 days is refused with 400 `invalid_request`; read a longer period one window at a time.
Each read runs in a read-only transaction with a 30 second statement timeout and answers 422 `query_timeout` past it.

A search returns 100 events a page by default and at most 1,000, newest first by `occurred_at` and then `id`.
`next_cursor` pages through one search without repeating or skipping an event, even while new events are recorded, because it keeps the window of the first page; a cursor sent with another query is refused.
An export streams up to 1,000,000 events, the value of its `X-Export-Row-Limit` header, and stops there.

## API, MCP and the CLI

| Read | API | MCP | CLI |
| ---- | --- | --- | --- |
| Search events | `GET /api/v1/audit/events` | `search_audit_events` | `observe audit search` |
| One event with its request and run | `GET /api/v1/audit/events/{id}` | `get_audit_event` | `observe audit show <id>` |
| Top values of a field | `GET /api/v1/audit/facets` | `list_audit_facets` | `observe audit facets <field>` |
| Counts over time | `GET /api/v1/audit/histogram` | `get_audit_histogram` | `observe audit histogram` |
| Aggregation | `GET /api/v1/audit/aggregate` | `aggregate_audit_events` | `observe audit aggregate` |
| Fields | `GET /api/v1/audit/fields` | `list_audit_fields` | `observe audit fields` |
| Export as NDJSON or CSV | `GET /api/v1/audit/export` | | `observe audit export` |

Every route answers 403 `forbidden_role` to a caller below Admin.
An export has no MCP tool, because an MCP answer holds at most 16 KiB; an agent pages the same events with `search_audit_events`, 20 a call by default and at most 100.
No route or tool changes or deletes an audit event.
