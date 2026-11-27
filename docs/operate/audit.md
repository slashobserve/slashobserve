---
title: Audit trail
description: What the audit trail records and leaves out, where it is stored, how retention and write pressure work, which metrics and log lines to watch, who can read it, how it is backed up and the settings that control it.
---

The audit trail is the server's record of who did what, through what, to what and how it ended.
This page is for the person who runs the server: how long it keeps events, what to watch, who can read it and how to back it up.

For reading and searching the trail, see [Audit queries](../reference/audit-queries.md) and [Activity](../use/activity.md#audit-log).
For copying it to another system, see [Audit export](audit-export.md).

## Check that the trail is healthy

The trail needs no setup.
To check it, look at these signals in [Metrics](metrics.md) and the logs.

| Signal | Meaning |
| --- | --- |
| `slashobserve_audit_dropped_total` | Any increase is a lost event, except `undeclared_key`, which counts a data key an action did not declare while the event itself was stored. |
| `slashobserve_audit_buffer_events` | Events in the buffer of this server at the last flush, out of 5,000. |
| `slashobserve_audit_write_seconds` | The time of each attempt to write a batch, a timed out attempt included. |
| `slashobserve_audit_partitions_ahead` | Should read 14. Below 14 means the `audit_partitions` job is not running. |
| `audit batch not written` and `audit request event not written` | Log lines with an `error_code` and, for a database error, the `sqlstate`. |

`/metrics` exports no counts of events by action, person or outcome, because it is open to anyone.
Read those through the trail.

Alert on `slashobserve_audit_dropped_total` increasing and on `slashobserve_audit_partitions_ahead` below 14.

## Change how long events are kept

`SLASHOBSERVE_AUDIT_RETENTION` sets how long events are kept.
The default is `8760h`, one year.
The value is a Go duration from `24h` to `87600h`.

1. Set the variable, for example `SLASHOBSERVE_AUDIT_RETENTION=4380h`.
2. Restart the server.
   A value that does not parse or is out of range stops it at start with the name of the setting.
3. Check that `slashobserve_audit_partitions_ahead` reads 14 after the start.

The `audit_partitions` job applies the period, and it runs at every start.
So a shorter period drops the partitions that are now too old at that start, and the server cannot restore a dropped partition.
A longer period does not bring back what was already dropped.

The job drops each partition whose end is at or before the start of the UTC day that contains the time one retention period ago.
So an event is never deleted before it is older than the period, and with daily partitions it can stay up to a day longer.
The monthly and yearly partitions of upgraded history stay until their whole span is past the period.
A run that drops partitions records an `audit.prune` event with the number of partitions and the date they were cut off at.

If the job fails at start, the server logs `audit partitions not prepared` with the `error_code` `audit_partitions_failed` and keeps running.
The activity log has its own period, `SLASHOBSERVE_ACTIVITY_RETENTION`; see [Activity](../use/activity.md#retention).

## Let Admins read the trail

Admins, and Admin service-account keys, read the trail.

| Way in | Rule |
| --- | --- |
| HTTP | Every route under `/api/v1/audit` needs the Admin role, and the live stream checks it again every 5 seconds. |
| MCP | The six tools `search_audit_events`, `get_audit_event`, `list_audit_facets`, `get_audit_histogram`, `aggregate_audit_events` and `list_audit_fields` need an Admin. |
| CLI | `observe audit` with `search`, `show`, `facets`, `histogram`, `aggregate`, `fields` and `export` needs an Admin key. |
| Datasource | `slashobserve-audit`, named Audit trail, is built in and its audience is Admins. It cannot be edited or removed. |
| Folder | The Audit folder is Admins only and holds four starter dashboards on that datasource. |

Every read of the trail is itself recorded, so the trail shows who searched it.
A datasources file entry with the uid `slashobserve-audit` stops the server at start with the error code `audit_datasource_failed`.

The server creates the Audit folder once, at the first seeding, or adopts an existing folder named Audit that is Admins only.
While the folder exists, the server brings the starters up to date at every start.
A folder an Admin deletes is never created again; see [Audit folder](../use/activity.md#audit-folder).

If a folder named Audit already exists and is not Admins only, the server logs a warning and adds no starters until the folder is made Admins only.
If seeding fails, the server logs `could not add the Audit folder's starter dashboards` with the `error_code` `audit_folder_failed` and tries again at the next start.

## Copy the trail to another system

Set `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` to make one server at a time post the stored events to an OTLP/HTTP logs receiver, through the `audit_export` job.
[Audit export](audit-export.md) has the steps, the retry rules, the record layout and the metrics.

The server reads events in the order they were committed, from a position kept in the `audit_export_cursor` table of the same database.
It moves the position only after the receiver answers with a `2xx` status.
So delivery is at least once and in order for the events the receiver accepts, and a receiver should drop repeats by `slashobserve.audit.id`.

Events the receiver refuses are not delivered:

- When it answers `400`, `413` or `422` for a single event, the exporter moves past that event and counts it in `slashobserve_audit_export_dropped_total{reason="refused"}`.
- Events it refuses in a partial success reply are counted in `slashobserve_audit_export_rejected_total` and are not sent again.

Watch both counters when the copy must be complete.

Three points for the operator:

- The export copies stored events.
  An event dropped before it was stored never reaches the receiver, and an `audit.gap` event does.
- Delivery is guaranteed only while the receiver accepts events within the retention period.
  Retention still drops partitions that were not exported, and `slashobserve_audit_export_dropped_total{reason="retention"}` counts the gap.
- Each record carries the event's `data` as the JSON string `slashobserve.audit.data`, the client address and the user agent.
  That includes the query text of audit searches, so give the receiver the same access controls as the trail.

## Back up and restore the trail

The trail is in the same Postgres database as everything else, so the dump in [Upgrades and backups](upgrades-and-backups.md#back-up-postgres) holds it, and so does the export position.
The table holds client addresses, user agents, actor names and the query text of audit searches, so protect the dump like the database.

The `audit_partitions` job runs at start.
A server that starts on a restored database creates the missing partitions from today on, and it drops the partitions that are older than the retention period.

## Understand write pressure

Events reach the table by one of two paths.

A change writes its event inside the change's own transaction, so the buffer below never holds it.
The two commit or roll back together.

The request's own event is written straight to the database with a 5 second limit when its outcome is `unknown`, or when it is not a read, a tool call, a query or `denied`.
If that write fails with an error a retry can fix, such as a timeout, a lost connection, a lock that was not available or a read-only database during a failover, the writer retries it.
The server logs `audit event queued to be written again`.
If the error cannot be fixed by a retry, or the retry queue is full, the event is lost.
`slashobserve_audit_dropped_total{reason="record_failed"}` counts it, and the server logs `audit request event not written` with the `error_code` `audit_record_failed`.

Reads, tool calls, queries and `denied` events go into an in-memory buffer on each server instead.
The buffer holds 5,000 events.
The writer takes up to 500 at a time and writes them once a second, or sooner when 500 are waiting.
A write that takes more than 5 seconds is cut off.

| Situation | What happens |
| --- | --- |
| The buffer is full | The recorder waits up to 100 milliseconds for room, then drops the event and counts it as `buffer_full`. |
| A batch fails with an error a retry can fix | The writer keeps the batch and writes it again at each flush, for as long as the database stays unavailable. New events queue behind it. |
| A batch times out 3 times in a row, or Postgres cancels it | The writer splits it in half and writes the halves separately. |
| A batch fails with an error a retry cannot fix | The writer drops the batch and counts its events as `write_failed`. |
| A batch was already stored by an earlier attempt | The server logs it as already stored and does not write or count it again. |
| The server stops | The writer waits up to 5 seconds for requests still running, then writes what is left for up to 5 seconds. Events it cannot write count as `write_failed`. |

### How a drop shows

- `slashobserve_audit_dropped_total` increases, by `reason`.
  A series appears when its first drop happens.
- The next batch that is written carries one `audit.gap` event, whose `data.lost` is how many events were lost since the last one.
  Find it with the query `action:audit.gap`.
- The server logs `audit batch not written` at `WARN`, with the `error_code` `audit_write_failed` and the `sqlstate` of a database error.

The buffer is in memory.
Events in it when a server is killed or crashes are lost, and neither a counter nor an `audit.gap` event records them.

## Reference

### What is recorded

Every request under `/api/v1` and every mounted endpoint, such as `/mcp` and the OAuth endpoints, passes through the audit recorder.
Health checks and the static web files do not.

| Kind | What the trail holds |
| --- | --- |
| Changes | One event for each change, written in the same database transaction as the change. |
| Failed changes | When a request that should have recorded a change ends without a committed event, the recorder writes a `failed` event for it, with the reason `rolled_back` unless the request had a better one. |
| Unconfirmed changes | When the recorder cannot confirm whether the event committed, it writes an `unknown` event with the reason `commit_check_failed`. |
| Redirects | A redirect status (301, 302, 303, 307 or 308) on an audited route is recorded with the outcome `unknown`. |
| Denied requests | A request answered with 401, 403 or 429, or with 404 on a route that names its target, is recorded as `denied`. A refused request on a route with no audit action is recorded as `request.refuse`. |
| Repeated refusals | Repeated `auth.login_failed`, `auth.credential_rejected` and `auth.csrf_rejected` events are folded into one event a minute, with the count in `data.count`; see [Audit queries](../reference/audit-queries.md#fields). |
| Tool calls | One `agent.tool_call` event for each MCP tool call, with the tool name, the outcome, the reason and the duration. The arguments and the result are not recorded. |
| Datasource queries | One `query.run` event for each query, with the datasource uid and the time range it read. The query text is not recorded. |
| Reads | Views of a dashboard, an observation, a public link, a chat and a run, and lists of chats and runs. |
| Audit reads | A search, an export and each opening of the live stream are recorded as `audit.search` and `audit.export`, with the query text cut at 4 KiB. |
| System | Server start and stop, applied migrations, provisioning, scheduled job runs, `audit.prune` and `audit.gap`. |

Listing dashboards, folders and search results, and reading a dashboard version, are not recorded; each route says so in its declaration in the API description.
An event keeps only the data keys that the catalog in `internal/audit/catalog.go` declares for its action.
The server drops any other key, counts it as `undeclared_key`, and stores the rest of the event.

### Repeated reads

Some reads are recorded once in 10 minutes and not each time.
This holds for an audit search, a chat or run view and a chat or run list, and a query on the audit datasource.
A dashboard, observation or public link view is recorded each time.

The server remembers these reads in memory, in a separate table for each kind, with room for 10,000 entries.
A restart forgets them, and so does a second server, which records the read again.
When a table is full, a new read is recorded each time until older entries expire.

### Where it is stored

The trail is one table, `audit_events`, in the server's own Postgres database.
There is no other store.
Every server writes to it, and each event carries the `server_id` and `server_version` of the server that recorded it.

The table is partitioned by `occurred_at`:

- New events go into one partition for each UTC day, named `audit_events_YYYYMMDD`.
- The schema has no default partition, so an event dated for a day with no partition has nowhere to go.
  The `audit_partitions` job prevents this by creating the partitions for today and the next 14 days.
  It runs once at every server start and then daily.

The database refuses `UPDATE`, `DELETE` and `TRUNCATE` on the table.
No API route, MCP tool or CLI command changes or deletes an event.
The only removal is retention dropping a whole partition.

A row holds the actor's name and role as they were when the event happened, the client address, the user agent, the request id and the `data` keys of its action.
For an audit search it holds the query text.
The table has indexes on action, actor, target, run and request, and a full-text index.
It grows as events are recorded, and nothing but retention shrinks it.
The code gives no figure for its size.

### Settings

The audit settings are environment variables only; there are no configuration file keys.
The Helm chart has no audit values.
Set them under `env` in the values, as [Run on Kubernetes](kubernetes.md) describes.
The chart has no value that mounts an extra file, so it gives the headers file no mount.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SLASHOBSERVE_AUDIT_RETENTION` | `8760h` | How long events are kept, from `24h` to `87600h`. |
| `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` | unset | An absolute `http` or `https` URL of an OTLP/HTTP receiver, with no query, fragment or user info. Unset turns the export off. |
| `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE` | unset | A file of `Name: value` request headers, one per line, with no repeated name and at least one header. It needs the endpoint. |
| `SLASHOBSERVE_TRUSTED_PROXIES` | unset | The proxies the server trusts for the client address the trail records; see [Put TLS in front](production.md#put-tls-in-front). |

[Configuration](../reference/configuration.md#audit) lists the settings with the rest of the server's.

### Writer limits

The writer's limits are fixed in the code and have no setting.

| Limit | Value |
| --- | --- |
| Buffer | 5,000 events on each server |
| Batch | 500 events |
| Flush | Every second |
| Wait for room in a full buffer | 100 milliseconds |
| Write timeout | 5 seconds |
| Repeated reads | Recorded once in 10 minutes, 10,000 remembered for each kind |
| Partitions kept ahead | 14 days |
