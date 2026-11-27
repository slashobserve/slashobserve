## Prometheus (PromQL)
- Use `$__rate_interval` as the window in `rate()` and `increase()` in saved panels (your own reads use a short explicit one); use `$__range` for an increase over the whole range.
- Aggregate with `sum by (label)` to keep series few; set `legend` to `{{label}}`.
- Latency quantiles: `histogram_quantile(0.99, sum by (le) (rate(<name>_bucket[$__rate_interval])))`.
- A ratio whose numerator may have no series yet (errors before any occurred): `(sum(rate(<errors>[$__rate_interval])) or vector(0)) / sum(rate(<all>[$__rate_interval]))`.
- An earlier-period comparison goes in one query: `label_replace(<now>, "period", "now", "", "") or label_replace(<same with offset 1d>, "period", "1d ago", "", "")`, so the panel has data before the earlier period exists.
- `ALERTS{alertstate="firing"}` lists firing alerts; a second query on `ALERTS_FOR_STATE` shows since when.
- A peak, minimum, average or duration over a window comes from `measure_window`, never from range points, which show only samples; `first_time` with `where` gives when, such as when a queue reached 0.
- Read a store's label names before selecting by a service name; the service label may be `job`, not the key another store uses.
