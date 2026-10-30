---
title: Datasources and variables
description: How a dashboard names its datasources, the seven variable types, and how multi-value, All and datasource variables work.
---

A dashboard names its datasources by `uid` and lets viewers change what its queries use through variables.
Both are fields of the [ObserveSpec](observespec.md) document.

## Datasources

The `uid` of an entry in the [datasources file](../reference/datasources-file.md) is the name a dashboard uses.
A uid is up to 40 letters, digits, `_` or `-`, starting with a letter or digit.
The entry's `type`, `prometheus` or `clickhouse`, decides the query language and which macros a query may use.

A data panel sets `datasource`, and any of its queries can set its own `datasource` to use a different one.
A `query` variable and an `adhoc` variable set `datasource` too.
A save is rejected with `unknown_datasource` when a uid is not in the file, so renaming or removing a uid breaks the next save of every dashboard that uses it.

`list_datasources` over MCP and the agent, and `GET /api/v1/datasources`, return each uid with its type and health.
The agent is told which datasources are healthy before it starts.

## Variables

A variable has a `name`, which starts with a letter and continues with letters, digits or `_` up to 40 characters, and the optional `label`, `description` and `hide`.
`hide: label` drops the name and `hide: variable` drops the control, and neither drops the selection.
Variable names are unique, and a document has at most 20 variables.

A query refers to a variable as `$name`, `${name}` or `${name:format}`.
Each type has its own fields:

| Type         | Fields                                                                                                              | Values                                                             |
| ------------ | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `query`      | `datasource`, `query`, `regex`, `sort`, `refresh`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default` | The result of the query on its datasource.                         |
| `custom`     | `options`, `multi`, `includeAll`, `allValue`, `allowCustomValue`, `default`                                         | The listed options, up to 200.                                     |
| `constant`   | `value`                                                                                                             | The one value. It has no control.                                  |
| `interval`   | `options`, `auto`, `autoSteps`, `autoMin`, `default`                                                                | Durations such as `1m` or `1h`, plus `$__auto` when `auto` is set. |
| `textbox`    | `default`                                                                                                           | Free text up to 200 characters.                                    |
| `datasource` | `datasourceType`, `regex`, `default`                                                                                | The uids of the configured datasources of that type.               |
| `adhoc`      | `datasource`, `default`                                                                                             | Key, operator and value filters that the viewer adds.              |

A `query` variable on Prometheus uses `label_values(label)` or `label_values(selector, label)`.
On ClickHouse it is one `SELECT`, and the first column gives the values.
`regex` keeps only the values it matches, or the part of each value it captures, and `sort` is one of:

- `alpha` and `alphaDesc` compare by character, so every capital letter sorts before every lowercase letter.
- `alphaCaseInsensitive` and `alphaCaseInsensitiveDesc` compare by character after lowercasing, so `a` comes before `B`; values that differ only in case keep a fixed order, capitals first.
- `num` and `numDesc` compare by the first number in each value, and values with no number come first in `num`.
- `natural` and `naturalDesc` compare digit runs as numbers and other text by character, so `a9` comes before `a10`; with equal numbers, the longer run comes first, so `007` comes before `7`.

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

When a reference gives no format, the server picks one from where the variable sits.
In a PromQL `=~` or `!~` matcher it is `regex`, so `pod=~"$pod"` works for one value, several values and All.
Elsewhere in PromQL, and inside a quoted string in SQL, it is `raw`.
In SQL outside quotes it is `sqlstring`, so `ServiceName IN ($service)` works for several values.

All becomes the full list of values, formatted the same way, except in the `regex` format, where it becomes `.*`.
Set `allValue` to send your own text for All instead, such as `.*`, and the server uses it without a format.
The full list is used only when it has at most 1000 values and 64 KiB of text and every value can be substituted safely.
Otherwise a query that uses it fails, and the fix is to use the `regex` format, set `allValue` or narrow the variable query.

## Datasource variables

A `datasource` variable lets a viewer switch a whole dashboard between datasources of one type, such as a production and a staging Prometheus.
`datasourceType` is `prometheus` or `clickhouse`, the optional `regex` keeps only the uids it matches, and the optional `default` is a uid, never a `${name}`.
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

An `adhoc` variable lets a viewer add key, operator and value filters (`=`, `!=`, `=~` and `!~`).
On Prometheus the filters are added as label matchers to every selector in every query on that datasource, and a query cannot refer to the variable.
A ClickHouse query uses the variable explicitly, as `$name` in a `WHERE` clause such as `AND $name`, which becomes the filters joined by `AND`, or `1 = 1` when there are none.
A ClickHouse variable with `applyToAll` also filters every query on its datasource that leaves out `$name`, through `additional_table_filters` on the tables the query reads, and each table gets only the filters whose key is one of its columns.
A query on a ClickHouse datasource that leaves out `$name` ignores the filters when the variable does not set `applyToAll`.
The Grafana importer sets `applyToAll` on every ClickHouse ad hoc variable.

An `adhoc` variable can start with filters: `default` is up to 20 objects, each with a `key` (1 to 200 characters), an `operator` (`=`, `!=`, `=~` or `!~`) and a `value` (up to 500 characters, possibly empty).
Without a `var-<name>` parameter the dashboard uses these filters, and a link that sets the parameter replaces them.
`var-<name>=` with nothing after the equals sign means no filters, which is how a viewer clears the saved ones.
An operator outside the four is a schema error when the Spec is saved, and a key that is not a label name (Prometheus) or a column name (ClickHouse) fails the save checks.
