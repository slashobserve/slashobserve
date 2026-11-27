## Measuring a value
1. Resolve the service and the metric from the `<triage>` block's service map, or with `list_services` and `list_metrics` scoped to the service.
2. Measure over the exact window the question names: `measure_window` for a peak, a minimum or a value at a time, `measure_aggregate` for a total, ratio, share or quantile; on Loki `measure_aggregate` counts lines and sums or unwraps fields.
3. A current or right-now rate is a short window ending now, such as `rate(x[5m])`, and a rate an hour ago or yesterday is the same read with PromQL's `offset 1h`; never compute one as a longer increase divided by its length, and use a wide window only when the question asks for an average over that span.
4. Answer with the one value on the first line, in the question's unit, then the window and the series it came from.

Answer at the grain the question names, its window, its aggregation and whether it is per request or a total: a short spike or peak inside the window comes later as a separate observation, never as the headline.
When a term has two plausible readings, such as a sum versus a per-member mean, the headline gives one figure for the reading you used, and the other reading goes in words, with its figure only in a separate one-line note; a total across members is a sum, and a ratio or percentile across a set is computed over the set, never summed.
When you compare or rank the members of a set, such as services or endpoints, give every member the same metric, window and unit, including the ones that look fine.
When the question is whether one factor or another explains a change, decide from the ratio of each over the same windows and state the ratios: one within about 10% of 1, or one that moves against the change, explains nothing, and a mean over another span or a short spike does not overturn them.
Take a baseline such as typical or normal from the question's whole window without the event's own span, never from a sub-window.
When the first metric you try is empty, look for the service's span-metrics series, whose names contain `span_metrics`, before you end.
Saved evidence panels keep `$__rate_interval` as the authoring guide says.
