---
title: Metrics
description: Run PromQL on a Prometheus datasource, see the result as a time series with a legend table or as a table, ask in plain language, and read query errors with their position.
---

The metrics page runs a PromQL query on a Prometheus datasource without building a dashboard first.
Use it to check a query before it goes on a dashboard, or to look at a metric once.
It reads from the datasources an Admin configured, as [Connect Prometheus](../connect/prometheus.md) describes, and it sits beside the [logs page and the trace search](logs-and-traces.md) in the sidebar.

## Run a query

1. Choose **Metrics** in the sidebar, then a Prometheus source.
   With two or more sources they are listed under **Metrics**, and **More** opens the full list at `/explore/metrics`.
2. Type PromQL in the query box, such as `sum by (service) (rate(http_requests_total[5m]))`.
   The editor completes functions and keywords, and the server checks the query as you type.
3. Press **Run query**, or Cmd+Enter (Ctrl+Enter on Windows and Linux).
   Press `/` to move the focus to the query box from anywhere else on the page.
4. Read the result under **Graph**, or switch to **Table**.
   A line above it says how many series or rows came back, the step of a graph and how long the query took.
5. Change the range with the time picker above the results, and the query runs again.
   Dragging across the chart narrows the range to the part you selected.

A query the server finds invalid does not run.
The page shows the message with its line and column and a caret under the position.
A query that fails on the datasource shows the datasource's message and a **Retry** button.
A query that returns no series shows "No data in this range", with a hint to widen the time range or loosen the label matchers.

The same query runs from a terminal:

```sh
observe metrics query 'sum by (service) (rate(http_requests_total[5m]))' --datasource prom --from now-6h
```

