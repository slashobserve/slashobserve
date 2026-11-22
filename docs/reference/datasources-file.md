---
title: Datasources file
description: The YAML file that lists the Prometheus, ClickHouse, Loki, Tempo, Elasticsearch and OpenSearch servers, and its rules for secrets and limits.
---

The datasources file lists the Prometheus, ClickHouse, Loki, Tempo, Elasticsearch and OpenSearch servers SlashObserve queries.
`SLASHOBSERVE_DATASOURCES_FILE` names its path; with it unset the server has no file datasources.
It is a single YAML document with a top-level `datasources` list; unknown fields are rejected, a missing `datasources` key or a second YAML document stops the server at startup, and an empty list logs a warning.
The server reads it once at startup, and its entries show as "From file", read-only, on the Datasources page.
An Admin can add more datasources in the app; see [Add datasources in the app](../connect/datasources-in-the-app.md).

The file never holds a secret.
Passwords and tokens are referenced by environment variable name or by file path instead, so the file can live in version control.

## Example

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
  - uid: loki
    type: loki
    name: Loki
    loki:
      url: http://loki:3100
      tenantId: shop
  - uid: tempo
    type: tempo
    name: Tempo
    tempo:
      url: http://tempo:3200
  - uid: es
    type: elasticsearch
    name: Elasticsearch
    elasticsearch:
      url: http://elasticsearch:9200
      index: logs-*
```

`deploy/compose/datasources.yaml` is the minimal exemplar used by the demo.

## Who can query a datasource

Every entry is open to every signed-in person from Viewer up and to every service-account key, through the explore query and the panel editor, under the entry's limits, unless it sets `audience: admins`.
An `admins` entry is read only by Admins and Admin keys: every other role, key and public link gets the same answer as for a datasource that does not exist, the agent and MCP clients of other roles do not see it, and a dashboard version that queries it serves its panel data and variable values only to Admins.
There is no per-team or per-folder datasource access; a restricted folder hides its dashboards, not the datasources they read.

A dashboard, proposal, observation or saved search that reads an `admins` entry lives only in a [folder only Admins can read](../teams/folders.md), as for a [datasource added in the app](../connect/datasources-in-the-app.md#who-can-query-it).
The server reads the file only when it starts, and it refuses to start while content in any other folder reads an entry the file marks `admins`.
The startup error `datasource_audience_refused` names the datasource and how many items read it, lists up to 20 of the ones outside people's My Dashboards with their kind, id, title and folder, counts the rest, and says the two ways out: move them into a folder only Admins can read while the entry is still `everyone`, or keep `audience: everyone` in the file.
Items in someone's My Dashboards are only counted, never named, because nobody but their owner may see them.

To make an entry `admins`:

1. While the entry is still `everyone`, make or pick a folder only Admins can read and move every dashboard, observation and saved search that reads the datasource into it; content in someone's My Dashboards can be moved only by its owner.
2. Set `audience: admins` on the entry in the file on every server.
3. Restart the servers. Each one checks at start, so a server that refuses names what is still outside; move it and restart that server again.

## Fields common to every entry

| Field         | Notes                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `uid`         | Required, unique. Up to 40 letters, digits, `_` or `-`, starting with a letter or digit. Dashboards refer to a datasource by it. Quote a value that looks like a number, such as `"0123"`, because an unquoted number is refused.                                                                                                                                                                                                    |
| `type`        | Required: `prometheus`, `clickhouse`, `loki`, `tempo`, `elasticsearch` or `opensearch`. Give only the block that matches (`prometheus:`, `clickhouse:`, `loki:`, `tempo:`, `elasticsearch:` or `opensearch:`).                                                                                                                                                                                                                      |
| `name`        | A display name. Quote a value that looks like a number, such as `"0123"`, because an unquoted number is refused.                                                                                                                                                                                                                                                                                                                     |
| `limits`      | Optional: `timeout` (a Go duration), `maxSeries`, `maxPoints`, `maxRows` and `maxConcurrentQueries` (whole numbers, not negative). An omitted value or `0` keeps the default. See below.                                                                                                                                                                                                                                                                                                                |
| `audience`    | Optional: `everyone` (the default) or `admins`. See [Who can query a datasource](#who-can-query-a-datasource). |
| `grafanaUids` | Optional: up to 20 Grafana datasource uids or names this datasource stands for when importing Grafana dashboards, each 1 to 100 bytes after trimming and listed by one datasource only. Matching is exact and case-sensitive. Entries are trimmed, control and invisible characters are refused, an entry may not be another datasource's `uid`, and a value that looks like a number, such as `"0123"` or `"1.50"`, must be quoted. |

The `uid`, `name`, `audience` and `grafanaUids` values must not carry a YAML tag such as `!!str`; quote the value instead.

The Grafana importer matches a dashboard's datasources by the mapping given with the import, then by a Grafana datasource uid that equals one of your datasource uids (so a dashboard exported from a Grafana that already uses your uids needs no `grafanaUids`), then by `grafanaUids`, then by being the only datasource of that type; it never guesses between two datasources of the same type.

The fields of each block are described under [Prometheus](../connect/prometheus.md), [ClickHouse](../connect/clickhouse.md), [Loki](../connect/loki.md), [Tempo](../connect/tempo.md), [Elasticsearch](../connect/elasticsearch.md) and [OpenSearch](../connect/opensearch.md).

## Rules for secrets

- An environment variable name must start with `SLASHOBSERVE_`, must be set and non-empty, and must not be one of the server's own settings (such as `SLASHOBSERVE_DATABASE_URL`).
- A file's content is used with trailing newlines removed; an empty file is an error.
- Set only one of the env and file forms for the same secret.
- A header name may appear in only one of `headers`, `headersFromEnv` and `headersFromFile`.
  Prometheus, Loki, Tempo, Elasticsearch and OpenSearch reject an `Authorization` header there; use `basicAuth`, a bearer token or, on Elasticsearch and OpenSearch, an API key instead.

## Limits

Limits bound every query the server runs for that datasource.
The defaults are a 30 second timeout, 1000 series, 1,000,000 points and 10,000 rows.
A value you set replaces the default, up or down; a ClickHouse settings profile can still cap it further.

`maxConcurrentQueries` caps the queries running against that datasource at once across every server that shares the database: 32 by default, at most 1024.
Each server allows its share, the total divided by the live servers and rounded up, and at most 32 datasource calls at once across all datasources.
A panel waits for a slot until its deadline; the logs page, traces, label lists and the agent's tools wait up to 5 seconds and then answer 429 `too_many_requests`.

The agent's and MCP's ad-hoc queries, the draft preview in the panel editor on dashboards and observations, the logs page, the trace view and `POST /api/v1/datasources/{uid}/query` use tighter limits still (15 seconds, 200 series, 100,000 points and 1,000 rows), or the datasource's limits when those are lower.

For ClickHouse table results (the `table` and `logs` formats, and instant queries) each row counts as one point, so the row limit bounds them.
A Loki log query reads at most the row limit or 5,000 lines, and each line counts as one point; a Tempo query returns at most the row limit of spans or traces.
Separate from these limits, the server refuses a Prometheus, Loki, Tempo, Elasticsearch or OpenSearch response over 64 MiB and a ClickHouse result over 32 MiB.
