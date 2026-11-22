---
title: Audit export
description: Send every audit event to an OpenTelemetry collector or any OTLP/HTTP logs receiver, in order and at least once, with the settings, the record layout and the metric to watch.
---

The audit trail can be copied as it grows to a log pipeline you already run.
Set `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` and the server posts each audit event as an OTLP log record, so a collector, a SIEM or a log store can keep a copy of the trail.
Nothing is sent while the setting is unset.

## Settings

| Variable | Default | Notes |
| --- | --- | --- |
| `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` | unset | An absolute `http` or `https` URL of an OTLP/HTTP receiver, such as `http://collector:4318`. The server posts to this URL followed by `/v1/logs`. |
| `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE` | unset | A file of request headers, one `Name: value` per line, for a receiver that needs a token. It needs the endpoint, and it is the only place a header value is read from. |

A bad value stops the server at start with the name of the setting.
The endpoint, the header values and the text of a receiver's answers are never logged.
Keep the headers file out of the image and mount it as a secret.

## How it delivers

One server holds the export at a time, through the `audit_export` job described in [Scheduler](../concepts/scheduler.md).
It reads events in the order they were committed, from a position stored in the database, and posts up to 500 at a time as OTLP JSON.
The position moves only after the receiver answers with a `2xx` status, so a failed post sends the same events again.
A restart, or another server taking the job over, continues from the stored position.
Delivery is at least once: the position write that follows a `2xx` answer is not cut short when the job's lease ends, but a database timeout can still fail it, and so can a crash, a lost database connection or any other failed write between the answer and the write.
The same batch of up to 500 events is then sent again, and again each time the write fails, until a position write succeeds, so an event can reach a receiver more than twice.
Receivers should deduplicate on the event id: `slashobserve.audit.id` identifies an event, so a receiver or a query can drop the repeats.

A receiver can answer `200` and still refuse some records in its `partialSuccess` answer, for example records that are too large or too old.
The server treats those as final: it moves the position past them, adds their count to `slashobserve_audit_export_rejected_total` and logs the count and the status.
What the receiver writes in its answer is never logged, because it can repeat a credential or an event.

A failed post is retried with the same events and a growing wait from 1 second to 1 minute, and a `Retry-After` header of the receiver is honoured up to that limit.
This holds for a network error, a timeout and every status except the three below, including `401`, `403` and `404`, which mean the endpoint or the headers are wrong: the export keeps trying and skips nothing until you fix the setting.

When the receiver answers `400`, `413` or `422`, it has refused the request itself.
The server halves the batch and tries again, down to one event.
An event the receiver still refuses alone is skipped: the position moves past it, `slashobserve_audit_export_dropped_total{reason="refused"}` counts it, and the log names its sequence number and the status.

Skipping is for one bad event among good ones.
After 3 events in a row are skipped with none accepted in between, the server stops skipping: it treats the refusal as a configuration problem, keeps the position where it is, retries with the growing wait like a `401` or `404`, and logs once that the receiver refuses every event.
That is what a wrong service behind the endpoint looks like.
An accepted event resets the count, and a restart does too.
Failures are logged with a fixed message and a code, never with the URL or the event.
The first export starts at the oldest event the audit table still holds, so a new setting sends the existing history before the new events.

The request goes out through the same dialer as the datasources, so the endpoint is an operator choice and is not held to the [outbound domain list](outbound-access.md).
It does not follow redirects.
A direct connection to a link-local address, such as a cloud metadata address, is refused.
The request honours `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` like the datasources do, so a collector behind a proxy can be reached; when a proxy is used, the server connects only to the proxy and the proxy decides which addresses it will reach.

## What is guaranteed

