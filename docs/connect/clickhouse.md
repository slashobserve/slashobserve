---
title: Connect ClickHouse
description: Add a ClickHouse server as a datasource, with the read-only user it requires.
---

A ClickHouse entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

## Fields

| Field                         | Notes                                                           |
| ----------------------------- | --------------------------------------------------------------- |
| `addr`                        | Required. The native-protocol `host:port`.                      |
| `database`                    | Default `otel`.                                                 |
| `username`                    | Required.                                                       |
| `passwordEnv`, `passwordFile` | Exactly one is required.                                        |
| `tls`                         | `true` to connect over TLS (TLS 1.2 or newer). Default `false`. |

## What the logs and trace pages read

The logs page starts from a template over `otel_logs` and the trace search reads `otel_traces`, both in the datasource's `database`, with the column names of the OpenTelemetry Collector's ClickHouse exporter: `Timestamp`, `SeverityText`, `Body`, `ServiceName`, `TraceId`, `SpanId` and `LogAttributes` for logs, and for traces the same ids and service plus `ParentSpanId`, `SpanName`, `SpanKind`, `Duration`, `StatusCode`, `StatusMessage`, `SpanAttributes`, `ResourceAttributes`, and the `Events` and `Links` nested columns.
The services list reads the `ServiceName` column of whichever of the two tables the signal names.
Metric names come from the `otel_metrics_*` tables.

On the logs page, and in the SQL view of the trace search, you write the query as SQL.
It must be one read-only `SELECT`, and it must read the time range through a macro such as `$__timeFilter(Timestamp)`, which names the table's time column, because the server never reads a table beyond the range at the top of the page.
[Logs and traces](../use/logs-and-traces.md#write-and-run-a-query) describes the rules and the errors.

## Large numbers

A result carries numbers as floating point, which holds integers exactly only up to 2^53.
In a table, a column with an integer above 2^53 or a decimal whose whole part is above 2^53 comes back as a string column holding every digit.
In a time series such a value is still plotted as a number, rounded, and the frame carries `meta.warning` saying so.
Select the column as a string, for example `toString(id)`, to keep the digits in a series.

## ClickHouse needs a read-only user

SlashObserve queries ClickHouse only as a user with `readonly = 2`, and it refuses a user that is not read-only.
`readonly = 2` rather than `1` is required because the server sets per-query limits.
The demo defines such a user with a settings profile in `internal/datasource/clickhouse/testdata/readonly_user.sql`, with the development password `slashobserve`; use the same statements with your own password:

```sql
CREATE SETTINGS PROFILE IF NOT EXISTS slashobserve_ro_profile SETTINGS
    readonly = 2 CONST,
    max_execution_time = 30 MIN 1 MAX 300,
    max_result_rows = 10000 MIN 1 MAX 1000000,
    max_result_bytes = 104857600 MIN 1 MAX 1073741824,
    result_overflow_mode = 'throw' CONST;

CREATE USER IF NOT EXISTS slashobserve_ro IDENTIFIED WITH sha256_password BY '<password>'
    SETTINGS PROFILE 'slashobserve_ro_profile';

GRANT SELECT ON otel.* TO slashobserve_ro;
```

Grant `SELECT` only on the database that holds your OpenTelemetry tables.
The server also refuses a user that could reach outside systems or create temporary tables.
It checks with `CHECK GRANT` that the user holds neither `CREATE TEMPORARY TABLE` nor a read or write grant on a source such as `FILE`, `URL`, `REMOTE`, `S3` or `MYSQL`, whether directly, through a role, or through `SOURCES` or `ALL`.
The datasource then shows as unhealthy with "The ClickHouse user holds a grant that reaches outside systems; revoke it".
A result over 32 MiB is refused as exceeding a limit, whatever the row limit.
