---
title: Connect ClickHouse
description: Add a ClickHouse server as a datasource for logs, traces and metrics, with the read-only user it requires, a first SQL query and the tables the pages read.
---

Connect a ClickHouse server that holds OpenTelemetry data to search logs and traces with SQL and chart them in dashboards.
SlashObserve queries ClickHouse only as a read-only user, so you create that user first.
An Admin adds the entry to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect a ClickHouse server

1. Create the read-only user.
   Run these statements as a ClickHouse admin, with your own password:

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
   The demo defines the same user in `internal/datasource/clickhouse/testdata/readonly_user.sql`, with the development password `slashobserve`.
2. Put the password in the server's env file, for example `SLASHOBSERVE_CLICKHOUSE_PASSWORD=<password>`.
3. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names:

   ```yaml
   datasources:
     - uid: ch
       type: clickhouse
       name: ClickHouse
       clickhouse:
         addr: clickhouse:9000
         database: otel
         username: slashobserve_ro
         passwordEnv: SLASHOBSERVE_CLICKHOUSE_PASSWORD
   ```

4. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
5. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck ch
   ```

   A healthy datasource shows no failure.
   An unhealthy one shows why, such as "The ClickHouse user must be readonly = 2".
6. Run a first query:

   ```sh
   observe logs 'SELECT Timestamp, Body FROM otel_logs WHERE $__timeFilter(Timestamp) LIMIT 10' --datasource ch --from now-1h
   ```

   You see up to 10 log lines with their time and message.
   Single quotes keep the shell from expanding `$__timeFilter`.
   The same SQL works on the [Logs page](../use/logs-and-traces.md#write-and-run-a-query).

## Fix an unhealthy ClickHouse datasource

The health check pings the server, then checks the user.
Two checks fail on a user that is too powerful, and the datasource shows as unhealthy:

- The user is not `readonly = 2`.
  Use `2` rather than `1`, because the server sets per-query limits, and a user with `readonly = 1` is refused.
- The user holds a grant that reaches outside systems.
  The message is "The ClickHouse user holds a grant that reaches outside systems; revoke it".

The server checks with `CHECK GRANT` that the user holds neither `CREATE TEMPORARY TABLE` nor a read or write grant on a source such as `FILE`, `URL`, `REMOTE`, `S3` or `MYSQL`.
That covers a grant given directly, through a role, or through `SOURCES` or `ALL`.

## Keep the digits of large numbers

A result carries numbers as floating point, which holds integers exactly only up to 2^53.
In a table, a column with an integer above 2^53 or a decimal whose whole part is above 2^53 comes back as a string column holding every digit.
In a time series such a value is still plotted as a number, rounded, and the frame carries `meta.warning` saying so.
Select the column as a string, for example `toString(id)`, to keep the digits in a series.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `addr` | Required. The native-protocol `host:port`. |
| `database` | Default `otel`. |
| `username` | Required. |
| `passwordEnv`, `passwordFile` | Exactly one is required. |
| `tls` | `true` connects over TLS 1.2 or newer. Default `false`. |

A result over 32 MiB is refused as exceeding a limit, whatever the row limit.

### What the logs and trace pages read

The logs page starts from a template over `otel_logs`, and the trace search reads `otel_traces`.
Both are in the datasource's `database`, with the column names of the OpenTelemetry Collector's ClickHouse exporter:

- For logs: `Timestamp`, `SeverityText`, `Body`, `ServiceName`, `TraceId`, `SpanId` and `LogAttributes`.
- For traces: the same ids and service, plus `ParentSpanId`, `SpanName`, `SpanKind`, `Duration`, `StatusCode`, `StatusMessage`, `SpanAttributes`, `ResourceAttributes`, and the `Events` and `Links` nested columns.

The services list reads the `ServiceName` column of whichever of the two tables the signal names.
Metric names come from the `otel_metrics_*` tables.

On the logs page, and in the SQL view of the trace search, you write the query as SQL.
It must be one read-only `SELECT`.
It must read the time range through a macro such as `$__timeFilter(Timestamp)`, which names the table's time column, because the server never reads a table beyond the range at the top of the page.
[Logs and traces](../use/logs-and-traces.md#write-and-run-a-query) describes the rules and the errors.
