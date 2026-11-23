---
title: Datasources and variables
description: How a dashboard names its datasources, the eight variable types, and how multi-value, All and datasource variables work.
---

A dashboard names its datasources by `uid` and lets viewers change what its queries use through variables.
Both are fields of the [ObserveSpec](observespec.md) document.

## Datasources

The `uid` of an entry in the [datasources file](../reference/datasources-file.md), or of a datasource an Admin [added in the app](../connect/datasources-in-the-app.md), is the name a dashboard uses.
A uid is up to 40 letters, digits, `_` or `-`, starting with a letter or digit.
The entry's `type`, `prometheus`, `clickhouse`, `loki`, `tempo`, `jaeger`, `elasticsearch` or `opensearch`, decides the query language and which macros a query may use.

A data panel sets `datasource`, and any of its queries can set its own `datasource` to use a different one.
A `query` variable and an `adhoc` variable set `datasource` too.
A save is rejected with `unknown_datasource` when a uid names no configured datasource, so renaming or removing a uid breaks the next save of every dashboard that uses it.

`list_datasources` over MCP and the agent, and `GET /api/v1/datasources`, return each uid with its type, health and `source`, and an unhealthy one carries `message` and `detail`, the cause the server knows, such as the HTTP status the datasource answered.
`recheck_datasource` over MCP, `POST /api/v1/datasources/{uid}/recheck` and `observe datasources recheck` run a fresh check instead of reading the 10 second cache; checks of one datasource are at least 2 seconds apart, so a retry loop cannot hammer it.
On the Datasources page an Admin sees the same reason on each row and a Recheck button.
The agent is told which datasources are healthy before it starts.

## Variables

A variable has a `name`, which starts with a letter and continues with letters, digits or `_` up to 40 characters, and the optional `label`, `description`, `hide` and `skipUrlSync`.
`hide: label` drops the name and `hide: variable` drops the control, and neither drops the selection.
Variable names are unique, and a document has at most 20 variables.
`skipUrlSync: true` keeps the variable out of the address: its selection is not read from `var-<name>` and not written to it, so a shared link does not carry it.

A query refers to a variable as `$name`, `${name}` or `${name:format}`.
Each type has its own fields:

| Type         | Fields                                                                                                                                              | Values                                                             |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `query`      | `datasource`, `query`, `regex`, `sort`, `refresh`, `staticOptions`, `staticOrder`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default` | The result of the query on its datasource.                         |
| `custom`     | `options`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default`                                                                         | The listed options, up to 200.                                     |
| `constant`   | `value`                                                                                                                                             | The one value. It has no control.                                  |
| `interval`   | `options`, `auto`, `autoSteps`, `autoMin`, `default`                                                                                                | Durations such as `1m` or `1h`, plus `$__auto` when `auto` is set. |
| `textbox`    | `default`                                                                                                                                           | Free text up to 200 characters.                                    |
| `datasource` | `datasourceType`, `regex`, `default`                                                                                                                | The uids of the configured datasources of that type.               |
| `adhoc`      | `datasource`, `applyToAll`, `default`, `baseFilters`, `keys`, `listedValuesOnly`                                                                    | Key, operator and value filters that the viewer adds.              |
| `groupby`    | `datasource`, `default`, `keys`                                                                                                                     | Labels the viewer picks to group by.                               |

