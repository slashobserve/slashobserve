---
title: Audit queries
description: The fields of an audit event, the three query forms the audit trail accepts, the window and the limits, and how to read the trail over the API, MCP and the CLI.
---

The audit trail records every sign-in, read, change, agent tool call and refusal, with who did it, through what, to what and how it ended.
Only Admins read it, and an Admin service-account key can too.
A search, an export and opening the stream are themselves recorded, as `audit.search` or `audit.export`, with the query as sent (cut at 4 KiB and marked `[truncated]`), its form, the window and the row count, so the trail shows who searched it for what.
A search is recorded once per caller and query text in 10 minutes: the repeats, the pages after the first and a page that refreshes itself add no event, and the first one after the 10 minutes is recorded again.
The counts, the histogram, the aggregations, the field list and opening one event record nothing, because they only draw what a recorded search already named; an export and each opening of the stream are recorded every time.
A query that is refused is recorded every time, as a `failed` event with its reason.

A view of a chat or an agent request, as `chat.view` or `run.view`, is recorded once per reader and subject in 10 minutes, the owner's own views included.
A list of chats or requests is one `chat.list` or `run.list` event, recorded once per reader and scope in 10 minutes, never one per row.
The Agents page's list of one agent's runs is a `run.list` with scope `workspace` for an Admin or `you`, and the connected agent or service account as its target.
A refused view is recorded every time.

## Fields

Every event has these fields; `GET /api/v1/audit/fields`, `list_audit_fields` and `observe audit fields` list them with their descriptions.
A field with an alias takes either name in a query, in KQL, Lucene and a Query DSL body alike, and the query lint of the audit datasource accepts either; the `fields` list shows every alias, and a query that names an unknown field says which valid ones are nearest to it.

