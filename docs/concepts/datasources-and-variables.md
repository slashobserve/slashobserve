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

| Type         | Fields                                                                                          | Values                                                             |
| ------------ | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `query`      | `datasource`, `query`, `regex`, `sort`, `refresh`, `multi`, `includeAll`, `allValue`, `default` | The result of the query on its datasource.                         |
| `custom`     | `options`, `multi`, `includeAll`, `allValue`, `default`                                         | The listed options, up to 200.                                     |
| `constant`   | `value`                                                                                         | The one value. It has no control.                                  |
| `interval`   | `options`, `auto`, `autoSteps`, `autoMin`, `default`                                            | Durations such as `1m` or `1h`, plus `$__auto` when `auto` is set. |
| `textbox`    | `default`                                                                                       | Free text up to 200 characters.                                    |
| `datasource` | `datasourceType`, `regex`, `default`                                                            | The uids of the configured datasources of that type.               |
| `adhoc`      | `datasource`                                                                                    | Key, operator and value filters that the viewer adds.              |

A `query` variable on Prometheus uses `label_values(label)` or `label_values(selector, label)`.
On ClickHouse it is one `SELECT`, and the first column gives the values.
`regex` keeps only the values it matches, or the part of each value it captures, and `sort` is `alpha`, `alphaDesc`, `num` or `numDesc`.

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

The Datasources page and the datasource filter on the Dashboards page count a dashboard under a datasource variable's `default`.
A datasource variable with no `default` adds nothing to those counts.

## Ad hoc filters

An `adhoc` variable lets a viewer add key, operator and value filters (`=`, `!=`, `=~` and `!~`).
On Prometheus the filters are added as label matchers to every selector in every query on that datasource, and a query cannot refer to the variable.
ClickHouse filters are not applied automatically.
A ClickHouse query has to include the variable itself, as `$name` in a `WHERE` clause such as `AND $name`, which becomes the filters joined by `AND`, or `1 = 1` when there are none.
A query on a ClickHouse datasource that leaves out `$name` ignores the filters.
The Grafana importer lists the queries that a ClickHouse ad hoc variable will not filter.
