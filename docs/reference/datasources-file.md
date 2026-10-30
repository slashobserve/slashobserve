---
title: Datasources file
description: The YAML file that lists the Prometheus and ClickHouse servers, and its rules for secrets and limits.
---

The datasources file lists the Prometheus and ClickHouse servers SlashObserve queries.
It is a single YAML document with a top-level `datasources` list; unknown fields are rejected, and an empty list logs a warning at startup.
The server reads it once at startup.

The file never holds a secret.
Passwords and tokens are referenced by environment variable name or by file path instead, so the file can live in version control.

```yaml
datasources:
  - uid: prom
    type: prometheus
    name: Prometheus
    prometheus:
      url: http://prometheus:9090
      scrapeInterval: 15s
    limits:
      timeout: 30s
      maxSeries: 1000
  - uid: ch
    type: clickhouse
    name: ClickHouse
    clickhouse:
      addr: clickhouse:9000
      database: otel
      username: slashobserve_ro
      passwordEnv: SLASHOBSERVE_CLICKHOUSE_PASSWORD
```

`deploy/compose/datasources.yaml` is the minimal exemplar used by the demo.

Fields common to every entry:

| Field         | Notes                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `uid`         | Required, unique. Up to 40 letters, digits, `_` or `-`, starting with a letter or digit. Dashboards refer to a datasource by it. Quote a value that looks like a number, such as `"0123"`, because an unquoted number is refused.                                                                                                                                                                                                    |
| `type`        | Required: `prometheus` or `clickhouse`. Give only the block that matches (`prometheus:` or `clickhouse:`).                                                                                                                                                                                                                                                                                                                           |
| `name`        | A display name. Quote a value that looks like a number, such as `"0123"`, because an unquoted number is refused.                                                                                                                                                                                                                                                                                                                     |
| `limits`      | Optional: `timeout` (a Go duration), `maxSeries`, `maxPoints` and `maxRows` (whole numbers, not negative). See below.                                                                                                                                                                                                                                                                                                                |
| `grafanaUids` | Optional: up to 20 Grafana datasource uids or names this datasource stands for when importing Grafana dashboards, each 1 to 100 bytes after trimming and listed by one datasource only. Matching is exact and case-sensitive. Entries are trimmed, control and invisible characters are refused, an entry may not be another datasource's `uid`, and a value that looks like a number, such as `"0123"` or `"1.50"`, must be quoted. |

The `uid`, `name` and `grafanaUids` values must not carry a YAML tag such as `!!str`; quote the value instead.

The Grafana importer matches a dashboard's datasources by the mapping given with the import, then by a Grafana datasource uid that equals one of your datasource uids (so a dashboard exported from a Grafana that already uses your uids needs no `grafanaUids`), then by `grafanaUids`, then by being the only datasource of that type; it never guesses between two datasources of the same type.

The fields of each block are described under [Prometheus](../connect/prometheus.md) and [ClickHouse](../connect/clickhouse.md).

Rules for secrets:

- An environment variable name must start with `SLASHOBSERVE_`, must be set and non-empty, and must not be one of the server's own settings (such as `SLASHOBSERVE_DATABASE_URL`).
- A file's content is used with trailing newlines removed; an empty file is an error.
- Set only one of the env and file forms for the same secret.
- A header name may appear in only one of `headers`, `headersFromEnv` and `headersFromFile`.
  Prometheus rejects an `Authorization` header there; use `basicAuth` or a bearer token instead.

Limits bound every query the server runs for that datasource.
The defaults are a 30 second timeout, 1000 series, 1,000,000 points and 10,000 rows.
A value you set replaces the default, up or down; a ClickHouse settings profile can still cap it further.
The agent's and MCP's ad-hoc queries use tighter limits still (15 seconds, 200 series, 100,000 points and 1,000 rows), or the datasource's limits when those are lower.
For ClickHouse table results (the `table` and `logs` formats, and instant queries) each row counts as one point, so the row limit bounds them.