A `query` variable on Prometheus uses `label_values(label)` or `label_values(selector, label)`.
On ClickHouse it is one `SELECT`, and the first column gives the values.
On Loki it is `label_values(label)` or `label_values({selector}, label)`, and on Elasticsearch and OpenSearch `label_values(field)`, as the [Loki](../connect/loki.md#variables-and-discovery) and [Elasticsearch](../connect/elasticsearch.md#variables-and-discovery) pages describe; Tempo and Jaeger have no query variables.
`regex` keeps only the values it matches, or the part of each value it captures; named groups `text` and `value` set what the picker shows and what is selected, a `text` group alone keeps the whole value, and `sort` (which orders by the shown text) is one of:

- `alpha` and `alphaDesc` compare by character, so every capital letter sorts before every lowercase letter.
- `alphaCaseInsensitive` and `alphaCaseInsensitiveDesc` compare by character after lowercasing, so `a` comes before `B`; values that differ only in case keep a fixed order, capitals first.
- `num` and `numDesc` compare by the first number in each value, and values with no number come first in `num`.
- `natural` and `naturalDesc` compare digit runs as numbers and other text by character, so `a9` comes before `a10`; with equal numbers, the longer run comes first, so `007` comes before `7`.

`refresh: "never"` on a `query` variable runs no query and offers only its `staticOptions`.
`refresh: "load"`, which is the default, runs the query when the dashboard opens, and again when a variable it depends on changes, but not when the time range changes: the values keep the time range the dashboard opened with.
`refresh: "timeRange"` also runs the query again whenever the time range changes.
`staticOptions` (up to 50 `text` and `value` pairs) are offered next to the query results, before them by default, or after them or merged and sorted with `sort` when `staticOrder` is `after` or `sorted`; a value that the query also returns appears once.

A `custom` variable's `options` are strings, or `{ "text": "Production", "value": "prod" }` objects that show `text` in the picker while the selection, the URL and the queries use `value`.
The values list of a `query` variable carries the same texts when its regex captures them.

`allowCustomValue: true` on a `query` or `custom` variable lets the viewer enter a value that is not in its list.
A `custom` variable refuses a value outside its `options` unless it sets `allowCustomValue`, and a `query` variable accepts any value either way.

Variables are resolved in the order they are declared.
A `query` variable's query can use only variables declared before it, so a `service` variable can feed a `route` variable but not the reverse, and it cannot use an `adhoc` variable.
Panel queries can use any variable.
When a viewer changes a variable, the variables whose queries use it reload their values.

The value in the URL is `var-<name>`, so a link keeps a viewer's selection.
Without a selection a variable starts at its `default`.
With no `default`, a `query` or `custom` variable starts at All if it sets `includeAll` and at its first value if not, an `interval` variable at `$__auto` if it sets `auto` and at its first option if not, a `textbox` variable at its text or empty, and a `datasource` variable at the first matching datasource.

## Multi-value and All

`multi: true` on a `query` or `custom` variable lets the viewer pick several values, and `includeAll: true` adds All.
A `default` can be one value, an array of values for a `multi` variable, or `"$__all"` for All, which needs `includeAll` and is given alone.
A `custom` or `interval` default has to be one of its options.

How a selection reaches the query depends on its format:

| Format        | Result for `a` and `b`                                     |
| ------------- | ---------------------------------------------------------- |
| `regex`       | `(a\|b)`, with regular expression characters escaped.      |
| `pipe`        | `a\|b`                                                     |
| `csv`         | `a,b`                                                      |
| `singlequote` | `'a','b'`                                                  |
| `doublequote` | `"a","b"`                                                  |
| `sqlstring`   | `'a','b'`, with SQL escaping.                              |
| `raw`         | The value itself. It allows one value and refuses several. |

When a reference gives no format, the server picks one from where the variable sits and which datasource runs the query:

| Datasource                | Where the variable sits   | Default format                                                                  |
| ------------------------- | ------------------------- | ------------------------------------------------------------------------------- |
| Prometheus, Loki, Tempo   | In a `=~` or `!~` matcher | `regex`                                                                         |
| Prometheus, Loki, Tempo   | Anywhere else             | `raw`                                                                           |
| Jaeger                    | Anywhere                  | `raw`                                                                           |
| ClickHouse                | Inside a quoted string    | `raw`                                                                           |
| ClickHouse                | Outside quotes            | `sqlstring`                                                                     |
| Elasticsearch, OpenSearch | Inside a quoted string    | The one value, JSON-escaped; several values need a format such as `${var:pipe}` |
| Elasticsearch, OpenSearch | Outside quotes            | `"a"` for one value, `("a" or "b")` for several                                 |

So `pod=~"$pod"` in PromQL, `{app=~"$app"}` in LogQL and `{ resource.service.name =~ "$service" }` in TraceQL work for one value, several values and All.
In SQL, `ServiceName IN ($service)` works for several values.
In a search query string, `service.name:$service` matches any of the selected values.

All becomes the full list of values, formatted the same way, except in the `regex` format, where it becomes `.*`, or `.+` inside the stream selector of a Loki query because Loki refuses a selector whose every matcher can match an empty value; after the selector, in a label or line filter, it stays `.*` so lines without the label are kept.
Set `allValue` to send your own text for All instead, such as `.*`, and the server uses it without a format; on Loki, use `.+` there too, because `.*` is refused when it is the only matcher in the selector.
The full list is used only when it has at most 1000 values and 64 KiB of text and every value can be substituted safely.
Otherwise a query that uses it fails, and the fix is to use the `regex` format, set `allValue` or narrow the variable query.

## Datasource variables

A `datasource` variable lets a viewer switch a whole dashboard between datasources of one type, such as a production and a staging Prometheus.
`datasourceType` is `prometheus`, `clickhouse`, `loki`, `tempo`, `elasticsearch` or `opensearch`, the optional `regex` keeps only the uids it matches, and the optional `default` is a uid, never a `${name}`.
A viewer picks exactly one datasource, so it has no `multi` or `includeAll`.

To use it, write `${name}` as the `datasource` of a panel, of a query, of a `query` variable or of an `adhoc` variable.
The form is exactly `${name}`, so `$name` and `${name:format}` are not datasource references.
The query then runs on the selected datasource, and its macros follow the variable's `datasourceType`.
The variable lists only datasources of its `datasourceType`, so the same query text runs on whichever one the viewer picks.

The ordering rule is that a `query` or `adhoc` variable can use `${name}` only when the `datasource` variable is declared before it, and a panel or panel query can use any `datasource` variable.
A `${name}` that names no variable is rejected as `unknown_variable`, one that names another type of variable as `invalid_reference`, and one that is declared later as `variable_order`.

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

The Datasources page and the datasource filter on the Dashboards page count a dashboard under a datasource variable's `default`.
A datasource variable with no `default` adds nothing to those counts.

## Interval variables as a minimum step

An `interval` variable can also set how fine a panel's queries are.
Write `${name}` as the `interval` of a panel or of a query, and the query's step is at least the step selected in that variable, with `$__auto` choosing a step from the time range and the variable's `autoSteps` and `autoMin`.
The form is exactly `${name}`, and the variable must be an `interval` variable: an undeclared name is rejected as `unknown_variable` and another type as `invalid_reference`.
A query's own `interval` wins over the panel's.
`$__interval` in the query follows the step, so a coarser step also widens it.

A query can also set `intervalFactor`, a whole number from 2 to 10, to multiply its step after the minimum is applied.
An instant query passes the selected step on as its minimum interval, and the factor does not apply to it.

## Ad hoc filters

An `adhoc` variable lets a viewer add key, operator and value filters (`=`, `!=`, `=~` and `!~`, plus `<` and `>` on ClickHouse, Elasticsearch and OpenSearch).
The filters apply to queries on Prometheus, ClickHouse, Elasticsearch and OpenSearch datasources; a Loki, Tempo or Jaeger query takes no ad hoc filters.
On Prometheus the filters are added as label matchers to every selector in every query on that datasource, and a query cannot refer to the variable.
A ClickHouse query uses the variable explicitly, as `$name` in a `WHERE` clause such as `AND $name`, which becomes the filters joined by `AND` (a filter that starts an `or` group opens a new group, so `a, b, or c` is `(a AND b) OR c`), or `1 = 1` when there are none.
`<` and `>` compare as numbers when both sides are plain numbers and as text otherwise.
Prometheus can only AND label matchers, so a Prometheus variable that uses `<`, `>` or `or` makes its queries fail with an error that names it.

On Elasticsearch and OpenSearch every ad hoc variable filters every query on its datasource, and filters joined with `or` become a `should` group; the [Elasticsearch](../connect/elasticsearch.md#ad-hoc-filters) page describes the clauses.

A ClickHouse variable with `applyToAll` also filters every query on its datasource that leaves out `$name`, through `additional_table_filters` on the tables the query reads, and each table gets only the filters whose key is one of its columns.
A query on a ClickHouse datasource that leaves out `$name` ignores the filters when the variable does not set `applyToAll`.
The Grafana importer sets `applyToAll` on every ClickHouse ad hoc variable.

A `groupby` variable (Prometheus only) lets a viewer pick labels, up to 20, and adds them to the grouping of every aggregation in every query on its datasource, as Grafana does, so the queries need no change.
Its `default` lists the labels selected from the start, and a query that writes `$name` gets the labels there and no automatic grouping.
An aggregation written with `without` keeps a picked label by dropping it from its `without` list.

An `adhoc` variable can start with filters: `default` is up to 20 objects, each with a `key` (1 to 200 characters), an `operator` (`=`, `!=`, `=~`, `!~`, `<` or `>`), a `value` (up to 500 characters, possibly empty), an optional `condition` (`and` or `or`) and optional chip texts `keyLabel` (up to 200 characters) and `valueLabel` (up to 500), which change only what the chip shows.
Without a `var-<name>` parameter the dashboard uses these filters, and a link that sets the parameter replaces them.
`var-<name>=` with nothing after the equals sign means no filters, which is how a viewer clears the saved ones.
A filter with `condition: "or"` is written in the URL with a leading `|`, as `var-<name>=|key|op|value`.
`baseFilters` take the same objects (up to 20) and are always applied, never shown and never in the URL, as one more group that the selected filters cannot loosen.
An operator outside the six is a schema error when the Spec is saved, and a key that is not a label name (Prometheus) or a column name (ClickHouse) fails the save checks.

An `adhoc` variable can set `keys` (up to 100), which the filter picker offers instead of asking the datasource.
`listedValuesOnly: true` makes the picker accept a filter only when its key and value come from the lists it shows, and the value list comes from Prometheus label values.
A `groupby` variable takes `keys` (up to 100 label names) for its label picker in the same way.
