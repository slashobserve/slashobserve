---
title: Connect OpenSearch
description: Add an OpenSearch cluster as a datasource for logs, with its index pattern, time field, field roles and authentication.
---

An OpenSearch entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.
OpenSearch 2.x and 3.x are supported; for an Elasticsearch cluster use the [Elasticsearch](elasticsearch.md) type instead, and the health check says so when the two are mixed up.
The two types share one adapter, so this page and the Elasticsearch page describe the same fields, roles and discovery.

A cluster behind basic auth names the index pattern and references the password by variable name:

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
Amazon OpenSearch Service domains that require AWS Signature Version 4 are not supported yet; put a proxy that signs requests in front of the domain, or use a domain with basic auth through fine-grained access control.

## Health

The health check reads the server's version, confirms that it is OpenSearch 2.x or 3.x, confirms that the index pattern matches at least one index, alias or data stream, reads the field mapping and checks that the time field is a date and that every field named under `fields` exists.
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

These cover OpenSearch's Simple Schema for Observability (`body`, `severity.text`, `traceId`), ECS documents from Data Prepper and Fluent Bit, and the OpenTelemetry Collector's OpenSearch exporter.
A field named under `fields` must exist in the mapping, or the datasource is unhealthy.
An OpenSearch `flat_object` field is reported with the kind `flattened`.

## Variables and discovery

A query variable on an OpenSearch datasource takes `label_values(field)` and returns the field's values over the dashboard's time range, up to 10,000 of them, through a terms aggregation.
A `text` field is read through its `.keyword` subfield; a `text` field without one has no listable values.

The agent and the field browser list the aggregatable fields plus the `text` fields that have a `.keyword` subfield, the values of one field over a time range, every mapped field with its kind (`keyword`, `text`, `number`, `date`, `date_nanos`, `boolean`, `ip`, `nested`, `flattened`, `object` or `other`), the indices, aliases and data streams the pattern matches, and up to 20 of the newest documents with the resolved roles as columns.
A field mapped differently across indices keeps the first type the server lists.
OpenSearch has no metric names, so the metric list is empty.

## Queries

Queries in KQL and Lucene, with `$__from`, `$__to`, `$__interval` and dashboard variables, are described on this page once query execution lands.
