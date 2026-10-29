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

## ClickHouse needs a read-only user

SlashObserve queries ClickHouse only as a user with `readonly = 2`, and it refuses a user that is not read-only.
`readonly = 2` rather than `1` is required because the server sets per-query limits.
The demo defines such a user with a settings profile; use `internal/datasource/clickhouse/testdata/readonly_user.sql` as the template, replacing `__PASSWORD__`:

    CREATE SETTINGS PROFILE IF NOT EXISTS slashobserve_ro_profile SETTINGS
        readonly = 2 CONST,
        max_execution_time = 30 MIN 1 MAX 300,
        max_result_rows = 10000 MIN 1 MAX 1000000,
        max_result_bytes = 104857600 MIN 1 MAX 1073741824,
        result_overflow_mode = 'throw' CONST;

    CREATE USER IF NOT EXISTS slashobserve_ro IDENTIFIED WITH sha256_password BY '__PASSWORD__'
        SETTINGS PROFILE 'slashobserve_ro_profile';

    GRANT SELECT ON otel.* TO slashobserve_ro;

Grant `SELECT` only on the database that holds your OpenTelemetry tables.
