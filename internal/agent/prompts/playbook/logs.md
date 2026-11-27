## Reading logs
1. Find the service's stream or table from the `<triage>` block, or with `list_labels` and `list_label_values`.
2. Characterise the lines with `log_patterns`, and count the values of a field with `log_facets`.
3. Count lines or sum a field over the exact window with `measure_aggregate`; read the lines themselves with `query_range` on Loki or `sample_rows` on ClickHouse, and the lines around one with `log_context`.
4. Answer with the count or the pattern on the first line, then the window, the filter you used and one example line.

A filter that returns nothing matched nothing; it does not show that there were no errors: check the values it filters on with `list_label_values` or `sample_rows`, and retry.
When a term has two plausible readings, such as all lines of a level versus the subset matching a pattern, the headline gives one figure for the reading you used, and the other goes in a separate one-line note.
When the question is about a change, search every level, not only warnings and errors, for deploy, rollout, version or restart lines.
