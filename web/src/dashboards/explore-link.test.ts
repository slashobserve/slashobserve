import { describe, expect, it } from "vitest"

import { resolveDatasourceRef, type DataPanel } from "@/spec"

import {
  exploreProblemText,
  exploreTarget,
  type ExploreProblem,
} from "./explore-link"

const types: Record<string, string> = {
  loki: "loki",
  tempo: "tempo",
  ch: "clickhouse",
  prom: "prometheus",
  pg: "postgres",
  es: "elasticsearch",
}

const resolve = (ref: string) =>
  resolveDatasourceRef(ref, { ds: ["loki"], other: ["$__all"] })
const typeOf = (uid: string) => types[uid]

function panel(
  type: DataPanel["type"],
  queries: DataPanel["queries"],
  datasource?: string
): DataPanel {
  return { id: "p1", type, queries, datasource } as DataPanel
}

function opening(
  input: DataPanel,
  range = {},
  executed?: (ref: string) => string | undefined
) {
  const result = exploreTarget(input, resolve, typeOf, range, executed)
  if ("problem" in result) throw new Error(result.problem)
  return result.opening
}

describe("exploreTarget", () => {
  it("opens a logs panel on Loki in the logs explorer with the dashboard range", () => {
    expect(
      opening(
        panel("logs", [{ query: '{app="checkout"} |= "error"' }], "loki"),
        { from: "now-6h", to: "now" }
      )
    ).toEqual({
      label: "Open in Logs",
      target: {
        to: "/logs/$datasourceUid",
        params: { datasourceUid: "loki" },
        search: {
          q: '{app="checkout"} |= "error"',
          from: "now-6h",
          to: "now",
        },
      },
    })
  })

  it("opens a traces panel on Tempo in the trace search without a range when the address has none", () => {
    expect(
      opening(
        panel("traces", [{ query: "{ span.http.status_code = 500 }" }], "tempo")
      )
    ).toEqual({
      label: "Open in Traces",
      target: {
        to: "/traces/$datasourceUid",
        params: { datasourceUid: "tempo" },
        search: { q: "{ span.http.status_code = 500 }" },
      },
    })
  })

  it("opens a time series on Prometheus in the metrics explorer with the dashboard range", () => {
    expect(
      opening(panel("timeseries", [{ query: "sum(rate(up[5m]))" }], "prom"), {
        from: "now-1h",
        to: "now",
      })
    ).toEqual({
      label: "Open in Metrics",
      target: {
        to: "/explore/metrics/$datasourceUid",
        params: { datasourceUid: "prom" },
        search: { q: "sum(rate(up[5m]))", from: "now-1h", to: "now" },
      },
    })
  })

  it("opens a table panel and an instant query in table mode", () => {
    expect(
      opening(panel("table", [{ query: "up" }], "prom")).target
    ).toMatchObject({ search: { mode: "table" } })
    expect(
      opening(panel("stat", [{ query: "up", instant: true }], "prom")).target
    ).toMatchObject({ search: { mode: "table" } })
  })

  it("sends a time series on Loki to the logs explorer", () => {
    expect(
      opening(panel("timeseries", [{ query: "rate({a=1}[1m])" }], "loki"))
        .target
    ).toMatchObject({ to: "/logs/$datasourceUid" })
  })

  it("opens a logs panel whose datasource list has not loaded", () => {
    const result = exploreTarget(
      panel("logs", [{ query: "{a=1}" }], "loki"),
      resolve,
      undefined,
      {}
    )
    expect(result).toMatchObject({ opening: { label: "Open in Logs" } })
  })

  it("takes the query's own datasource over the panel's and resolves a variable reference", () => {
    expect(
      opening(
        panel("logs", [{ query: "{app=x}", datasource: "${ds}" }], "tempo")
      ).target
    ).toEqual({
      to: "/logs/$datasourceUid",
      params: { datasourceUid: "loki" },
      search: { q: "{app=x}" },
    })
  })

  it("opens the first visible query and says so when the panel has several", () => {
    const result = opening(
      panel(
        "timeseries",
        [
          { query: "hidden", hide: true },
          { ref: "B", query: "second" },
          { ref: "C", query: "third" },
        ],
        "prom"
      )
    )
    expect(result.label).toBe("Open in Metrics (first query)")
    expect(result.target).toMatchObject({ search: { q: "second" } })
  })

  it("counts only visible queries when it says the panel has several", () => {
    const result = opening(
      panel(
        "timeseries",
        [
          { ref: "A", query: "up" },
          { ref: "B", query: "hidden", hide: true },
          { ref: "C", expression: { type: "math", math: "$A * 2" } },
        ],
        "prom"
      )
    )
    expect(result.label).toBe("Open in Metrics")
  })

  it("uses the query as it ran when the saved text has variables", () => {
    const asked: string[] = []
    const result = opening(
      panel("timeseries", [{ ref: "A", query: 'up{job="$service"}' }], "prom"),
      {},
      (ref) => {
        asked.push(ref)
        return 'up{job="checkout"}'
      }
    )
    expect(asked).toEqual(["A"])
    expect(result.target).toMatchObject({ search: { q: 'up{job="checkout"}' } })
  })

  it("does not ask for the executed text when the saved query has no variables", () => {
    const result = opening(
      panel("timeseries", [{ query: "up" }], "prom"),
      {},
      () => "should not be used"
    )
    expect(result.target).toMatchObject({ search: { q: "up" } })
  })

  it("waits for the datasource list before choosing an explorer", () => {
    expect(
      exploreTarget(
        panel("timeseries", [{ query: "up" }], "prom"),
        resolve,
        undefined,
        {}
      )
    ).toEqual({ problem: "datasources-not-loaded" })
  })

  const problems: [string, DataPanel, ExploreProblem][] = [
    [
      "a datasource with no explorer",
      panel("timeseries", [{ query: "select 1" }], "pg"),
      "no-explorer",
    ],
    [
      "a datasource whose type is not known",
      panel("timeseries", [{ query: "up" }], "missing"),
      "no-explorer",
    ],
    [
      "an expression and a reused panel only",
      panel(
        "timeseries",
        [
          { ref: "A", expression: { type: "math", math: "1" } },
          { source: { panel: "p0" } },
        ],
        "prom"
      ),
      "no-query",
    ],
    [
      "a time series on Tempo",
      panel("timeseries", [{ query: "{}" }], "tempo"),
      "not-a-search",
    ],
    [
      "a time series on ClickHouse",
      panel("timeseries", [{ query: "SELECT 1" }], "ch"),
      "not-a-search",
    ],
    [
      "a table on Elasticsearch",
      panel("table", [{ query: "*" }], "es"),
      "not-a-search",
    ],
    [
      "no datasource anywhere",
      panel("logs", [{ query: "{a=1}" }]),
      "no-datasource",
    ],
    [
      "a datasource variable without a single selection",
      panel("logs", [{ query: "{a=1}" }], "${other}"),
      "no-datasource",
    ],
    [
      "a dashboard query instead of a panel query",
      panel("logs", [{ source: { panel: "p0" } }], "loki"),
      "no-query",
    ],
    [
      "only hidden queries",
      panel("logs", [{ query: "{a=1}", hide: true }], "loki"),
      "no-query",
    ],
    [
      "a $service variable in the query before the panel has loaded",
      panel("logs", [{ query: '{app="$service"}' }], "loki"),
      "uses-variables",
    ],
    [
      "a ${service} variable in the query before the panel has loaded",
      panel("logs", [{ query: '{app="${service}"}' }], "loki"),
      "uses-variables",
    ],
    [
      "a traces panel on ClickHouse",
      panel("traces", [{ query: "SELECT * FROM otel_traces" }], "ch"),
      "clickhouse-traces-query",
    ],
  ]

  it.each(problems)("reports %s", (_name, input, problem) => {
    expect(exploreTarget(input, resolve, typeOf, {})).toEqual({ problem })
  })

  it("lets a dollar sign that is not a variable through", () => {
    expect(
      opening(panel("logs", [{ query: '{app="shop"} |= "$12"' }], "loki"))
        .target
    ).toEqual({
      to: "/logs/$datasourceUid",
      params: { datasourceUid: "loki" },
      search: { q: '{app="shop"} |= "$12"' },
    })
  })

  it("does not send a ClickHouse logs query through the trace rule", () => {
    expect(
      opening(panel("logs", [{ query: "SELECT Body FROM otel_logs" }], "ch"))
        .target
    ).toEqual({
      to: "/logs/$datasourceUid",
      params: { datasourceUid: "ch" },
      search: { q: "SELECT Body FROM otel_logs" },
    })
  })
})

describe("exploreProblemText", () => {
  it.each<[ExploreProblem, string]>([
    ["no-query", "no query to open"],
    ["datasources-not-loaded", "datasources not loaded"],
    ["not-a-search", "its query is not a log or trace search"],
    ["no-explorer", "no explorer for this datasource"],
    ["no-datasource", "no datasource"],
    ["uses-variables", "variables not resolved yet"],
    ["clickhouse-traces-query", "SQL trace search is not supported"],
  ])("words %s as %s", (problem, text) => {
    expect(exploreProblemText(problem)).toBe(text)
  })
})
