---
title: Audit queries
description: How to search, count, export and follow the audit trail over the CLI, API and MCP, with the query forms, the event fields, the data of each event, the window and the limits.
---

The audit trail records every sign-in, read, change, agent tool call and refusal, with who did it, through what, to what and how it ended.
Only Admins read it, and an Admin service-account key can too.
Every read of the trail is itself recorded, so the trail shows who searched it for what.

## Search the trail

In the web app, choose **Audit**, then **Log**; see [Audit log](../use/activity.md#audit-log).
To search from a terminal or a script, use the CLI, the HTTP API or MCP.
All three read through the same server code and need an Admin credential.

1. Sign in once with an Admin service-account key, as [CLI](../use/cli.md#sign-in) shows.
2. Search for events, newest first:

   ```sh
   observe audit search --query 'outcome:denied and origin:mcp' --from now-7d
   ```

   You see one line per event: when, who, through what, the action, how it ended and its target.
3. Open one event with the other events of its request and run:

   ```sh
   observe audit show <event id>
   ```

4. For the raw JSON, call the HTTP API:

   ```sh
   curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
     --get "$OBSERVE_SERVER/api/v1/audit/events" \
     --data-urlencode 'q=outcome:denied and origin:mcp' \
     --data-urlencode 'since=now-7d'
   ```

   An agent calls the `search_audit_events` tool with the same `query`.

The window is the last 24 hours unless you set one.
The CLI takes `--from` and `--to`, and the API takes `since` and `until`.
An empty query matches every event in the window.

## Write a query

A query is KQL unless it starts with `lucene:` or `{`.
Use the fields from [Fields](#fields).

KQL, the default, joins `field:value` terms with `and`, `or` and `not`, and groups them with parentheses:

```text
outcome:denied and origin:mcp
action:dashboard.* and not actor_name:"Sam Lee"
status >= 500 or reason:query_timeout
target:dashboard/8f3c2a1e-0d6f-4b7e-9d39-5a7f0f0c4a11
client_ip:10.4.0.0/16 and initiator:agent
```

Lucene, after a `lucene:` prefix, uses `AND`, `OR`, `NOT`, `+` and `-`, ranges in brackets and wildcards at the end of a word.
Fuzzy and proximity searches are refused.

```text
lucene: outcome:denied AND origin:(mcp OR cli)
lucene: status:[500 TO 599] -action:auth.*
lucene: occurred_at:[2026-11-01 TO 2026-11-08}
```

The Query DSL is a JSON body that starts with `{`, with an optional `query`, `size` (the page size of a search) and `aggs`.

```json
{"query": {"bool": {"filter": [{"term": {"outcome": "denied"}}, {"range": {"status": {"gte": 400}}}]}}, "size": 50}
```

Free text, a word with no field, matches the action, the actor's and the target's names, the reason, the tool, the route and a query's text.

## Fix a query that is refused

A query that cannot be read answers 422 `query_invalid` with the parser's message, and with `start` and `end`, the byte offsets in the query to underline.
Over MCP the message names the same offsets.
The CLI prints the query with a caret line under the span and keeps `start` and `end` in the `--json` error.

A query with a likely typo is still searched, and the answer lists a warning.
A word between two terms that is a known misspelling of an operator (`andd`, `adn`, `nad`, `annd`, `orr`, `nto`, `nott`) is searched as text.
So is an operator in the wrong case in Lucene (`a or b`, `a And b`), because only `AND`, `OR` and `NOT` are operators there.
The search, facets, histogram and aggregate answers list it in `warnings`, such as `"andd" is searched as text; did you mean "and"?`.
`warnings` is an empty list when there is nothing to warn about, and the CLI prints each on standard error as `warning: ...`.

A query that names an unknown field says which valid fields are nearest to it.

## Count and group events

1. Count the top values of a field:

   ```sh
   observe audit facets actor_name --query 'action:dashboard.*'
   ```

2. Count events over time:

   ```sh
   observe audit histogram --query 'outcome:denied' --from now-7d
   ```

3. Group and total with a Query DSL body that has `aggs`:

   ```sh
   observe audit aggregate --from now-7d \
     --query '{"aggs": {"by_origin": {"terms": {"field": "origin"}, "aggs": {"people": {"cardinality": {"field": "actor_id"}}}}}}'
   ```

A `date_histogram` takes an optional `time_zone`, `UTC` or a named zone such as `Europe/Berlin`.
Its buckets then start on that zone's clock, so a `1d` bucket starts at local midnight.
An offset such as `+05:30` is refused.
`aggs` runs only through the aggregate endpoint; [Aggregation limits](#aggregation-limits) lists what it accepts.

## Export events

`observe audit export` writes events as NDJSON or CSV:

```sh
observe audit export --from 2026-11-01T00:00:00Z --to 2026-11-08T00:00:00Z --format csv --file audit.csv
```

Without `--file` it writes to standard output.
Over HTTP, `GET /api/v1/audit/export` streams the same events.
An export has no MCP tool, because an MCP answer holds at most 16 KiB.
An agent pages the same events with `search_audit_events`.

## Follow new events

`GET /api/v1/audit/stream?q=<query>` is a server-sent event stream.
It takes the same query text as a search and no window.
It sends each event that matches the query and is recorded after it opens, in the order the events committed.
The CLI does the same:

```sh
observe audit search --query 'outcome:denied' --follow
```

It prints the matching events recorded from now on until you press Ctrl-C, and `--json` prints each as one JSON line.
`--follow` cannot be combined with `--all`, `--cursor`, `--limit`, `--from` or `--to`.
When the connection drops, the CLI reconnects from the last event it printed.
The stream has no MCP tool of its own.
An agent calls `search_audit_events` again to read new events.
[Stream details](#stream-details) lists the frames and limits.

## Reference

### Fields

Every event has these fields.
`GET /api/v1/audit/fields`, `list_audit_fields` and `observe audit fields` list them with their descriptions.
Field names ignore case.
A field with an alias takes either name in a query, in KQL, Lucene and a Query DSL body alike.
The query lint of the audit datasource accepts either name, and the `fields` list shows every alias.

| Field | Alias | Type | What it holds |
| ----- | ----- | ---- | ------------- |
| `occurred_at` | | date | When the event happened |
| `id` | | uuid | Event id |
| `category` | | enum | `auth`, `access`, `content`, `conversation`, `agent`, `query`, `admin` or `system` |
| `action` | | text | What was attempted, such as `dashboard.update`; `dashboard.*` matches a prefix |
| `outcome` | | enum | `success`, `denied`, `failed` or `unknown` |
| `reason` | | text | The API error code of a refusal or failure, such as `forbidden_role` |
| `status` | | int | HTTP status of the request |
| `duration_ms` | `duration` | int | How long the action took, in milliseconds |
| `actor_kind` | `actor.kind` | enum | Whose permissions were used: `user`, `service_account`, `anonymous` or `system` |
| `actor_id` | `actor.id` | uuid | The user or service account |
| `actor_name` | `actor`, `actor.name` | text | The actor's name when the event happened |
| `actor_email` | `actor.email` | text | The user's current email address |
| `actor_role` | `actor.role` | text | The actor's role when the event happened |
| `initiator` | | enum | Who started it: `person`, `agent` or `system` |
| `agent_id`, `agent_name` | | uuid, text | The agent that took the run |
| `run_id` | `run` | uuid | The agent run that made the event |
| `auth_method` | | enum | `session`, `api_key`, `oauth`, `public_link` or `none` |
| `credential_id` | | text | The session, key, grant or public link used, never the secret |
| `origin` | | enum | `web`, `cli`, `mcp`, `api`, `agent`, `job`, `provision` or `unknown` |
| `origin_client` | | text | The connected client's name |
| `tool` | | text | The MCP or agent tool |
| `client_ip` | `ip` | ip | The caller's address; a query takes an address or a CIDR such as `10.4.0.0/16` |
| `user_agent` | | text | The request's user agent |
| `request_id` | `request` | uuid | The request id, also the `X-Request-Id` response header |
| `trace_id` | | text | The request's trace id, when one was sent |
| `http_method`, `route` | | text | The request's method and route template |
| `target` | | target | What the event was about, as `kind/id` |
| `target_kind`, `target_id`, `target_name` | `target.kind`, `target.id`, `target.name` | text | The parts of the target, and its name when the event happened |
| `folder_id`, `dashboard_id`, `observation_id`, `thread_id`, `chat_id` | | uuid | The objects the event touched |
| `datasource_uid` | `datasource` | text | The datasource a query ran on |
| `server_id`, `server_version` | | text | The server replica and build that recorded the event |

Events also carry `seq` and `data`, which a query cannot name.
`data` holds the action's own fields; see [Event data](#event-data).

### Details of the fields

- `outcome` is `unknown` for a redirect or an event whose commit the server could not confirm.
  A `run.finish` is `success` when the run saved its result and `failed` when it failed, timed out or was canceled.
  Its `result` says which of `succeeded`, `failed` or `canceled`.
- A `run.finish` recorded by a server older than 0.1.4 says `success` whatever the run's result.
  Read `data.result` on those events, which a query cannot filter on.
  Every later one agrees with its run.
- `actor` alone also matches the actor's current email address.
- `actor_email` is read from the user and not stored in the event.
  A service account has none, and a query can match it but not count it.
- `agent_id` and `agent_name` appear on every event of a run session: its tool calls, the changes they make, and `run.start`, `run.finish` and `run.release`.
  The value is `Observe Agent` with no id, a connected agent's grant id and client name, or a service account's id and name.
  They are empty on what a person does to a run, such as `run.queue`, `run.cancel` and `run.keep`.
  They are also empty on the `run.finish` of a run that no agent took (canceled while queued, or expired unclaimed), and outside a run.
- `origin` is `agent` for what the built-in agent does with its run key, and `unknown` for history copied from activity that recorded no origin.
- `client_ip` behind a proxy is the proxy's address unless `SLASHOBSERVE_TRUSTED_PROXIES` lists the proxy.
  See [Put TLS in front](../operate/production.md#put-tls-in-front).
- `target` takes `kind/id` such as `dashboard/8f3c...`, and `dashboard` alone and `dashboard/8f*` work too.

### Event data

An event's `data` holds fields that belong to its action and that a query cannot name.
Read them from a result.
The server keeps only the fields an action declares, and no event holds a message body, a prompt or an answer.

| Action | Data | What it holds |
| ------ | ---- | ------------- |
| `dashboard.view` | `version` | The version number the reader was shown |
| `observation.view` | `updated_at` | When the observation last changed before the reader opened it, as an RFC 3339 time |
| `observation.arrange` | `title`, `changed` | Which of `height`, `order` and `width` changed |
| `saved_prompt.*` changes | `owner_id`, `changed`, `from_name` | A change to a shared prompt; see the details below |
| `datasource.test`, `grafana_connection.test` | `type`, `host`, `healthy` | An Admin's connection test |
| `grafana_connection.fetch_dashboard` | `host` | The connection used and its host |
| `context_source.create`, `context_source.delete` | `name`, `host`, `audience`, `private` | A context source; never its secret; `private` is true only for a source the context sources file allows to use a private address |
| `context_source.update` | `name`, `host`, `audience`, `private`, `changes`, `url_changed`, `credential_changed` | What changed in a context source; `credential_changed` says a secret or its header changed, never what it is |
| `context_source.discover` | `host`, `private`, `found`, `new`, `changed`, `gone`, `ignored` | One discovery; a failed one carries the failure code as its reason instead of the counts |
| `context_source.approve`, `context_source.revoke` | `host`, `private`, `tool`, `hash`, `provisioned` | The tool and the SHA-256 of its pinned definition; `provisioned` is true only for an approval the context sources file made at start, which the system records as the actor |
| `context_source.call` | `host`, `private`, `tool`, `arguments`, `truncated` | One call of an approved tool; `arguments` are the values sent, cut short, with any named like a secret masked, and `truncated` says the result was cut. A refused or failed call carries its reason |
| any action that was denied | `needed_role`, `needed_access` | What the refusal said the request needed |
| `run.finish` | `model`, `input_tokens`, `output_tokens` | The model and tokens the run reported |
| `thread.open`, `thread.message`, `chat.turn` | `body_length`, `anchor_kind` | The message length in characters, and the anchor kind |
| `audit.search`, `audit.export` | `form`, `since`, `until`, `query`, `row_count` | The query as sent, its form, the window and the rows |
| `audit.search` | `view`, `field` | The read, and the field of a count |
| `audit.export` | `format`, `truncated` | The export's format, and whether it stopped at the limit |
| `query.run` | `query`, `row_count`, `from`, `to`, `stream` | One datasource query; see [How queries are recorded](#how-queries-are-recorded) |
| `auth.login_failed`, `auth.credential_rejected`, `auth.csrf_rejected` | `count`, `overflow` | Folded refusals; see [How refusals are folded](#how-refusals-are-folded) |

Events carry no `visible_to` field.
Who can see a run is recorded by the `run.share` and `run.withdraw` events instead.

### Details of event data

- `observation.view` records a time, as an RFC 3339 time, because an observation has no versions.
- `observation.arrange` is an arrangement of an observation's blocks that changed something, and is not an `observation.update`.
  Over the API, a request that changes nothing is still recorded, with reason `no_change` and without `changed`.
- A `saved_prompt.create`, `saved_prompt.update`, `saved_prompt.share`, `saved_prompt.unshare` or `saved_prompt.delete` event covers a prompt that is shared, or that becomes shared or private.
  `owner_id` is the prompt's owner, and `changed` lists which of `name`, `body` and `shared` changed.
  `from_name` is the name before a change to a prompt that stays shared.
  An unshare names the prompt by its name while it was shared, never its new private name.
  A change that alters nothing has reason `no_change`.
  A successful change to a prompt that is private before and after is never recorded.
  A refused or failed request on one is recorded without its name.
  The text is never recorded.
- A connection test records the datasource's `type` (datasources only), the `host` that was called and whether it answered `healthy`.
  An unhealthy test has outcome `failed` and reason `unhealthy`.
  The address, any credential and the response are never recorded.
- `grafana_connection.fetch_dashboard` is an Admin's fetch of a dashboard from a link, naming the connection that was used and its host.
  The link and the dashboard are never recorded.
- `needed_role` and `needed_access` hold the role or the folder access level the refusal said the request needed, over the API and over MCP.
  They are left out when the refusal named none.
  A refusal raised by a plain role check that names none, and failed and successful events, never carry them.
- `run.finish` leaves out `model` and the token counts when the run reported none.
  The activity feed does not carry them.
- `body_length` is the length of the message or prompt, never its text.
  `anchor_kind` is the kind of point the thread or turn is anchored to, such as `series` or `position`, and is left out when there is none.

### How reads of the trail are recorded

Searching, exporting and opening the stream are recorded as `audit.search` or `audit.export`.
The event holds the query as sent, with known secrets masked, cut at 4 KiB and marked `[truncated]`.
It also holds the query's form, the window and the row count.

- A plain search has `data.view` set to `events`.
  The counts of a field, the histogram, an aggregation and opening one event are `audit.search` too, with `data.view` set to `facets`, `histogram`, `aggregate` or `event`.
- A count of a field also holds `data.field`.
  Opening one event records the query `id:<event id>`.
- A read is recorded once per caller, view, field and query text in 10 minutes.
  The repeats, the pages after the first and a page that refreshes itself add no event.
  The first read after the 10 minutes is recorded again.
- The field list reads no event and records nothing.
- An export and each opening of the stream are recorded every time.
- A query that is refused is recorded every time, as a `failed` event with its reason.

### How views and lists are recorded

- A view of a chat or an agent request, as `chat.view` or `run.view`, is recorded once per reader and subject in 10 minutes.
  The owner's own views count.
- A list of chats or requests is one `chat.list` or `run.list` event, recorded once per reader and scope in 10 minutes, never one per row.
- The Agents page's list of one agent's runs is a `run.list` with scope `workspace` for an Admin or `you`.
  Its target is the connected agent or service account.
- A refused view is recorded every time.
- A call that answers 404 `not_found` for an id the caller cannot see is recorded every time.
  The event is a denied `request.refuse` or the route's own view action.
  Its target is the kind and id the caller asked for, and its reason is `not_found`.
  This holds for every route that names the id, whatever its method, including the dashboard versions, a diff, a folder, the datasource schema and a presence ping.
  Probing for hidden ids therefore leaves a trace.
- Any other 404 under a visible object, such as a dashboard that was never imported or a version that does not exist, is not recorded.
- An unknown trace id on a datasource the caller can read is a failed `query.run`, not a denial.

### How queries are recorded

Every query a person or an agent composes is a `query.run`.
Its target and its `datasource_uid` are the datasource's uid.
That covers the Logs, Metrics and Traces pages, the draft previews, the agent's and MCP query tools, and the CLI.

- `data.query` is the text that ran, cut at 4 KiB with ` [truncated]` appended.
  The secrets listed under [the exported record](../operate/audit-export.md#the-record) are replaced by `***`.
  Any other secret in a query text is stored as typed and exported with the event, so it reaches the trail.
- `data.row_count` is how many rows, points or spans came back.
- `data.from` and `data.to` are the range it read, as RFC 3339 times.
- A request or tool call that ran several queries on one datasource is one event.
  It has the distinct texts on separate lines, the rows added up and the widest range.
- The same caller's same query over the same period on the same datasource within 10 minutes is recorded once when it succeeds.
  A caller is one person, service-account key or agent run, so each run and each key records its own reads.
  A failed or refused query is recorded every time, and a live tail is never folded.
- The period is the range's length to the minute and the start of the stretch of that length it begins in.
  A page that refreshes the last 15 minutes is recorded at most once each 15 minutes, and a wider or other range is recorded on its own.
- A panel on a saved dashboard version is not recorded query by query.
  Its queries are fixed by the version, and the dashboard view carries it.
- A `query.run` whose `data` holds `stream: true` is a live tail of a logs query.
  It is written when the stream ends.
  Its `duration_ms` is how long the stream stayed open, up to the 30 minute limit, not how long a query took.

### How refusals are folded

Repeated refusals are folded so an attack cannot flood the trail.
`auth.credential_rejected`, `auth.csrf_rejected` and `auth.login_failed` events that share an action, reason, target, actor and provider error are one key.

- The first event of a key is written at once with `data.count` of 1.
- The rest of that key's minute become one later event whose `data.count` is how many more there were.
  There is no later event when there were none.
  The `count` of all the events of a key adds up to the refusals.
- When more than 1000 keys are open, refusals of new keys fold under one anonymous event marked `data.overflow`.
  It names no actor or target, and those events follow the same rule.

### Window and limits

Every read covers a window from `since`, inclusive, to `until`, exclusive.

| Item | Rule |
| --- | --- |
| `since`, `until` | An RFC 3339 time, epoch milliseconds or a relative time such as `now-7d`. |
| Default window | `until` is now, and `since` is 24 hours before `until`. |
| Longest window | 366 days. A longer one answers 400 `invalid_request`; read a longer period one window at a time. |
| Statement timeout | 30 seconds. A read past it answers 422 `query_timeout`. |
| Search page | 100 events by default, at most 1,000. |
| Search order | Newest first by `occurred_at`, then `id`. |
| Export | Up to 1,000,000 events. |
| Facet values | 10 by default, at most 100. |
| Histogram buckets | 60 by default, at most 200. |
| MCP search | 20 events by default, at most 100, in at most 16 KiB. |
| Event detail | Up to 50 events of the same request within an hour and 50 of the same run within a day. |

Each read runs in a read-only transaction.

### Details of paging and export

- `next_cursor` pages through one search without repeating or skipping an event, even while new events are recorded.
  It keeps the window of the first page, and a cursor sent with another query is refused.
- An export stops at the value of its `X-Export-Row-Limit` header.
  A complete export ends with the HTTP trailer `X-Export-Truncated`: `true` when more matching events existed past the limit, `false` otherwise.
- When a read fails after the first rows were sent, such as a statement timeout or a lost database connection, the server cuts the connection off.
  The failed export is recorded with its row count.
  The client sees an unexpected end of the body, not a short file that looks complete.
- Before each page after the first, the server re-reads the caller's credentials and role.
  An export whose caller is no longer an Admin is cut off the same way and recorded as denied with `forbidden_role`.
- When the 16 KiB cap, not `limit` or the end of the results, ends an MCP page early, the answer carries `truncated: true` and a `note` that says how many events it kept.
  `next_cursor` continues from there.

### Aggregation limits

The aggregate endpoint runs `aggs` with these limits:

- bucket aggregations are `date_histogram` and `terms`, up to three levels with one bucket aggregation per level;
- metrics are `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles`;
- a body holds at most 50 aggregations;
- a `terms` size is a whole number from 1 to 1000, 10 by default, and the sizes of nested levels multiply to at most 1000 groups;
- `percentiles` takes 1 to 20 percents between 0 and 100;
- a `date_histogram` interval is at most 366 days, and the window holds at most 11,000 of its buckets;
- an aggregation whose buckets would make more than 100,000 rows is refused with the reason.

The `query` of a Query DSL body takes `bool` (`must`, `filter`, `should`, `must_not`), `term`, `terms`, `match`, `match_phrase`, `range`, `wildcard`, `exists`, `multi_match`, `query_string`, `match_all` and `match_none`.

### Stream details

- Each frame is an `audit` event with the audit event as its data.
  A new stream sends events from the 10 minutes before it opened onward and never one recorded earlier.
- The first frame, `ready`, carries the id of the position the stream starts from.
  A client cut off before any event resumes without missing one.
- Each frame's `id` is its position in the trail and the event's time.
  Send it back as `Last-Event-ID` to resume after it.
  A resumed stream sends every matching event committed after that frame whose time is at most 10 minutes before the frame's event, however long the client was away.
- A `Last-Event-ID` whose event is more than 24 hours old answers 400 `invalid_request`.
  Read the gap with a search, then open a new stream.
  An id the stream did not send answers 400 `invalid_request` too.
- Opening the stream records one `audit.search` event with the query, not one per event it delivers.
- Every 5 seconds the server checks that the caller is still an Admin.
  It ends the stream with a `forbidden` frame (`forbidden_role`) once they are not.
  It also ends the stream at shutdown and after 30 minutes, and a client reconnects from its last id.
- A caller may hold 5 audit streams and a server 20.
  One more answers 429 `too_many_streams` for a caller over its limit and 503 `too_many_streams` for a server over its limit.
- Each stream reads the trail every 2 seconds, one read at a time, under a 5 second statement timeout.
  A stream whose read times out twice in a row ends with an `error` frame (`query_too_expensive`) saying the query is too expensive to follow.
  Narrow the query or search instead.

### API, MCP and the CLI

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

The filter is the `q` parameter of the API, the `query` argument of the MCP tools and the `--query` flag of the CLI.
The window is `since` and `until` in the API and `--from` and `--to` in the CLI.
Every route answers 403 `forbidden_role` to a caller below Admin.
No route or tool changes or deletes an audit event.

### The audit datasource

Every server has the built-in datasource `slashobserve-audit`, named Audit trail.
It serves this trail in the OpenSearch format, so the Logs page, panels and the agent read it like an OpenSearch datasource.
It is listed with type `opensearch` and source `builtin`.
Only Admins and Admin keys see or query it, and nobody can edit or remove it.
See [Datasources in the app](../connect/datasources-in-the-app.md).
A datasources file entry with the uid `slashobserve-audit` stops the server at start with `audit_datasource_failed`.

The four starter dashboards in the Audit folder are read-only, and the server brings each up to date with its release at every start.
See [Audit folder](../use/activity.md#audit-folder).
In the web app, **Open in Logs** on **Audit**, then **Log**, opens the Logs page on this datasource.
**Audit**, then **Dashboards** or **Observations**, lists what the Audit folder holds.

### Details of the audit datasource

- It takes the three query forms above.
  A KQL range query counts the matching events per interval.
  A logs panel or an instant query returns events newest first.
  A DSL body with `aggs` returns series when it holds a `date_histogram` and a table when it does not.
- Each event comes back with the fields of [Fields](#fields) and a `message` line such as `dashboard.update denied (forbidden_role) by Sam via web on dashboard "Checkout"`.
  `outcome` stands in for a log level.
- The Logs page histogram counts `failed` as error, `denied` as warning, `success` as info and `unknown` as other.
- The time field is `occurred_at`, and the field list marks the fields that `terms` can group by.
- It answers searches sorted by `occurred_at`, oldest or newest first, paged with `from` and `size`.
  A search reaches at most 10,000 events deep, counts totals up to 10,000 and returns at most 1,000 events a read.
- It answers `date_histogram` and `terms` buckets, `date_histogram` with a `time_zone` as above, and the `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles` metrics.
- A request holds at most 20 aggregations, nested ones included, and a `terms` size of at most 1000.
  Its `date_histogram` buckets, empty ones filled in, come to at most 11,000.
  A request over any of these is refused as an invalid query.
- The request body is at most 1 MiB.
- Any other request, body key, sort field or aggregation is refused with 400 and a message that names it, which a panel shows as its query error.
- The whole fleet runs at most 4 queries on it at once.
  Its queries are left out of the [proxy metrics](../operate/metrics.md), so `/metrics` never shows when Admins read the trail.
- Reading through it is recorded as `query.run`, never as `audit.search`, with the datasource uid as its target and the same `data` as any other datasource.
  A panel on a saved dashboard records nothing beyond the dashboard view.
  The repeat rule of [How queries are recorded](#how-queries-are-recorded) keeps a dashboard that refreshes from filling the trail.

### Who an agent run can read the trail for

An agent run reads the audit trail only when everyone who can read what the run writes is an Admin, so an answer never shows audit data to someone who could not query it.

- An Admin's private chat reads it, whether or not the chat is about a dashboard or an observation, because the answer stays the Admin's until they share it.
- A thread ask, a build, an investigation, a revision or a suggestion reads it only when one of these is in a folder only Admins can read, such as Audit:
  - the thread's dashboard or observation;
  - the build's folder;
  - the observation it files into;
  - the dashboard it changes.
- Anywhere else the run does not see it.
  Its brief says the audit data is available in a private chat or in a folder only Admins can read.
  A query on it answers the tool error `admins_only_datasource`, recorded as a denied `agent.tool_call`.
- The rule holds for the built-in agent and for a connected agent that took the run over MCP, because both reach datasources through the run's session.