While the receiver accepts events within the audit retention period (`SLASHOBSERVE_AUDIT_RETENTION`), every event is delivered at least once and in order.
The export does not change retention: the daily `audit_partitions` job still deletes a partition once it is older than the retention period, whether or not its events were exported.
When the receiver was down, or refused every request, for longer than that, events were deleted before they could be sent.
The server notices when the event at the stored position is older than the oldest event the table still holds and the next event does not directly follow it, logs a warning with the size of the gap and adds one to `slashobserve_audit_export_dropped_total{reason="retention"}` for each gap, then continues with the oldest event that is left.
The gap is found at the start of a run and after a failed attempt, so it can take a few minutes to show.
A caught-up export on a quiet system reports nothing when retention deletes its last event, because the next event follows it directly; a sequence number skipped by a rolled-back change can, rarely, make a harmless report.
Events the export gave up on in other ways are counted too, as `refused` and in `slashobserve_audit_export_rejected_total`.

Alert on all three counters when the copy must be complete, and keep the retention period longer than the longest receiver outage you accept.

## The record

Each event is one log record in the `slashobserve.audit` scope, under the resource `service.name = slashobserve`.

| OTLP field | Holds |
| --- | --- |
| `timeUnixNano` | When the event happened |
| `eventName` and `event.name` | The action, such as `dashboard.update` |
| `body` | The line the Audit page and `observe audit` show for the event |
| `severityNumber` | 9 (`INFO`) for `success`, 13 (`WARN`) for `denied` and `unknown`, 17 (`ERROR`) for `failed` |
| `traceId` | The event's trace id, when it has a valid one |

The attributes use the standard names where one exists: `user.id`, `client.address`, `user_agent.original`, `http.route`, `http.request.method` and `http.response.status_code`.
Every other field is under `slashobserve.audit.`, for example `slashobserve.audit.outcome`, `slashobserve.audit.category`, `slashobserve.audit.actor_name`, `slashobserve.audit.target_kind` and `slashobserve.audit.run_id`; a field an event does not have is left out.
The full list of fields is in [Audit queries](../reference/audit-queries.md#fields).

The data of a `query.run` and of an `audit.search` event carries the query text, and the export sends it to the collector as it was recorded.
Before the text is cut at 4 KiB and stored, these secrets in it are replaced by `***`:

- The user and password of a URL, as in `scheme://user:pass@host`.
- The credential arguments of the ClickHouse functions and engines `s3`, `s3Cluster`, `gcs`, `azureBlobStorage`, `hdfs`, `url`, `mysql`, `postgresql`, `mongodb`, `redis`, `remote`, `remoteSecure`, `jdbc` and `odbc`, and the same arguments of `deltaLake`, `iceberg`, `hudi`, `s3Queue`, `azureBlobStorageCluster`, `MaterializedPostgreSQL` and `MaterializedMySQL`: access keys, secrets, passwords, connection strings and the values of `headers(...)`.
- The value after `password`, `passwd`, `pwd`, `secret`, `token`, `api_key`, `access_key` or `secret_key`, also when the name has a prefix such as `access_token`, `client_secret` or `X-Api-Key`, written as `key=value`, `key: value`, `key!=value` or `key=~value` in any case, with quoted, unquoted, unterminated or escaped values.
- The credential after `authorization` with any scheme (the scheme word stays), and a `Bearer` token.
- AWS access key ids, JSON web tokens and PEM private key blocks, wherever they appear.

Everything else is stored as typed and travels through the export, so a secret in a form this list does not name, such as a password in a free-text log search, is in the trail and in the collector, with the retention and readers of each.
Give the collector no wider an audience than the Admins who can read the trail.

## Watching it

`slashobserve_audit_export_lag_seconds` is the time between the newest audit event in the database and the last event the export sent.
Every server that has the export set reads it from the database every 15 seconds, whether or not it holds the job, so one scrape of any such server shows the real lag and it never reads `0` just because the job is not running.
A server without the export set exposes the series at `0` and never updates it, so read it from a server that has the export.
It stays above `0` when the receiver refuses events, when no server runs the `audit_export` job, and when a long-running transaction in the database hides newer events from the export until it ends.
Alert on it staying high for several minutes.
Before the first event is exported it is the span of the events the audit table holds.
See [Metrics](metrics.md) for the other audit series.
