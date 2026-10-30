---
title: Connect Prometheus
description: Add a Prometheus server as a datasource and set its authentication.
---

A Prometheus entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

A Prometheus behind a bearer token references the token by variable name, and the variable goes in the same env file as the other settings:

```yaml
prometheus:
  url: https://prometheus.example.com
  bearerTokenEnv: SLASHOBSERVE_PROMETHEUS_TOKEN
```

## Fields

| Field                               | Notes                                                                                                   |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                          |
| `scrapeInterval`                    | A Go duration, default `15s`. Feeds the `$__rate_interval` macro and the smallest query step.           |
| `headers`                           | Plain request headers. Not for secrets.                                                                 |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets. |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                       |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                 |

## Exemplars

A time series query with `exemplar: true` also asks Prometheus for exemplars over the same range, through `/api/v1/query_exemplars`.
Prometheus returns them only when it runs with `--enable-feature=exemplar-storage` and the scraped application exposes them.
SlashObserve keeps at most one exemplar per step, the one with the highest value, and at most 2,000 per query.
If the exemplar request fails, the panel still shows its series.
