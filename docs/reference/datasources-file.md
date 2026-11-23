---
title: Datasources file
description: The YAML file that lists the Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch servers, and its rules for secrets and limits.
---

The datasources file lists the Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch servers SlashObserve queries.
Use it when you want datasources kept in version control and the same on every server.
An Admin can also add datasources in the app; see [Add datasources in the app](../connect/datasources-in-the-app.md).

The file never holds a secret.
Passwords and tokens are referenced by environment variable name or by file path instead, so the file can live in version control.

## Write a datasources file

1. Write a single YAML document with a top-level `datasources` list.
   Each entry has a `uid`, a `type` and a block named after the type:

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
     - uid: jaeger
       type: jaeger
       name: Jaeger
       jaeger:
         url: http://jaeger:16686
     - uid: es
       type: elasticsearch
       name: Elasticsearch
       elasticsearch:
         url: http://elasticsearch:9200
         index: logs-*
   ```

   `deploy/astronomy/datasources.yaml` is the exemplar the [demo](../operate/demo.md) uses.
2. Set `SLASHOBSERVE_DATASOURCES_FILE` to the file's path on every server, and restart the servers.
   With it unset, the server has no file datasources.
3. Open the Datasources page.
   Each entry shows as "From file", read-only.

The server reads the file once at startup, so a change needs a restart.
It stops at startup when the YAML does not parse, when `datasources` is missing, when the file holds a second YAML document, or when any entry breaks a rule on this page.
Unknown fields are rejected, and a duplicate `uid` is refused.
An empty list logs the warning `datasources file lists no datasources`.

## Keep a password out of the file

Name the environment variable that holds the secret, or a file that holds it.
This ClickHouse entry reads its password from the environment of the server:

```yaml
clickhouse:
  addr: clickhouse:9000
  username: slashobserve_ro
  passwordEnv: SLASHOBSERVE_CLICKHOUSE_PASSWORD
```

Use `passwordFile: /run/secrets/clickhouse` to read a file instead.
[Rules for secrets](#rules-for-secrets) lists what each form accepts.

## Set a query limit

Add a `limits` block to cap what one query may take, or how many may run at once:

```yaml
limits:
  timeout: 20s
  maxSeries: 500
  maxConcurrentQueries: 64
