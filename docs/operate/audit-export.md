---
title: Audit export
description: Send every audit event to an OpenTelemetry collector or any OTLP/HTTP logs receiver, in order and at least once, with the settings, the record layout and the metric to watch.
---

The audit trail can be copied as it grows to a log pipeline you already run.
Set `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` and the server posts each audit event as an OTLP log record, so a collector, a SIEM or a log store keeps a copy that outlives the audit retention.
Nothing is sent while the setting is unset.

## Settings

| Variable | Default | Notes |
| --- | --- | --- |
| `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` | unset | An absolute `http` or `https` URL of an OTLP/HTTP receiver, such as `http://collector:4318`. The server posts to this URL followed by `/v1/logs`. |
| `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE` | unset | A file of request headers, one `Name: value` per line, for a receiver that needs a token. It needs the endpoint, and it is the only place a header value is read from. |

A bad value stops the server at start with the name of the setting.
The endpoint and the header values are never logged.
Keep the headers file out of the image and mount it as a secret.

## How it delivers

One server holds the export at a time, through the `audit_export` job described in [Scheduler](../concepts/scheduler.md).
It reads events in the order they were committed, from a position stored in the database, and posts up to 500 at a time as OTLP JSON.
The position moves only after the receiver answers with a `2xx` status, so a failed post sends the same events again and no event is dropped.
A restart, or another server taking the job over, continues from the stored position.
Delivery is at least once: an event can arrive twice when the server stopped between the receiver's answer and storing the position, and `slashobserve.audit.id` identifies it.

A failed post is retried with a growing wait from 1 second to 1 minute, and a `Retry-After` header of the receiver is honoured up to that limit.
Failures are logged with a fixed message and a code, never with the URL or the event.
The first export starts at the oldest event the audit table still holds, so a new setting sends the existing history before the new events.

The request goes out through the same dialer as the datasources, so the endpoint is an operator choice and is not held to the [outbound domain list](outbound-access.md).
It does not follow redirects, and it refuses link-local addresses.

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

## Watching it

`slashobserve_audit_export_lag_seconds` is the age of the oldest event waiting at the holding server's last attempt, and `0` when it is caught up or when that server does not hold the export.
Alert when it stays high, which means the receiver refuses events or the job is not running.
See [Metrics](metrics.md) for the other audit series.
