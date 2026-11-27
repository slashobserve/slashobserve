---
title: Datasources and variables
description: How a dashboard names its datasources, the eight variable types, and how multi-value, All, datasource and ad hoc variables work, with the formats and limits.
---

A datasource is a configured connection to a backend such as Prometheus or Loki, and a dashboard names each one by `uid`.
A variable lets a viewer change what a dashboard's queries use, so one dashboard serves many services or environments without anyone editing the document.
Use a datasource uid in every panel, and add variables when viewers need to filter, switch service or switch environment.
Both are fields of the [ObserveSpec](observespec.md) document.

For example, a dashboard can offer a `job` dropdown, and the viewer's choice replaces `$job` in every query:

```json
"variables": [
  {
    "name": "job",
    "type": "query",
    "datasource": "prom",
    "query": "label_values(up, job)",
    "multi": true,
    "includeAll": true
  }
],
"panels": [
  {
    "id": "p_up",
    "type": "timeseries",
    "title": "Up by job",
    "gridPos": { "x": 0, "y": 0, "w": 12, "h": 8 },
    "datasource": "prom",
    "queries": [{ "query": "sum by (job) (up{job=~\"$job\"})" }]
  }
]
```

Here `prom` is the uid of a datasource, and the viewer can pick one job, several jobs or All.

## Datasources

The `uid` of an entry in the [datasources file](../reference/datasources-file.md), or of a datasource an Admin [added in the app](../connect/datasources-in-the-app.md), is the name a dashboard uses.
A uid is up to 40 letters, digits, `_` or `-`, starting with a letter or digit.
The entry's `type` decides the query language and which macros a query may use.
It is `prometheus`, `clickhouse`, `loki`, `tempo`, `jaeger`, `elasticsearch` or `opensearch`.

A data panel sets `datasource`, and any of its queries can set its own `datasource` to use a different one.
A `query`, `adhoc` or `groupby` variable sets `datasource` too.
A save is rejected with `unknown_datasource` when a uid names no configured datasource.
So renaming or removing a uid breaks the next save of every dashboard that uses it.

### Check that a datasource is healthy

1. As an Admin, open the Datasources page.
   Each row shows the datasource's health and, when it is unhealthy, the cause the server knows, such as the HTTP status the datasource answered.
   A line under the cause says what to do: try again in a moment, check the credentials, change a setting, or connect a supported version.