| Field | Alias | Type | What it holds |
| ----- | ----- | ---- | ------------- |
| `occurred_at` | | date | When the event happened |
| `id` | | uuid | Event id |
| `category` | | enum | `auth`, `access`, `content`, `conversation`, `agent`, `query`, `admin` or `system` |
| `action` | | text | What was attempted, such as `dashboard.update`; `dashboard.*` matches a prefix |
| `outcome` | | enum | `success`, `denied`, `failed` or `unknown`, for a redirect or an event whose commit the server could not confirm; a `run.finish` is `success` when the run saved its result and `failed` when it failed, timed out or was canceled, and its `result` says which of `succeeded`, `failed` or `canceled` |
| `reason` | | text | The API error code of a refusal or failure, such as `forbidden_role` |
| `status` | | int | HTTP status of the request |
| `duration_ms` | `duration` | int | How long the action took, in milliseconds |
| `actor_kind` | `actor.kind` | enum | Whose permissions were used: `user`, `service_account`, `anonymous` or `system` |
| `actor_id` | `actor.id` | uuid | The user or service account |
| `actor_name` | `actor`, `actor.name` | text | The actor's name when the event happened; `actor` alone also matches the actor's current email address |
| `actor_email` | `actor.email` | text | The current email address of the user who acted, read from the user and not stored in the event; a service account has none; a query can match it but not count it |
| `actor_role` | `actor.role` | text | The actor's role when the event happened |
| `initiator` | | enum | Who started it: `person`, `agent` or `system` |
| `agent_id`, `agent_name` | | uuid, text | The agent that took the run, on every event of its run session: its tool calls, the changes they make, and `run.start`, `run.finish` and `run.release`. `Observe Agent` with no id, a connected agent's grant id and client name, or a service account's id and name. Empty on what a person does to a run, such as `run.queue`, `run.cancel` and `run.keep`, on the `run.finish` of a run that no agent took (canceled while queued, or expired unclaimed), and outside a run |
| `run_id` | `run` | uuid | The agent run that made the event |
| `auth_method` | | enum | `session`, `api_key`, `oauth`, `public_link` or `none` |
| `credential_id` | | text | The session, key, grant or public link used, never the secret |
| `origin` | | enum | `web`, `cli`, `mcp`, `api`, `agent` (what the built-in agent does with its run key), `job`, `provision` or `unknown` (history copied from activity that recorded no origin) |
| `origin_client` | | text | The connected client's name |
| `tool` | | text | The MCP or agent tool |
| `client_ip` | `ip` | ip | The caller's address; a query takes an address or a CIDR such as `10.4.0.0/16`. Behind a proxy it is the proxy's address unless `SLASHOBSERVE_TRUSTED_PROXIES` lists the proxy; see [Put TLS in front](../operate/production.md#put-tls-in-front) |
| `user_agent` | | text | The request's user agent |
| `request_id` | `request` | uuid | The request id, also the `X-Request-Id` response header |
| `trace_id` | | text | The request's trace id, when one was sent |
| `http_method`, `route` | | text | The request's method and route template |
| `target` | | target | What the event was about as `kind/id`, such as `dashboard/8f3c...`; `dashboard` alone and `dashboard/8f*` work too |
| `target_kind`, `target_id`, `target_name` | `target.kind`, `target.id`, `target.name` | text | The parts of the target, and its name when the event happened |
| `folder_id`, `dashboard_id`, `observation_id`, `thread_id`, `chat_id` | | uuid | The objects the event touched |
| `datasource_uid` | `datasource` | text | The datasource a query ran on |
| `server_id`, `server_version` | | text | The server replica and build that recorded the event |

Events also carry `seq` and `data`, the action's own fields as the audit catalog declares them, which a query cannot name.
A `run.finish` recorded by a server older than 0.1.4 says `success` whatever the run's result, so read `data.result` on those events, which a query cannot filter on; every later one agrees with its run.
A `query.run` whose `data` holds `stream: true` is a live tail of a logs query: it is written when the stream ends, and its `duration_ms` is how long the stream stayed open, up to the 30 minute limit, not how long a query took.
Repeated refusals are folded so an attack cannot flood the trail: `auth.credential_rejected`, `auth.csrf_rejected` and `auth.login_failed` events that share an action, reason, target, actor and provider error are one key.
The first event of a key is written at once with `data.count` of 1.
The rest of that key's minute become one later event whose `data.count` is how many more there were, and there is no later event when there were none, so the `count` of all the events of a key adds up to the refusals.
When more than 1000 keys are open, refusals of new keys fold under one anonymous event marked `data.overflow` that names no actor or target, and those follow the same rule.
Free text, a word with no field, matches the action, the actor's and the target's names, the reason, the tool, the route and a query's text.

## Event data

An event's `data` holds fields that belong to its action and that a query cannot name; read them from a result.
Only the fields an action declares are kept, and no event holds a message body, a prompt or an answer.

| Action | Data | What it holds |
| ------ | ---- | ------------- |
| `dashboard.view` | `version` | The version number the reader was shown |
| `observation.view` | `updated_at` | When the observation last changed before the reader opened it, as an RFC 3339 time, since an observation has no versions |
| `observation.arrange` | `title`, `changed` | An arrangement of an observation's blocks that changed something, with `changed` listing which of `height`, `order` and `width` did; it is not an `observation.update`; over the API a request that changes nothing is still recorded, with reason `no_change` and without `changed` |
| `saved_prompt.create`, `saved_prompt.update`, `saved_prompt.share`, `saved_prompt.unshare`, `saved_prompt.delete` | `owner_id`, `changed`, `from_name` | A change to a prompt that is shared, or that becomes shared or private, with the prompt's owner and which of `name`, `body` and `shared` changed; `from_name` is the name before a change to a prompt that stays shared; an unshare names the prompt by its name while it was shared, never its new private name; a change that alters nothing has reason `no_change`; a successful change to a prompt that is private before and after is never recorded, a refused or failed request on one is recorded without its name, and the text is never recorded |
| `datasource.test`, `grafana_connection.test` | `type`, `host`, `healthy` | An Admin's connection test, with the datasource's type (datasources only), the host that was called and whether it answered healthy; an unhealthy test has outcome `failed` and reason `unhealthy`; the address, any credential and the response are never recorded |
| `grafana_connection.fetch_dashboard` | `host` | An Admin's fetch of a dashboard from a link, naming the connection that was used and its host; the link and the dashboard are never recorded |
| every action, when `outcome` is `denied` | `needed_role`, `needed_access` | The role or the folder access level the refusal said the request needed, over the API and over MCP, left out when the refusal named none; a refusal raised by a plain role check that names none, and failed and successful events, never carry them |
| `run.finish` | `model`, `input_tokens`, `output_tokens` | The model that ran and the tokens it used, as the run reported them, left out when the run reported none; the activity feed does not carry them |
| `thread.open`, `thread.message`, `chat.turn` | `body_length`, `anchor_kind` | The length of the message or prompt in characters, never its text, and the kind of point its thread or turn is anchored to, such as `series` or `position`, left out when there is none |

Events carry no `visible_to` field.
Who can see a run is recorded by the `run.share` and `run.withdraw` events instead.

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
A `date_histogram` takes an optional `time_zone`, `UTC` or a named zone such as `Europe/Berlin`, and its buckets then start on that zone's clock, so a `1d` bucket starts at local midnight; an offset such as `+05:30` is refused.
An aggregation whose buckets would make more than 100,000 rows is refused with the reason.

```json
{"aggs": {"by_origin": {"terms": {"field": "origin"}, "aggs": {"people": {"cardinality": {"field": "actor_id"}}}}}}
```

A query that cannot be read answers 422 `query_invalid` with the parser's message and `start` and `end`, the byte offsets in the query to underline; over MCP the message names the same offsets, and the CLI prints the query with a caret line under the span and keeps `start` and `end` in the `--json` error.

A word between two terms that is a known misspelling of an operator (`andd`, `adn`, `nad`, `annd`, `orr`, `nto`, `nott`), or in Lucene an operator in the wrong case (`a or b`, `a And b`), is still searched as text, so the query keeps its meaning, but the search, facets, histogram and aggregate answers list it in `warnings`, such as `"andd" is searched as text; did you mean "and"?`.
In a Lucene query a lowercase `and` gets the same warning, since only `AND`, `OR` and `NOT` are operators there.
`warnings` is an empty list when there is nothing to warn about; the CLI prints each on standard error as `warning: ...`.

## Window and limits

Every read covers a window from `since`, inclusive, to `until`, exclusive.
Each takes an RFC 3339 time, epoch milliseconds or a relative time such as `now-7d`; `until` is now by default and `since` 24 hours before `until`.
A window longer than 366 days is refused with 400 `invalid_request`; read a longer period one window at a time.
Each read runs in a read-only transaction with a 30 second statement timeout and answers 422 `query_timeout` past it.

A search returns 100 events a page by default and at most 1,000, newest first by `occurred_at` and then `id`.
`next_cursor` pages through one search without repeating or skipping an event, even while new events are recorded, because it keeps the window of the first page; a cursor sent with another query is refused.
An export streams up to 1,000,000 events, the value of its `X-Export-Row-Limit` header, and stops there.
A complete export ends with the HTTP trailer `X-Export-Truncated`: `true` when more matching events existed past the limit, `false` otherwise.
When a read fails after the first rows were sent (a statement timeout, a lost database connection), the server cuts the connection off instead of ending the response, so the client sees an unexpected end of the body rather than a short file that looks complete; the failed export is recorded with its row count.
Before each page after the first the server re-reads the caller's credentials and role, and an export whose caller is no longer an Admin is cut off the same way and recorded as denied with `forbidden_role`.

## Following new events

`GET /api/v1/audit/stream?q=<query>` is a server-sent event stream of the events that match the query and are recorded after it opens, in the order they committed, one `audit` frame each with the event as its data.
It takes the same query text as a search and no window: a new stream sends events from the 10 minutes before it opened onward and never one recorded earlier.
Each frame's `id` is its position in the trail and the event's time; send it back as `Last-Event-ID` to resume after it.
The first frame, `ready`, carries the id of the position the stream starts from, so a client cut off before any event resumes without missing one.
A resumed stream sends every matching event committed after that frame whose time is at most 10 minutes before the frame's event, however long the client was away.
A `Last-Event-ID` whose event is more than 24 hours old answers 400 `invalid_request`; read the gap with a search, then open a new stream.
An id the stream did not send answers 400 `invalid_request` too.

Opening the stream records one `audit.search` event with the query, not one per event it delivers.
The server checks every 5 seconds that the caller is still an Admin, and ends the stream with a `forbidden` frame (`forbidden_role`) once they are not; it also ends at shutdown and after 30 minutes, and a client reconnects from its last id.

A caller may hold 5 audit streams and a server 20; one more answers 429 `too_many_streams`.
Each stream reads the trail every 2 seconds, one read at a time, under a 5 second statement timeout, and a stream whose read times out twice in a row ends with an `error` frame (`query_too_expensive`) saying the query is too expensive to follow; narrow it or search instead.

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
| Follow new events | `GET /api/v1/audit/stream` | `search_audit_events` | `observe audit search --follow` |

The filter is the `q` parameter of the API and the `query` argument of the MCP tools and the `--query` flag of the CLI.
Every route answers 403 `forbidden_role` to a caller below Admin.
An export has no MCP tool, because an MCP answer holds at most 16 KiB; an agent pages the same events with `search_audit_events`, 20 a call by default and at most 100.
When the 16 KiB cap, not `limit` or the end of the results, ends a page early, the answer carries `truncated: true` and a `note` that says how many events it kept; `next_cursor` continues from there.
The stream has no MCP tool of its own, as the activity stream has none: an agent reads new events by calling `search_audit_events` again.
No route or tool changes or deletes an audit event.

## The audit datasource

Every server has the built-in datasource `slashobserve-audit`, named Audit trail, which serves this trail in the OpenSearch format so the Logs page, panels and the agent read it like an OpenSearch datasource.
It is listed with type `opensearch` and source `builtin`, only Admins and Admin keys see or query it, and nobody can edit or remove it; see [Datasources in the app](../connect/datasources-in-the-app.md).
A datasources file entry with the uid `slashobserve-audit` stops the server at start with `audit_datasource_failed`.

The four starter dashboards in the Audit folder are read-only and the server brings each up to date with its release at every start; see [Audit folder](../use/activity.md#audit-folder).
In the web app, **Audit**, then **Explore**, opens the Logs page on this datasource, and **Audit**, then **Folder**, opens the Audit folder.

It takes the three query forms above: a KQL range query counts the matching events per interval, a logs panel or an instant query returns events newest first, and a DSL body with `aggs` returns series when it holds a `date_histogram` and a table when it does not.
Each event comes back with the fields of [Fields](#fields) and a `message` line such as `dashboard.update denied (forbidden_role) by Sam via web on dashboard "Checkout"`; `outcome` stands in for a log level.
The Logs page histogram counts `failed` as error, `denied` as warning, `success` as info and `unknown` as other.
The time field is `occurred_at`, and the field list marks the fields that `terms` can group by.

It answers searches sorted by `occurred_at`, oldest or newest first, paged with `from` and `size` up to 10,000 events deep, with totals counted up to 10,000 and at most 1,000 events a read.
It answers `date_histogram` and `terms` buckets, `date_histogram` with a `time_zone` as above, and the `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles` metrics.
A request holds at most 20 aggregations, nested ones included, and its `date_histogram` buckets, empty ones filled in, come to at most 11,000; a request over either is refused as an invalid query.
Any other request, body key, sort field or aggregation is refused with 400 and a message that names it, which a panel shows as its query error.

Reading through it is recorded as `query.run`, never as `audit.search`: the Logs page and the agent's queries record one event per query, with the datasource uid as its target, and a panel records nothing beyond the dashboard view.
Each such `query.run` that succeeds names the datasource in `datasource_uid` and holds `from` and `to`, the range it read as RFC 3339 times.
The same query from the same caller on the same route over the same period within 10 minutes is recorded once when it succeeds, and a request is left out only when every search in it was such a repeat; a refused or failed query is recorded every time.
The period is the range's length to the minute and the start of the stretch of that length it begins in, so a panel that refreshes the last 15 minutes is recorded at most once each 15 minutes, while a wider or other range is recorded on its own.

An agent run reads it only when everyone who can read what the run writes is an Admin, so an answer never shows audit data to someone who could not query it.
An Admin's private chat reads it, whether or not the chat is about a dashboard or an observation, because the answer stays the Admin's until they share it.
A thread ask, a build, an investigation, a revision or a suggestion reads it only when the thread's dashboard or observation, the build's folder, the observation it files into or the dashboard it changes is in a folder only Admins can read, such as Audit.
Anywhere else the run does not see it: its brief says the audit data is available in a private chat or in a folder only Admins can read, and a query on it answers the tool error `admins_only_datasource`, recorded as a denied `agent.tool_call`.
The same rule holds for the built-in agent and for a connected agent that took the run over MCP, because both reach datasources through the run's session.

The whole fleet runs at most 4 queries on it at once, and its queries are left out of the [proxy metrics](../operate/metrics.md) so `/metrics` never shows when Admins read the trail.

