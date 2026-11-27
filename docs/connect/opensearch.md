---
title: Connect OpenSearch
description: Add an OpenSearch cluster as a datasource for logs, or for the spans Jaeger stored there, with its index pattern, time field, field roles and authentication, a first query, and the rules for queries, aggregations, filters and paging.
---

Connect an OpenSearch cluster to search its logs on the Logs page, chart document counts and aggregations in dashboards, and let the agent read them.
SlashObserve supports OpenSearch 2.x and 3.x.
For an Elasticsearch cluster use the [Elasticsearch](elasticsearch.md) type instead, and the health check says so when the two are mixed up.
The two types share one adapter, so this page and the Elasticsearch page describe the same fields, roles and discovery.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect an OpenSearch cluster

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names.
   `index` is required and names the logs to search:

   ```yaml
   datasources:
     - uid: os
       type: opensearch
       name: OpenSearch
       opensearch:
         url: http://opensearch:9200
         index: logs-*
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck os
   ```

   The [health check](#health) tells a wrong type, an unsupported version, an empty index pattern and a missing field apart.
4. Run a first query:

   ```sh
   observe logs 'error' --datasource os --from now-1h
   ```

   You see up to 100 log lines that contain "error", oldest first, with their time, level, service and message.
   The same query works on the [Logs page](../use/logs-and-traces.md).

## Connect a cluster behind a login

Reference the password by variable name or by file, never by value.
The variable name must start with `SLASHOBSERVE_`, and it must not be one the server uses for its own configuration.
It goes in the same env file as the server's other settings.

```yaml
- uid: os
  type: opensearch
  name: OpenSearch
  opensearch:
    url: https://opensearch.example.com:9200
    index: logs-*
    basicAuth:
      username: reader
      passwordEnv: SLASHOBSERVE_OS_PASSWORD
```

Basic auth, a bearer token and an API key cannot be combined.
For an API key, give `apiKeyEnv` or `apiKeyFile`, and the key is sent as `Authorization: ApiKey`.

Amazon OpenSearch Service domains that require AWS Signature Version 4 are not supported yet.
Put a proxy that signs requests in front of the domain, or use a domain with basic auth through fine-grained access control.

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

## Read spans that Jaeger stored

Jaeger can store its spans in OpenSearch.
Set `spans: jaeger` on an entry whose index holds those span documents, and the datasource answers the trace reads as a Jaeger or Tempo datasource does.
Use it when the Jaeger in front of the cluster is older than the 2.21 the [Jaeger](jaeger.md) type needs, or when only the cluster is reachable:

```yaml
- uid: traces
  type: opensearch
  name: Jaeger spans
  opensearch:
    url: http://opensearch:9200
    index: jaeger-main-jaeger-span-*
    spans: jaeger
```

The server never guesses that an index holds spans; only this setting says so.
[Jaeger spans](#jaeger-spans) lists what such a datasource reads and what it cannot do.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. |
| `index` | Required. One or more comma-separated index names, patterns, aliases or data streams. |
| `timeField` | The `date` or `date_nanos` field that orders documents. Default `@timestamp`. |
| `queryLanguage` | `kql` (default) or `lucene`. |
| `spans` | `jaeger` when the index holds span documents that Jaeger wrote. |
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

### Jaeger spans

An entry with `spans: jaeger` reads span documents in the shape Jaeger writes to OpenSearch and Elasticsearch.
The shape was checked against documents a Jaeger 2.21 wrote to OpenSearch 2.19, and the tests index those documents with the mapping Jaeger created.

- The time field is `startTimeMillis`, and a `timeField` other than that is refused.
- A span's parent is `parentSpanID`, or else the `CHILD_OF` reference in the same trace; other references are links.
- Tags are read from both forms Jaeger writes: the nested `tags` array and the `tag` object of tags stored as fields, whose keys carry `@` for each dot.
  Process tags are read the same way and become the span's resource attributes, with `service.name` from `process.serviceName`.
- A span failed when its `error` tag is true or its `otel.status_code` is `ERROR`; `otel.status_description` is its status message and `span.kind` its kind.
- `startTime` and `duration` are microseconds, so durations are exact to the microsecond.
- A trace id is matched as given, without its leading zeros and, when its high half is zero, as 16 characters, the forms older Jaeger versions stored.

What it answers:

- A whole trace by id, read in pages of 5,000 spans, up to 20,000 spans with the earliest kept, as `GET /api/v1/datasources/{uid}/traces/{trace_id}` and the `get_trace` tool describe.
- The services list, from `process.serviceName`, as the `traces` signal.
- A listing of the traces in a window, newest first, by service, operation, root span name, failure and least duration, which `trace_error_origins` and `trace_slow_path` read before they sample traces.
  A trace matches when each condition holds in some span of it, and its duration is its root span's, or its longest span's when no root is stored.
  With more than one condition the newest 8,000 traces that meet any of them are weighed, so a listing past that says it is not complete.
- Span counts, failures and a duration histogram per time bucket, as `POST /api/v1/datasources/{uid}/trace-stats` and the `trace_stats` tool describe.
  The filter is a query over the span documents in the datasource's query language, such as `process.serviceName:checkout`, and a Query DSL body may hold only `query`.
  KQL and Lucene reach the top-level fields and the `tag` object; a tag in the nested `tags` array takes a Query DSL body with a `nested` query on `tags`, matching `tags.key` and `tags.value`.
  Per-bucket quantiles are the cluster's `percentiles` estimates (t-digest), so they are approximate.
  The p50, p95 and p99 over the whole window and per group are exact: the server reads up to 20,000 matching span durations, newest first, and says they are a sample when the window holds more.
  `group_by` takes `service`, `name`, `status`, `kind`, `statusMessage`, `span.<tag>` and `resource.<tag>`.
- The attribute pairs of a set of traces, read 4 traces at a time, as `compare_traces` describes.
- The log lines of a trace are not in a span index: read them from a logs datasource.

`list_datasources` and `GET /api/v1/datasources` show such a datasource with `spans: jaeger`, `GET /api/v1/datasources` also shows it without `trace_search`, and the agent's guide tells it to use the trace tools on it rather than write Query DSL by hand.
The Traces page, the trace search and `observe traces` do not offer it, because they read traces through the explore query, which a span index does not answer yet.

### Health

The health check does these steps in order:

- Reads the server's version and confirms that it is OpenSearch 2.x or 3.x.
- Confirms that the index pattern matches at least one index, alias or data stream.
- Reads the field mapping.
- Checks that the time field is a date and that every field named under `fields` exists.

Each failure has its own message on the datasources page:

| Failure | Message |
| --- | --- |
| The server is Elasticsearch | The server is Elasticsearch, configure it as type elasticsearch |
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

These cover OpenSearch's Simple Schema for Observability (`body`, `severity.text`, `traceId`), ECS documents from Data Prepper and Fluent Bit, and the OpenTelemetry Collector's OpenSearch exporter.
An OpenSearch `flat_object` field is reported with the kind `flattened`.

### Variables and discovery

A query variable on an OpenSearch datasource takes `label_values(field)` or `label_values(<query>, field)`.
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
OpenSearch has no metric names, so the metric list is empty.
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
`meta.rows_total` is the number of matching documents when OpenSearch counted them exactly, which it does up to 10,000, and is left out beyond that.
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

Every ad hoc variable on an OpenSearch datasource applies to its queries.
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
