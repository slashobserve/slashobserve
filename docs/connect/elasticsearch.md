---
title: Connect Elasticsearch
description: Add an Elasticsearch cluster as a datasource for logs, with its index pattern, time field, field roles and authentication, a first query, and the rules for queries, aggregations, filters and paging.
---

Connect an Elasticsearch cluster to search its logs on the Logs page, chart document counts and aggregations in dashboards, and let the agent read them.
SlashObserve supports Elasticsearch 8.x and 9.x.
For an OpenSearch cluster use the [OpenSearch](opensearch.md) type instead, and the health check says so when the two are mixed up.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect an Elasticsearch cluster

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names.
   `index` is required and names the logs to search:

   ```yaml
   datasources:
     - uid: es
       type: elasticsearch
       name: Elasticsearch
       elasticsearch:
         url: http://elasticsearch:9200
         index: logs-*
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck es
   ```

   The [health check](#health) tells a wrong type, an unsupported version, an empty index pattern and a missing field apart.
4. Run a first query:

   ```sh
   observe logs 'error' --datasource es --from now-1h
   ```

   You see up to 100 log lines that contain "error", oldest first, with their time, level, service and message.
   The same query works on the [Logs page](../use/logs-and-traces.md).

## Connect a cluster behind an API key

Reference the key by variable name or by file, never by value.
The variable name must start with `SLASHOBSERVE_`, and it must not be one the server uses for its own configuration.
It goes in the same env file as the server's other settings.

```yaml
- uid: es
  type: elasticsearch
  name: Elasticsearch
  elasticsearch:
    url: https://elasticsearch.example.com:9200
    index: logs-*
    apiKeyEnv: SLASHOBSERVE_ES_API_KEY
```

The key is sent as `Authorization: ApiKey`.
An API key cannot be combined with `basicAuth` or a bearer token.
For basic auth, give `basicAuth` with a `username` and one of `passwordEnv` or `passwordFile`.

## Search with KQL, Lucene or a Query DSL body

A query takes one of three forms, chosen by its first characters.
KQL is the default.
A `lucene:` prefix chooses Lucene, and text that starts with `{` is a Query DSL body.
[Queries](#queries) describes each form, what it returns and the variables and macros it takes.

## Chart aggregations

A Query DSL body with `aggs` folds into series or a table, so a dashboard can chart counts, averages and percentiles.
[Aggregations](#aggregations) lists what is supported.

## Pick the field that plays each role

The server finds the message, level, service, trace id and span id fields by name.
Set `fields` when your documents use other names.
[Field roles](#field-roles) lists the names it tries.

## Find fields, values and services

A query variable takes `label_values(field)` or `label_values(<query>, field)`.
[Variables and discovery](#variables-and-discovery) lists what the agent and the field browser can read.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. |
| `index` | Required. One or more comma-separated index names, patterns, aliases or data streams. |
| `timeField` | The `date` or `date_nanos` field that orders documents. Default `@timestamp`. |
| `queryLanguage` | `kql` (default) or `lucene`. |
| `fields` | Role overrides: `message`, `level`, `service`, `traceId` and `spanId`. |
| `apiKeyEnv`, `apiKeyFile` | An API key from an environment variable or a file. |
| `headers` | Plain request headers. Not for secrets. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path. |
| `basicAuth` | `username` plus one of `passwordEnv` or `passwordFile`. The password is required. |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. |

Details of `index`:

- Examples are `logs-*` and `logs-shop,logs-web-*`.
- It must be narrower than `_all` and must not start with `-` or `_`.

Details of `queryLanguage`:

- It is the language of a query that carries no `kql:` or `lucene:` prefix.

Details of `fields`:

- Each value is a field name that must exist in the mapping, or the datasource is unhealthy.
- See [Field roles](#field-roles).

Details of `apiKeyEnv` and `apiKeyFile`:

- The key is sent as `Authorization: ApiKey`.
- It cannot be combined with `basicAuth` or a bearer token.

Details of `headers`:

- `Accept`, `Content-Type`, `Host` and `Content-Length` are managed by the server and refused.
- `Authorization` is refused; use `basicAuth`, a bearer token or an API key.

Details of `bearerTokenEnv` and `bearerTokenFile`:

- They cannot be combined with `basicAuth` or an API key.

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

### Health

The health check does these steps in order:

- Reads the server's version and confirms that it is Elasticsearch 8.x or 9.x.
- Confirms that the index pattern matches at least one index, alias or data stream.
- Reads the field mapping.
- Checks that the time field is a date and that every field named under `fields` exists.

Each failure has its own message on the datasources page:

| Failure | Message |
| --- | --- |
| The server is OpenSearch | The server is OpenSearch, configure it as type opensearch |
| An unsupported version | The datasource version is not supported: Elasticsearch 8.x or 9.x, OpenSearch 2.x or 3.x |
| An empty pattern | The index pattern matches no index |
| A missing time field | The time field is missing or is not a date |
| A missing role field | A configured field is missing from the mapping |

### Field roles

The server reads the mapping of the index pattern through the field capabilities API, limited to indices with documents in the last 7 days, and keeps it for a minute.
From it, five roles are resolved by the first field present in the mapping, unless `fields` names one:

| Role | Candidates, in order |
| --- | --- |
| `message` | `message`, `body`, `body.text`, `log`, `msg`, `event.original` |
| `level` | `log.level`, `severity.text`, `severity_text`, `level`, `severity`, `loglevel` |
| `service` | `service.name`, `resource.service.name`, `resource.attributes.service.name`, `resource.attributes.service_name`, `service_name`, `app` |
| `traceId` | `trace.id`, `traceId`, `trace_id`, `traceID` |
| `spanId` | `span.id`, `spanId`, `span_id`, `spanID` |

These cover ECS documents from Elastic Agent and Filebeat, the OpenTelemetry Collector's Elasticsearch exporter in both ECS and OTel modes, and OpenSearch's Simple Schema for Observability.

### Variables and discovery

A query variable on an Elasticsearch datasource takes `label_values(field)` or `label_values(<query>, field)`.
The query can be in any of the three forms and narrows the values.
The variable returns the field's values over the dashboard's time range, up to 10,000 of them, through a terms aggregation.
A `text` field is read through its `.keyword` subfield, and a `text` field without one has no listable values.

The agent and the field browser can list these:

- The aggregatable fields, plus the `text` fields that have a `.keyword` subfield.
- The values of one field over a time range.
- Every mapped field with its kind: `keyword`, `text`, `number`, `date`, `date_nanos`, `boolean`, `ip`, `nested`, `flattened`, `object` or `other`.
- The indices, aliases and data streams the pattern matches.
- Up to 20 of the newest documents, with the resolved roles as columns.

A field mapped differently across indices keeps the first type the server lists.
Elasticsearch has no metric names, so the metric list is empty.
The services list is the values of the `service` role field over a recent window.
The logs page shows it for each source, and `observe datasources services` and the `list_services` tool print it.
It is empty when no field plays that role.
The trace id and span id roles are what `observe datasources trace-logs` and the `logs_for_trace` tool match to read the log lines of a trace.

### Queries

A query takes one of three forms, chosen by its first characters:

| Form | How it is chosen | What it is |
| --- | --- | --- |
| KQL | The default, or a `kql:` prefix. | The Kibana query language, compiled on the server to Query DSL. |
| Lucene | A `lucene:` prefix, or `queryLanguage: lucene`. | The Lucene query string syntax, sent as a `query_string` query with `analyze_wildcard`. |
| Query DSL | Text that starts with `{`. | A JSON search body. |

Prefixes are case-insensitive, and an empty query matches every document.
[What KQL leaves out](../use/logs-and-traces.md#query-languages-on-elasticsearch-and-opensearch) lists the KQL features the compiler does not take.
A KQL query that finds nothing can come back with `meta.hint`, which says when a field it filters on holds the same value in other letters, as that section describes.
The time range and any ad hoc filters wrap your query in a `bool` filter, in every form.

Details of a Query DSL body:

- Only `query`, `aggs`, `aggregations`, `size`, `sort`, `fields`, `_source` and `runtime_mappings` are accepted.
  Any other key is refused, so the body cannot widen the time range or the limits.
- A script anywhere in the body is refused: `script`, `script_fields`, `script_score`, `scripted_metric` and the other script keys, including one inside `runtime_mappings`.
- A field, a sort or an aggregation that is only named `script` is accepted.
- So is the `script` of a `bucket_script` or `moving_fn` aggregation, which sees only the values of its buckets.

What a query returns depends on its kind and form:

- An instant query, such as the logs explorer's, returns log lines newest first as one frame.
  Its columns are `Time`, `TimeNs`, `Body`, `level`, `ID`, `Index`, `service`, `trace_id`, `span_id` and `Fields`.
  A role column is left out when no field plays that role, and `Fields` holds the remaining fields of the document as JSON.
  `TimeNs` is exact on a `date_nanos` time field, and on a `date` field it is the milliseconds the index stores, times one million.
- A range query in KQL or Lucene counts documents over time: one series named `count`, with one point per step.
  On a dashboard whose `timezone` names a zone the buckets follow that zone, so a daily step starts at local midnight.
- A Query DSL body with `aggs` runs with `size` forced to 0, and its result is folded into series or a table, whatever the kind.
  A body without `aggs` returns lines like an instant query, with `size` capped at the line limit.

Lines are capped at 5,000 and at the query's row limit, whichever is lower, and `meta.truncated` says the cap was reached.
`meta.rows_total` is the number of matching documents when Elasticsearch counted them exactly, which it does up to 10,000, and is left out beyond that.
`meta.partial` says the cluster timed out or a shard failed.
A range query is refused when its step would make more than 11,000 buckets or more points than the point limit.

`$__from` and `$__to` expand to epoch milliseconds, `$__interval` to the step in seconds such as `300s`, and `$__range` as on Loki.
Inside a quoted string, such as a Query DSL value, a variable is escaped for JSON, and a variable with several values needs a format such as `${var:pipe}`.
Outside quotes, a variable with several values expands to a parenthesised list of quoted values joined with `OR`, which KQL and Lucene both read as an operator.

### Aggregations

The server folds the result of `aggs` by walking it beside the request.
It supports these aggregations:

- Bucket aggregations: `date_histogram`, `auto_date_histogram`, `histogram`, `terms`, `multi_terms`, `filters`, `range`, `date_range` and `significant_terms`.
  A `histogram` is a time axis when its field is the time field, and otherwise numeric buckets in a table.
- Metric aggregations: `avg`, `sum`, `min`, `max`, `value_count`, `cardinality`, `median_absolute_deviation`, `percentiles`, `stats`, `extended_stats`, `rate`, `bucket_script`, `derivative`, `cumulative_sum` and `moving_fn`.

The shape of the result depends on the buckets:

- When a time bucket aggregation appears anywhere in the nesting, the result is series.
  There is one series per metric and per combination of the other buckets' keys, labelled with the bucket field or the aggregation's name.
  A bucket with no metric under it gives a series named `count`, and percentiles add a `percentile` label.
- With no time bucket, the result is one table.
  It has a column per bucket level, then `count`, then a column per metric, and a row per innermost bucket, capped at the row limit.

These are refused:

- More than one bucket aggregation at a level, and a time bucket inside another.
- Any other aggregation, such as `top_hits` or `composite`, with `unsupported aggregation "x" (type y)`.
- More series or points than the limits, with a limit error.

### Ad hoc filters

Every ad hoc variable on an Elasticsearch datasource applies to its queries.
Each operator becomes a clause:

| Operator | Clause |
| --- | --- |
| `=` and `!=` | A `term`, or a `match_phrase` on a text field with no `.keyword` subfield. |
| `=~` and `!~` | A case-insensitive `regexp`. |
| `<` and `>` | A `range`. |

A text field with a `.keyword` subfield is filtered through the subfield.
Filters joined with OR become a `should` group.

### Lint

The lint endpoint checks a query without running a search.
KQL is compiled on the server and reports syntax errors and unknown fields (a warning).
Lucene and the `query` part of a Query DSL body are checked with the cluster's validate API.
A Query DSL body is also checked for JSON syntax and for keys that are not accepted.
Each diagnostic carries the offsets of the text it concerns.

### Paging

A query sent with `page_size` pages by offset.
Any page can be asked for directly, the total is exact and no `next` cursor is returned.
The last page that can be reached is set by the cluster's `index.max_result_window`, which the server reads with the mapping: 10,000 unless an index sets it lower.
A page past the window is refused with "pages past 10,000 results are not available on this index; narrow the time range".
The last reachable page already says it has no next page.
A Query DSL body with `aggs` cannot be paged.