[With the CLI](#with-the-cli-and-mcp) lists the other routes.

## Ask in plain language

Use Ask when you know what you want to see but not the PromQL.

1. Switch the mode at the top of the query card from **PromQL** to **Write a query**.
   Ctrl+I or Cmd+I switches from anywhere on the page, and the page remembers the mode you last chose.
2. Write a question such as "requests per second by service over the last hour".
3. Press Enter or **Write query**.
   Shift+Enter adds a line, and Esc cancels a question that is still running.
4. The page runs the answer at once and shows its explanation and any warnings, such as "No series in the last 15 minutes."

Editing the query switches to the PromQL segment, with a "From: <question>" line and **Ask again**, which brings the question back.
**Edit as PromQL** switches without changing the query.
When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, and switches to the PromQL segment.
Without a model the Write a query segment is disabled and says "Add a model key to ask in plain language"; [LLM providers](../connect/llm-providers.md) describes how to configure one.

### How Ask writes the query

The server reads the datasource's labels, the metric names that fit your question with their types, and the values of a service label, so the query names real ones.
When the whole catalog is small, it reads every metric name.
When the catalog is large, it searches it for the words of your question.

Then it asks the model once.
It checks the answer with the query lint and one instant run over the last 15 minutes of the range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.
The limits on questions are the same as on the logs page, and [Ask in plain language](logs-and-traces.md#ask-in-plain-language) lists them.

## Find a metric or a label

**Browse metrics** opens a browser above the query box with the metrics of the datasource and a search box.

1. Type to filter the list.
   It shows the first 100 matches, and each metric's help text appears as its tooltip.
2. Choose a metric to see the labels it carries, then a label to see its values.
3. Choose a value to insert a matcher such as `service="checkout"` at the cursor.

**Insert metric** puts the metric's name at the cursor.
**Back to labels** returns from the values to the label list.
**Hide metrics** closes the browser.
The names come from the server, which asks the datasource, so the browser never reaches the datasource itself.

## Switch between a graph and a table

**Graph** runs a range query and draws each series as a line, with a legend table under the chart that lists the mean, maximum and last value of each series.
**Table** runs an instant query at the end of the range and lists one row per series with its labels and value.
The choice is in the address as `mode=table`, and with no `mode` the page shows the graph.

## Fix a query that hits a limit

The server caps every query by time, series and points, and a datasource may set lower caps.
A query that exceeds a limit fails with a message that names the limit it hit.
The page then says to add label matchers, aggregate with `sum by (label)`, or narrow the time range.
A query that takes too long fails with `query_timeout`, and the page says to narrow the range or aggregate.

When the table stops at the server's row limit, a note under the tabs says so.
Add label matchers or aggregate to see the rest.

A graph's step follows the range and the width of the chart, so a longer range draws coarser points.
It never drops below the datasource's scrape interval, as [Connect Prometheus](../connect/prometheus.md#step) describes.

## Save a search

**Save search** in the top bar keeps the query that ran, with the time range as written and the graph or table choice, in a folder.
The folder is your My Dashboards by default, and everyone who can view the folder can open the search.
**Save search** stays disabled until a query has run.

To open it again, choose **Saved searches** in the top bar, or find it on `/explore/metrics`, which lists saved searches under the sources with their folder and who saved them.
The [search](search.md) of the workspace finds it too.
Opening one runs it on its datasource over the saved range, shows its title and folder above the query, and keeps its id in the address as `saved`.

When you change the query, the range or the graph or table choice, someone with Edit on the search gets **Update saved search**, which saves the change into it.
**Save search** still saves a new search.
Only a Prometheus datasource holds a metrics search.
On a phone the same actions sit in the **More actions** menu.

## Add to a dashboard or an observation

**Add to dashboard**, in the header of the results, keeps the query as a panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The graph becomes a time series panel with a legend table, and the table becomes a table panel with an instant query.

**Add to observation** beside it copies the same panel into an [observation](observations.md#add-to-an-observation-from-a-panel-or-from-logs), named Metrics, over the range the page shows.
Both stay disabled until a query has run, and say "Run a query first."

## With the CLI and MCP

- `observe metrics query` runs the same query from a terminal, and `--ask "<question>"` takes a question instead of the query.
- `observe saved-searches save --kind metrics` saves a search.
- `observe datasources metrics`, `labels` and `label-values` list what the browser lists.
- The MCP tools `query_range`, `query_instant`, `list_metrics`, `list_labels` and `list_label_values` do the same for an agent.
- `draft_query` answers a question with PromQL.

See [Metrics](cli.md#metrics) and [Discovering what a datasource holds](cli.md#discovering-what-a-datasource-holds) in the CLI page, and [MCP](mcp.md).

## Reference

### Sources and addresses

- Each Prometheus datasource has its own page at `/explore/metrics/<datasource uid>`.
- `/explore/metrics` lists the Prometheus sources with their language and whether they answer.
- The five sources you used last on this browser come first in the sidebar and the rest follow by name.
  The order is set when the app loads and stays fixed until you reload.
- With one source nothing is listed under **Metrics**, and **Metrics** still opens the list.
- On a phone the line under the page title names the source and PromQL, as it does on the logs and trace pages.
  With two or more sources it opens the **Metric sources** sheet, and with one there is nothing to switch to.
- An address that carries a query but no source, such as `/explore/metrics?q=up`, opens the source you used last on this browser, else the first by name.
- The address keeps the source, `q`, `from`, `to` and `mode`, and `ask` when the query was written from a question, so **Copy link** in the top bar copies a query that opens the same way.
- With no `from` and `to` the page reads the last hour.

### Keys

| Keys | What they do |
| --- | --- |
| `/` | Focus the query. |
| Cmd+Enter or Ctrl+Enter | Run the query. |
| Cmd+I or Ctrl+I | Switch between Write a query and PromQL, from anywhere on the page. |

[Keyboard shortcuts](keyboard-shortcuts.md#metrics-page-keys) lists the keys of every page.

### Roles

Everyone who can open the app can run a metrics query, as with logs and traces.
The query runs as given, so a Prometheus datasource shows what its credentials can read.
