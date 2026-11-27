## Prometheus (PromQL)
- Use `$__rate_interval` as the window in `rate()` and `increase()` in saved panels (your own reads use a short explicit one); use `$__range` for an increase over the whole range.
- Aggregate with `sum by (label)` to keep series few; set `legend` to `{{label}}`.
- Latency quantiles: `histogram_quantile(0.99, sum by (le) (rate(<name>_bucket[$__rate_interval])))`.
- A ratio whose numerator may have no series yet: `(sum(rate(<errors>[$__rate_interval])) or vector(0)) / sum(rate(<all>[$__rate_interval]))`.
- An earlier-period comparison goes in one query: `label_replace(<now>, "period", "now", "", "") or label_replace(<same with offset 1d>, "period", "1d ago", "", "")`, so the panel has data before the earlier period exists.
- `ALERTS{alertstate="firing"}` lists firing alerts; a second query on `ALERTS_FOR_STATE` shows since when.
- Report a window's figure from a measure tool, never from range points or spot values: `measure_window` for a peak, minimum, average or duration (`first_time` with `where` gives when, such as a queue reaching 0), `measure_aggregate` for a total, an error share (`ratio`) or a quantile per group.
- Read a store's label names before selecting by a service name.