```

An omitted value or `0` keeps the default.
[Limits](#limits) says what each one bounds.

## Who can query a datasource

Every entry is open to every signed-in person from Viewer up and to every service-account key.
They reach it through the explore query and the panel editor, under the entry's limits.
Set `audience: admins` on an entry to restrict it.

An `admins` entry is read only by Admins and Admin keys:

- Every other role, key and public link gets the same answer as for a datasource that does not exist.
- The agent and MCP clients of other roles do not see it.
- A dashboard version that queries it serves its panel data and variable values only to Admins.

There is no per-team or per-folder datasource access.
A restricted folder hides its dashboards, not the datasources they read.

A dashboard, proposal, observation or saved search that reads an `admins` entry lives only in a [folder only Admins can read](../teams/folders.md), as for a [datasource added in the app](../connect/datasources-in-the-app.md#who-can-query-it).
The server reads the file only when it starts, and it refuses to start while content in any other folder reads an entry the file marks `admins`.

To make an entry `admins`:

1. While the entry is still `everyone`, make or pick a folder only Admins can read.
   Move every dashboard, observation and saved search that reads the datasource into it.
   Content in someone's My Dashboards can be moved only by its owner.
2. Set `audience: admins` on the entry in the file on every server.
3. Restart the servers.
   Each one checks at start, so a server that refuses names what is still outside.
   Move it and restart that server again.

The startup error `datasource_audience_refused` names the datasource and how many items read it.
It lists up to 20 of the items outside people's My Dashboards with their kind, id, title and folder, and counts the rest.
It says the two ways out: move them into a folder only Admins can read while the entry is still `everyone`, or keep `audience: everyone` in the file.
Items in someone's My Dashboards are only counted, never named, because nobody but their owner may see them.

## Map Grafana datasources for imports

Add `grafanaUids` to an entry to say which Grafana datasources it stands for when you import Grafana dashboards.
The Grafana importer matches a dashboard's datasources in this order:

1. The mapping given with the import.
2. A Grafana datasource uid that equals one of your datasource uids, so a dashboard exported from a Grafana that already uses your uids needs no `grafanaUids`.
3. `grafanaUids`.
4. Being the only datasource of that type.

It never guesses between two datasources of the same type.

## Reference

### Fields common to every entry

| Field | Notes |
| --- | --- |
| `uid` | Required, unique. Dashboards refer to a datasource by it. |
| `type` | Required: `prometheus`, `clickhouse`, `loki`, `tempo`, `jaeger`, `elasticsearch` or `opensearch`. |
| `name` | Optional display name. It defaults to the `uid`. |
| `limits` | Optional: `timeout`, `maxSeries`, `maxPoints`, `maxRows` and `maxConcurrentQueries`. |
| `audience` | Optional: `everyone` (the default) or `admins`. |
| `grafanaUids` | Optional: Grafana datasource uids or names this datasource stands for. |

The block named after the type holds the connection.
Give only the block that matches (`prometheus:`, `clickhouse:`, `loki:`, `tempo:`, `jaeger:`, `elasticsearch:` or `opensearch:`).
The fields of each block are described under [Prometheus](../connect/prometheus.md), [ClickHouse](../connect/clickhouse.md), [Loki](../connect/loki.md), [Tempo](../connect/tempo.md), [Jaeger](../connect/jaeger.md), [Elasticsearch](../connect/elasticsearch.md) and [OpenSearch](../connect/opensearch.md).

Details of the common fields:

- `uid` has up to 40 letters, digits, `_` or `-`, and starts with a letter or digit.
- `uid` and `name` must not carry a YAML tag such as `!!str`; quote the value instead.
  Quote a value that looks like a number, such as `"0123"`, because an unquoted number is refused.
- `limits` values are whole numbers and not negative, except `timeout`, which is a Go duration such as `30s`.
- `grafanaUids` holds up to 20 entries, each 1 to 100 bytes after trimming, and each listed by one datasource only.
  Matching is exact and case-sensitive.
  Entries are trimmed, and control and invisible characters are refused.
  An entry may not be another datasource's `uid`.
  A value that looks like a number, such as `"0123"` or `"1.50"`, must be quoted.
  The `grafanaUids` and `audience` values must not carry a YAML tag either.

### Rules for secrets

- An environment variable name must start with `SLASHOBSERVE_`, must be set and non-empty, and must not be one of the server's own settings (such as `SLASHOBSERVE_DATABASE_URL`).
- A file's content is used with trailing newlines removed, and an empty file is an error.
- Set only one of the env and file forms for the same secret.
- A header name may appear in only one of `headers`, `headersFromEnv` and `headersFromFile`.
  Prometheus, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch reject an `Authorization` header there.
  Use `basicAuth`, a bearer token or, on Elasticsearch and OpenSearch, an API key instead.

### Limits

Limits bound every query the server runs for that datasource.
The defaults are a 30 second timeout, 1000 series, 1,000,000 points and 10,000 rows.
A value you set replaces the default, up or down.
A ClickHouse settings profile can still cap it further.

`maxConcurrentQueries` caps the queries running against that datasource at once across every server that shares the database.
It is 32 by default and at most 1024.
Each server allows its share, the total divided by the live servers and rounded up, and at most 32 datasource calls at once across all datasources.
A panel waits for a slot until its deadline.
The logs page, traces, label lists and the agent's tools wait up to 5 seconds and then answer 429 `too_many_requests`.

The agent's and MCP's ad-hoc queries, the draft preview in the panel editor on dashboards and observations, the logs page, the trace view and `POST /api/v1/datasources/{uid}/query` use tighter limits still.
Those are 15 seconds, 200 series, 100,000 points and 1,000 rows, or the datasource's limits when those are lower.

How a result counts against the limits:

- For ClickHouse table results (the `table` and `logs` formats, and instant queries) each row counts as one point, so the row limit bounds them.
- A Loki log query reads at most the row limit or 5,000 lines, and each line counts as one point.
- A Tempo or Jaeger query returns at most the row limit of spans or traces.

Separate from these limits, the server refuses a Prometheus, Loki, Tempo, Jaeger, Elasticsearch or OpenSearch response over 64 MiB and a ClickHouse result over 32 MiB.