2. Choose **Recheck** on a row to run a fresh check.
3. Over the API, `GET /api/v1/datasources` returns each uid with its type, health and `source`.
   An unhealthy one carries `message` and `detail`.
   Each one also carries `health_state`: `unchecked`, `healthy`, `failing` or `rechecking`, as the [States reference](../reference/states.md#datasource-health) lists them.
   A failing or rechecking one carries `failure_kind`, so a script or an agent can decide without reading `message`:

   | `failure_kind` | What it means | What to do |
   | --- | --- | --- |
   | `transient` | The datasource did not answer, timed out or hit a limit, or the check failed for a reason the server cannot tie to its settings. | Try again later; the server keeps checking. |
   | `credentials` | The datasource refused the saved credentials, or the server cannot read them. | Set the credentials again. |
   | `configuration` | The datasource's settings, its user's grants or the server's settings do not fit. | Change what `message` names, then recheck. |
   | `unsupported` | The datasource runs a version the server does not support. | Connect a supported version. |

   A check that was cancelled, or a caller that left before the check answered, reads `unchecked` with `healthy: false` and no `failure_kind`, and the server does not keep that result.
   `POST /api/v1/datasources/{uid}/recheck` runs a fresh check.

`list_datasources` does the same over MCP and in the built-in agent, `recheck_datasource` over MCP, and `observe datasources recheck` from the CLI.
The agent is also told which datasources are healthy before it starts.
A recheck skips the 10 second cache of health results, and checks of one datasource are at least 2 seconds apart, so a retry loop cannot hammer it.

## Variables

A variable gives the viewer a control, and its selection fills `$name` in the queries.
To add one, declare it in `variables` and write `$name`, `${name}` or `${name:format}` in a query.

1. Pick the type that gives the control you want; the [types table](#variable-types) lists them.
2. Give it a `name` that starts with a letter and continues with letters, digits or `_`, up to 40 characters.
3. Set the fields of its type.
   A `query` variable needs `datasource` and `query`, a `custom` variable needs `options`, and a `constant` variable needs `value`.
4. Use it in a query.
   Without a selection the variable starts at its `default`.

Variable names are unique, and a document has at most 20 variables.
The value in the URL is `var-<name>`, so a link keeps a viewer's selection.
`skipUrlSync: true` keeps the variable out of the address: its selection is not read from `var-<name>` and not written to it, so a shared link does not carry it.
The optional `label`, `description` and `hide` change how the control looks.
`hide: label` drops the name and `hide: variable` drops the control, and neither drops the selection.

### Variables that depend on each other

Variables are resolved in the order they are declared.
A `query` variable's query can use only variables declared before it, so a `service` variable can feed a `route` variable but not the reverse.
It cannot use an `adhoc` variable.
Panel queries can use any variable.
When a viewer changes a variable, the variables whose queries use it reload their values.

### Pick what a variable starts at

With no `default`, a variable starts at a value that depends on its type:

- A `query` or `custom` variable starts at All if it sets `includeAll` and at its first value if not.
- An `interval` variable starts at `$__auto` if it sets `auto` and at its first option if not.
- A `textbox` variable starts at its text, or empty.
- A `datasource` variable starts at the first matching datasource.

## Let viewers pick several values

Set `multi: true` on a `query` or `custom` variable to let the viewer pick several values, and `includeAll: true` to add All.
A `default` can be one value, an array of values for a `multi` variable, or `"$__all"` for All.
`"$__all"` needs `includeAll` and is given alone.
A `custom` or `interval` default has to be one of its options.

How a selection reaches the query depends on its format.
Write the format in the query, as in `${job:pipe}`, or leave it out and the server picks one from where the variable sits.
So `pod=~"$pod"` in PromQL, `{app=~"$app"}` in LogQL and `{ resource.service.name =~ "$service" }` in TraceQL work for one value, several values and All.
In SQL, `ServiceName IN ($service)` works for several values.
In a search query string, `service.name:$service` matches any of the selected values.

All becomes the full list of values, formatted the same way, except in the `regex` format, where it becomes `.*`.
Set `allValue` to send your own text for All instead, such as `.*`.
The server uses `allValue` without a format.
[Formats and All](#formats-and-all) has the rest.

## Let viewers allow their own values

Set `allowCustomValue: true` on a `query` or `custom` variable to let the viewer enter a value that is not in its list.
A `custom` variable refuses a value outside its `options` unless it sets `allowCustomValue`.
A `query` variable accepts any value either way.

## Switch a whole dashboard between datasources

A `datasource` variable lets a viewer switch a whole dashboard between datasources of one type, such as a production and a staging Prometheus.

1. Declare the variable with `type: "datasource"` and a `datasourceType`: `prometheus`, `clickhouse`, `loki`, `tempo`, `elasticsearch` or `opensearch`.
   Add `regex` to keep only the uids it matches, and `default` with a uid, never a `${name}`.
2. Write `${name}` as the `datasource` of a panel, of a query, of a `query` variable, of an `adhoc` variable or of a `groupby` variable.
3. The query runs on the selected datasource, and its macros follow the variable's `datasourceType`.

The form is exactly `${name}`, so `$name` and `${name:format}` are not datasource references.
A viewer picks exactly one datasource, so the variable has no `multi` or `includeAll`.
The variable lists only datasources of its `datasourceType`, so the same query text runs on whichever one the viewer picks.

This is `spec/examples/datasource-variable.json` without its title and time range:

```json
"variables": [
  {
    "name": "ds",
    "type": "datasource",
    "datasourceType": "prometheus",
    "default": "prom"
  },
  {
    "name": "job",
    "type": "query",
    "datasource": "${ds}",
    "query": "label_values(up, job)"
  }
],
"panels": [
  {
    "id": "p_up",
    "type": "timeseries",
    "title": "Up by job",
    "gridPos": {
      "x": 0,
      "y": 0,
      "w": 12,
      "h": 8
    },
    "datasource": "${ds}",
    "queries": [
      {
        "query": "sum by (job) (up{job=~\"$job\"})"
      }
    ]
  }
]
```

A `query`, `adhoc` or `groupby` variable can use `${name}` only when the `datasource` variable is declared before it.
A panel or panel query can use any `datasource` variable.
The save refuses a bad reference:

| Code | Cause |
| --- | --- |
| `unknown_variable` | `${name}` names no variable. |
| `invalid_reference` | `${name}` names another type of variable. |
| `variable_order` | The `datasource` variable is declared later. |

The Datasources page and the datasource filter on the Dashboards page count a dashboard under a datasource variable's `default`.
A datasource variable with no `default` adds nothing to those counts.

`datasourceType` has no `jaeger` value, because the ObserveSpec schema does not list Jaeger there yet.
A panel names a [Jaeger](../connect/jaeger.md) datasource by its uid.

## Set a minimum step with an interval variable

An `interval` variable can also set how fine a panel's queries are.

1. Declare an `interval` variable with `options` such as `1m` and `5m`.
   Set `auto: true` to add `$__auto`, which chooses a step from the time range.
2. Write `${name}` as the `interval` of a panel or of a query.
3. The query's step is at least the step selected in that variable.

The form is exactly `${name}`, and the variable must be an `interval` variable.
An undeclared name is rejected as `unknown_variable` and another type as `invalid_reference`.
A query's own `interval` wins over the panel's.
`$__interval` in the query follows the step, so a coarser step also widens it.
`$__auto` uses the variable's `autoSteps` and `autoMin`.

A query can also set `intervalFactor`, a whole number from 2 to 10, to multiply its step after the minimum is applied.
An instant query passes the selected step on as its minimum interval, and the factor does not apply to it.

## Ad hoc filters

An `adhoc` variable lets a viewer add key, operator and value filters to every query on one datasource.

1. Declare the variable with `type: "adhoc"` and a `datasource`.
2. The viewer adds filters in the control, such as `service = checkout`.
3. The filters apply to queries on Prometheus, ClickHouse, Elasticsearch and OpenSearch datasources.
   They filter Prometheus, Elasticsearch and OpenSearch queries automatically, and ClickHouse queries only through `$name` or with `applyToAll`.
   A Loki, Tempo or Jaeger query takes no ad hoc filters.

The operators are `=`, `!=`, `=~` and `!~`, plus `<` and `>` on ClickHouse, Elasticsearch and OpenSearch.
The filter picker offers `<` and `>` on ClickHouse only, though a Spec may set them on the others.

How the filters reach the query depends on the datasource:

- On Prometheus the filters are added as label matchers to every selector in every query on that datasource, and a query cannot refer to the variable.
  Prometheus can only AND label matchers, so a variable that uses `<`, `>` or `or` makes its queries fail with an error that names the operator or condition.
- On ClickHouse a query uses the variable explicitly, as `$name` in a `WHERE` clause such as `AND $name`.
  It becomes the filters joined by `AND`, or `1 = 1` when there are none.
  A filter that starts an `or` group opens a new group, so `a, b, or c` is `(a AND b) OR c`.
  `<` and `>` compare as numbers when both sides are plain numbers and as text otherwise.
- On Elasticsearch and OpenSearch every ad hoc variable filters every query on its datasource, and filters joined with `or` become a `should` group.
  The [Elasticsearch](../connect/elasticsearch.md#ad-hoc-filters) page describes the clauses.

A ClickHouse variable with `applyToAll` also filters every query on its datasource that leaves out `$name`.
It does this through `additional_table_filters` on the tables the query reads, and each table gets only the filters whose key is one of its columns.
Without `applyToAll`, a query that leaves out `$name` ignores the filters.
The Grafana importer sets `applyToAll` on every ClickHouse, Elasticsearch and OpenSearch ad hoc variable.

### Start with filters in place

`default` is up to 20 filters that the dashboard uses until a viewer changes them.
Each filter has a `key` (1 to 200 characters), an `operator`, a `value` (up to 500 characters, possibly empty), an optional `condition` (`and` or `or`) and optional chip texts `keyLabel` (up to 200 characters) and `valueLabel` (up to 500), which change only what the chip shows.
Without a `var-<name>` parameter the dashboard uses these filters, and a link that sets the parameter replaces them.
`var-<name>=` with nothing after the equals sign means no filters, which is how a viewer clears the saved ones.
A filter with `condition: "or"` is written in the URL with a leading `|`, as `var-<name>=|key|op|value`.

### Always apply some filters

`baseFilters` take the same objects, up to 20, and are always applied.
They are never shown and never in the URL.
They are one more group that the selected filters cannot loosen.

### Limit what the picker offers

`keys` (up to 100) are the keys the filter picker offers instead of asking the datasource.
`listedValuesOnly: true` makes the picker accept a filter only when its key and value come from the lists it shows, and the value list comes from Prometheus label values.
An operator outside the six is a schema error when the Spec is saved.
A key that is not a label name (Prometheus, Elasticsearch and OpenSearch) or a column name (ClickHouse) fails the save checks.

### Let viewers group by labels

A `groupby` variable lets a viewer pick labels, up to 20, and adds them to the grouping of every aggregation in every query on its datasource, as Grafana does, so the queries need no change.
It works on Prometheus datasources only.
Its `default` lists the labels selected from the start, and `keys` (up to 100 label names) are the labels its picker offers.
A query that writes `$name` gets the labels there and no automatic grouping.
An aggregation written with `without` keeps a picked label by dropping it from its `without` list.

## Reference

### Variable types

Each type has its own fields.
A query refers to a variable as `$name`, `${name}` or `${name:format}`.

| Type | Fields | Values |
| --- | --- | --- |
| `query` | `datasource`, `query`, `regex`, `sort`, `refresh`, `staticOptions`, `staticOrder`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default` | The result of the query on its datasource. |
| `custom` | `options`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default` | The listed options, up to 200. |
| `constant` | `value` | The one value, up to 2000 characters. It has no control. |
| `interval` | `options`, `auto`, `autoSteps`, `autoMin`, `default` | Durations such as `1m` or `1h`, plus `$__auto` when `auto` is set. |
| `textbox` | `default` | Free text up to 200 characters. |
| `datasource` | `datasourceType`, `regex`, `default` | The uids of the configured datasources of that type. |
| `adhoc` | `datasource`, `applyToAll`, `default`, `baseFilters`, `keys`, `listedValuesOnly` | Key, operator and value filters that the viewer adds. |
| `groupby` | `datasource`, `default`, `keys` | Labels the viewer picks to group by. |

Every variable also takes `name`, `label`, `description`, `hide` and `skipUrlSync`.

Details of a `custom` variable:

- Its `options` are strings, or `{ "text": "Production", "value": "prod" }` objects.
- An object shows `text` in the picker while the selection, the URL and the queries use `value`.

Details of an `interval` variable:

- `options` takes up to 50 unique durations, each a number from 1 to 99999 and a unit of `ms`, `s`, `m`, `h`, `d`, `w` or `y`.
- `autoSteps` is a whole number from 1 to 1000, and `autoMin` is a duration.

### Query variables

A `query` variable's `query` depends on the datasource:

| Datasource | Query |
| --- | --- |
| Prometheus | `label_values(label)` or `label_values(selector, label)`. |
| ClickHouse | One `SELECT`, whose first column gives the values. |
| Loki | `label_values(label)` or `label_values({selector}, label)`. |
| Elasticsearch, OpenSearch | `label_values(field)` or `label_values(query, field)`. |
| Tempo, Jaeger | No query variables. |

The [Loki](../connect/loki.md#variables-and-discovery) and [Elasticsearch](../connect/elasticsearch.md#variables-and-discovery) pages describe the Loki and search forms.
A `query` variable lists at most 1000 values.

Details of `regex`:

- It keeps only the values it matches, or the part of each value it captures.
- Named groups `text` and `value` set what the picker shows and what is selected.
- A `text` group alone keeps the whole value.

Details of `sort`, which orders by the shown text:

- `alpha` and `alphaDesc` compare by character, so every capital letter sorts before every lowercase letter.
- `alphaCaseInsensitive` and `alphaCaseInsensitiveDesc` compare by character after lowercasing, so `a` comes before `B`.
  Values that differ only in case keep a fixed order, capitals first.
- `num` and `numDesc` compare by the first number in each value, and values with no number come first in `num`.
- `natural` and `naturalDesc` compare digit runs as numbers and other text by character, so `a9` comes before `a10`.
  With equal numbers, the longer run comes first, so `007` comes before `7`.

Details of `refresh`:

- `"never"` runs no query and offers only its `staticOptions`.
- `"load"`, the default, runs the query when the dashboard opens, and again when a variable it depends on changes.
  It does not run when the time range changes, so the values keep the time range the dashboard opened with.
- `"timeRange"` also runs the query again whenever the time range changes.

Details of `staticOptions`:

- It holds up to 50 `text` and `value` pairs.
- They are offered next to the query results, before them by default.
- `staticOrder` set to `after` puts them after the results, and `sorted` merges them with the results and sorts with `sort`.
- A value that the query also returns appears once.
- The values list of a `query` variable carries the same texts when its regex captures them.

### Formats and All

A selection reaches the query in one of these formats:

| Format | Result for `a` and `b` |
| --- | --- |
| `regex` | `(a\|b)`, with regular expression characters escaped. |
| `pipe` | `a\|b` |
| `csv` | `a,b` |
| `singlequote` | `'a','b'` |
| `doublequote` | `"a","b"` |
| `sqlstring` | `'a','b'`, with SQL escaping. |
| `raw` | The value itself. It allows one value and refuses several. |

When a reference gives no format, the server picks one from where the variable sits and which datasource runs the query:

| Datasource | Where the variable sits | Default format |
| --- | --- | --- |
| Prometheus, Loki, Tempo | In a `=~` or `!~` matcher | `regex` |
| Prometheus, Loki, Tempo | Anywhere else | `raw` |
| Jaeger | Anywhere | `raw` |
| ClickHouse | Inside a quoted string | `raw` |
| ClickHouse | Next to a `.` outside quotes, as in `${database}.otel_traces` | `raw`, and the value must be a plain identifier; an explicit `${var:raw}` there follows the same rule |
| ClickHouse | Elsewhere outside quotes | `sqlstring` |
| Elasticsearch, OpenSearch | Inside a quoted string | The one value, JSON-escaped; several values need a format such as `${var:pipe}` |
| Elasticsearch, OpenSearch | Outside quotes | `"a"` for one value, `("a" OR "b")` for several |

Details of All in the `regex` format:

- It becomes `.*`, or `.+` inside the stream selector of a Loki query, because Loki refuses a selector whose every matcher can match an empty value.
- After the selector, in a label or line filter, it stays `.*` so lines without the label are kept.

Details of `allValue`:

- It sends your own text for All, and the server uses it without a format.
- On Loki, use `.+` there too, because `.*` is refused when it is the only matcher in the selector.

Details of the full list:

- It is used only when it has at most 1000 values and 64 KiB of text and every value can be substituted safely.
- Otherwise a query that uses it fails.
  The fix is to use the `regex` format, set `allValue` or narrow the variable query.

### Limits

| Limit | Value |
| --- | --- |
| Variables in a document | 20 |
| Variable name | 40 characters |
| Values a viewer may select | 100, each at most 1024 bytes |
| Values a `query` variable lists | 1000 |
| `custom` options | 200 |
| `staticOptions` | 50 |
| `interval` options | 50 |
| Ad hoc filters in `default` or `baseFilters` | 20 each |
| Ad hoc or group by `keys` | 100 |
| Group by labels | 20 |
| Datasource uid | 40 characters |
