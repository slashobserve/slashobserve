---
title: Connect Elasticsearch
description: Add an Elasticsearch cluster as a datasource for logs, with its index pattern, time field, field roles and authentication.
---

An Elasticsearch entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.
Elasticsearch 8.x and 9.x are supported; for an OpenSearch cluster use the [OpenSearch](opensearch.md) type instead, and the health check says so when the two are mixed up.

A cluster behind an API key names the index pattern and references the key by variable name:

```yaml
- uid: es
  type: elasticsearch
  name: Elasticsearch
  elasticsearch:
    url: https://elasticsearch.example.com:9200
    index: logs-*
    apiKeyEnv: SLASHOBSERVE_ES_API_KEY
```

## Fields

| Field                               | Notes                                                                                                                                                                                                                   |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                                                                                                                                          |
| `index`                             | Required. One or more comma-separated index names, index patterns, aliases or data streams, such as `logs-*` or `logs-shop,logs-web-*`. It must be narrower than `_all` and must not start with `-` or `_`.             |
| `timeField`                         | The `date` or `date_nanos` field that orders documents. Default `@timestamp`.                                                                                                                                           |
| `queryLanguage`                     | `kql` (default) or `lucene`: the language of a query that carries no `kql:` or `lucene:` prefix.                                                                                                                        |
| `fields`                            | Role overrides, each a field name: `message`, `level`, `service`, `traceId` and `spanId`. See below.                                                                                                                    |
| `apiKeyEnv`, `apiKeyFile`           | An API key from an environment variable or a file, sent as `Authorization: ApiKey`. Not combinable with `basicAuth` or a bearer token.                                                                                  |
| `headers`                           | Plain request headers. Not for secrets. `Accept`, `Content-Type`, `Host` and `Content-Length` are managed by the server.                                                                                                |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets.                                                                                                                |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                                                                                                                                       |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth` or an API key.                                                                                                                   |

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

## Health

The health check reads the server's version, confirms that it is Elasticsearch 8.x or 9.x, confirms that the index pattern matches at least one index, alias or data stream, reads the field mapping and checks that the time field is a date and that every field named under `fields` exists.
Each failure has its own message on the datasources page, so a wrong type, an unsupported version, an empty pattern, a missing time field and a missing role field are told apart.

## Field roles

The server reads the mapping of the index pattern through the field capabilities API, limited to indices with documents in the last 7 days, and keeps it for a minute.
From it, five roles are resolved by the first field present in the mapping, unless `fields` names one:

| Role      | Candidates, in order                                                                                                                   |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `message` | `message`, `body`, `body.text`, `log`, `msg`, `event.original`                                                                         |
| `level`   | `log.level`, `severity.text`, `severity_text`, `level`, `severity`, `loglevel`                                                         |
| `service` | `service.name`, `resource.service.name`, `resource.attributes.service.name`, `resource.attributes.service_name`, `service_name`, `app` |
| `traceId` | `trace.id`, `traceId`, `trace_id`, `traceID`                                                                                           |
| `spanId`  | `span.id`, `spanId`, `span_id`, `spanID`                                                                                               |

These cover ECS documents from Elastic Agent and Filebeat, the OpenTelemetry Collector's Elasticsearch exporter in both ECS and OTel modes, and OpenSearch's Simple Schema for Observability.
A field named under `fields` must exist in the mapping, or the datasource is unhealthy.

## Variables and discovery

A query variable on an Elasticsearch datasource takes `label_values(field)` or `label_values(<query>, field)`, where the query is in any of the three forms and narrows the values, and returns the field's values over the dashboard's time range, up to 10,000 of them, through a terms aggregation.
A `text` field is read through its `.keyword` subfield; a `text` field without one has no listable values.

The agent and the field browser list the aggregatable fields plus the `text` fields that have a `.keyword` subfield, the values of one field over a time range, every mapped field with its kind (`keyword`, `text`, `number`, `date`, `date_nanos`, `boolean`, `ip`, `nested`, `flattened`, `object` or `other`), the indices, aliases and data streams the pattern matches, and up to 20 of the newest documents with the resolved roles as columns.
A field mapped differently across indices keeps the first type the server lists.
Elasticsearch has no metric names, so the metric list is empty.

## Queries

A query takes one of three forms, chosen by its first characters:

| Form      | How it is chosen                                                                              | What it is                                                                                         |
| --------- | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| KQL       | The default, or a `kql:` prefix.                                                              | The Kibana query language, compiled on the server to Query DSL. See [what is out](../use/logs-and-traces.md#kql-on-elasticsearch-and-opensearch). |
| Lucene    | A `lucene:` prefix, or `queryLanguage: lucene` on the datasource.                             | The Lucene query string syntax, sent as a `query_string` query with `analyze_wildcard`.            |
| Query DSL | Text that starts with `{`.                                                                    | A JSON search body. Only `query`, `aggs`, `aggregations`, `size`, `sort`, `fields`, `_source` and `runtime_mappings` are accepted; any other key is refused so the body cannot widen the time range or the limits. |

Prefixes are case-insensitive, and an empty query matches every document.
The time range and any ad hoc filters wrap your query in a `bool` filter, in every form.

What a query returns depends on its kind and form:

- An instant query, such as the logs explorer's, returns log lines newest first as one frame with the columns `Time`, `TimeNs`, `Body`, `level`, `ID`, `Index`, `service`, `trace_id`, `span_id` and `Fields`.
  A role column is left out when no field plays that role, and `Fields` holds the remaining fields of the document as JSON.
  `TimeNs` is exact on a `date_nanos` time field, and on a `date` field it is the milliseconds the index stores times one million.
- A range query in KQL or Lucene counts documents over time: one series named `count`, with one point per step.
- A Query DSL body with `aggs` runs with `size` forced to 0 and its result is folded into series or a table, whatever the kind.
  A body without `aggs` returns lines like an instant query, with `size` capped at the line limit.

Lines are capped at 5,000 and at the query's row limit, whichever is lower, and `meta.truncated` says the cap was reached.
`meta.rows_total` is the number of matching documents when Elasticsearch counted them exactly, which it does up to 10,000, and is left out beyond that.
`meta.partial` says the cluster timed out or a shard failed.
A range query is refused when its step would make more than 11,000 buckets or more points than the point limit.

`$__from` and `$__to` expand to epoch milliseconds, `$__interval` to the step in seconds such as `300s`, and `$__range` as on Loki.
Inside a quoted string, such as a Query DSL value, a variable is escaped for JSON, and a variable with several values needs a format such as `${var:pipe}`.
Outside quotes, a variable with several values expands to a parenthesised list of quoted values joined with `or`.

### Aggregations

The result of `aggs` is folded by walking it beside the request:

- Bucket aggregations: `date_histogram`, `auto_date_histogram`, `histogram` (a time axis when its field is the time field, else numeric buckets in a table), `terms`, `multi_terms`, `filters`, `range`, `date_range` and `significant_terms`.
- Metric aggregations: `avg`, `sum`, `min`, `max`, `value_count`, `cardinality`, `median_absolute_deviation`, `percentiles`, `stats`, `extended_stats`, `rate`, `bucket_script`, `derivative`, `cumulative_sum` and `moving_fn`.
- When a time bucket aggregation appears anywhere in the nesting, the result is series, one per metric and per combination of the other buckets' keys, labelled with the bucket field (or the aggregation's name); a bucket with no metric under it gives a series named `count`.
  Percentiles add a `percentile` label.
- With no time bucket, the result is one table with a column per bucket level, then `count`, then a column per metric, and a row per innermost bucket, capped at the row limit.
- Only one bucket aggregation may sit at each level, and a time bucket cannot sit inside another.
  Anything else, such as `top_hits` or `composite`, is refused with `unsupported aggregation "x" (type y)`.
- More series or points than the limits is refused with a limit error.

### Ad hoc filters

Every ad hoc variable on an Elasticsearch datasource applies to its queries.
`=` and `!=` become a `term` (a `match_phrase` on a text field with no `.keyword` subfield), `=~` and `!~` a case-insensitive `regexp`, and `<` and `>` a `range`.
A text field with a `.keyword` subfield is filtered through the subfield.
Filters joined with OR become a `should` group.

### Lint

The lint endpoint checks a query without running a search.
KQL is compiled on the server and reports syntax errors and unknown fields (a warning).
Lucene and the `query` part of a Query DSL body are checked with the cluster's validate API, and a Query DSL body is also checked for JSON syntax and for keys that are not accepted.
Each diagnostic carries the offsets of the text it concerns.

### Paging

A query sent with `page_size` pages by offset: any page can be asked for directly, the total is exact and no `next` cursor is returned.
The last page that can be reached is set by the cluster's `index.max_result_window`, which the server reads with the mapping: 10,000 unless an index sets it lower.
A page past the window is refused with "pages past 10,000 results are not available on this index; narrow the time range", and the last reachable page already says it has no next page.
A Query DSL body with `aggs` cannot be paged.

