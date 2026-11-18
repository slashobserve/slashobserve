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

## Ask in plain language

The mode switch at the top of the query card switches between **PromQL** and **Ask**.
Ctrl+I or Cmd+I switches from anywhere on the page, and the page remembers the mode you last chose.
Write a question such as "requests per second by service over the last hour" and press Enter or **Ask**, and press Shift+Enter to add a line.
Esc cancels a question that is still running.

The server reads the datasource's labels, the metric names that fit your question with their types, and the values of a service label, so the query names real ones.
Then it asks the model once, checks the answer with the query lint and one instant run over the last 15 minutes of the range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.
When the whole catalog is small, every metric name is read; when it is large, the server searches it for the words of your question.
The page runs the answer at once and shows its explanation and any warnings, such as "No series in the last 15 minutes."

Editing the query switches to the PromQL segment, with a "From: <question>" line and **Ask again**, which brings the question back.
When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, and switches to the PromQL segment.
Without a model the Ask segment is disabled and says "Add a model key to ask in plain language"; [LLM providers](../connect/llm-providers.md) describes how to configure one.

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

The server caps every query by time, series and points, and a datasource may set lower caps.
A query that exceeds a limit fails with a message that names the limit it hit, and the page says how to narrow the query: add label matchers, aggregate with `sum by (label)`, or shorten the range.
The step follows the range and the width of the chart, so a longer range draws coarser points.

## Save a search

**Save search** in the top bar keeps the query that ran, with the time range as written and the graph or table choice, in a folder, by default your My Dashboards.
It stays disabled until a query has run.
Everyone who can view the folder can open it.
**Saved searches** in the top bar lists the saved metrics searches, and `/metrics` lists them under the sources with their folder and who saved them.
Opening one runs it on its datasource over the saved range, and the [search](search.md) that covers saved searches finds it too.
Only a Prometheus datasource holds a metrics search.
On a phone the same actions sit in the **More actions** menu.

## Add to a dashboard or an observation

**Add to dashboard**, in the header of the results, keeps the query as a panel on a dashboard you can edit.
The graph becomes a time series panel with a legend table, and the table becomes a table panel with an instant query.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
**Add to observation** beside it copies the same panel into an [observation](observations.md#add-to-an-observation-from-a-panel-or-from-logs) over the range the page shows.
Both stay disabled until a query has run, and say "Run a query first."

## Roles

Everyone who can open the app can run a metrics query, as with logs and traces.
The query runs as given, so a Prometheus datasource shows what its credentials can read.

## With the CLI

`observe metrics query` runs the same query from a terminal, `--ask "<question>"` takes a question instead of the query, `observe saved-searches save --kind metrics` saves it, and `observe metrics names`, `labels` and `values` list what the browser lists; see [Metrics](cli.md#metrics).
The MCP tools `query_range`, `query_instant`, `list_metrics`, `list_labels` and `list_label_values` do the same for an agent, and `draft_query` answers a question with PromQL; see [MCP](mcp.md).
