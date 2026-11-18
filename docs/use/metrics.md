---
title: Metrics
description: Run PromQL on a Prometheus datasource, see the result as a time series with a legend table or as a table, and read query errors with their position.
---

The metrics page runs a PromQL query on a Prometheus datasource without building a dashboard first.
It reads from the datasources an Admin configured, as described in [Connect Prometheus](../connect/prometheus.md), and it sits beside the [logs page and the trace search](logs-and-traces.md) in the sidebar.

## Choose a source

Each Prometheus datasource has its own page at `/metrics/<datasource uid>`.
`/metrics` lists the Prometheus sources with their language and whether they answer, and a source opens on its own page.

With two or more Prometheus datasources they are listed under **Metrics** in the sidebar.
The five you used last on this browser come first and the rest follow by name, and **More** opens the Metrics list.
The order is set when the app loads and stays fixed until you reload.
With one source nothing is listed under **Metrics**, and **Metrics** still opens the list.
An address that carries a query but no source, such as `/metrics?q=up`, opens the source you used last on this browser, else the first by name.
The address keeps the source, `q`, `from`, `to` and `mode`, so **Copy link** in the top bar copies a query that opens the same way.

## Run a query

Type PromQL in the query box.
The editor completes functions and keywords, and the server checks the query as you type.
`⌘Enter` or `Ctrl+Enter` runs it, and so does **Run query**; `/` moves the focus to the query box from anywhere else on the page.
The time picker above the results sets the range, and changing it runs the query again.
Dragging across the chart narrows the range to the part you selected.

A query the server finds invalid does not run.
The page shows the message with its line and column and a caret under the position.
A query that fails on the datasource shows the datasource's message and a **Retry** button.

## Browse metrics

**Browse metrics** opens a browser above the query box with the metrics of the datasource and a search box.
Type to filter the list; the list shows the first 100 matches, and each metric's help text appears as its tooltip.
Choose a metric to see the labels it carries, choose a label to see its values, and choose a value to insert a matcher such as `service="checkout"` at the cursor.
**Insert metric** puts the metric's name at the cursor.
**Back to labels** returns from the values to the label list.
The names come from the server, which asks the datasource, so the browser never reaches the datasource itself.
**Hide metrics** closes the browser.

## Graph and table

**Graph** runs a range query and draws each series as a line, with a legend table under the chart that lists the mean, maximum and last value of each series.
**Table** runs an instant query at the end of the range and lists one row per series with its labels and value.
The choice is in the address as `mode=table`; with no `mode` the page shows the graph.

The server caps every query at 15 seconds, 200 series and 100,000 points.
A query that exceeds a limit fails with a message that names it, and the page says how to narrow the query: add label matchers, aggregate with `sum by (label)`, or shorten the range.
The step follows the range and the width of the chart, so a longer range draws coarser points.

## Roles

Everyone who can open the app can run a metrics query, as with logs and traces.
The query runs as given, so a Prometheus datasource shows what its credentials can read.

## With the CLI

`observe metrics query` runs the same query from a terminal, and `observe metrics names`, `labels` and `values` list what the browser lists; see [Metrics](cli.md#metrics).
The MCP tools `query_range`, `query_instant`, `list_metrics`, `list_labels` and `list_label_values` do the same for an agent; see [MCP](mcp.md).
