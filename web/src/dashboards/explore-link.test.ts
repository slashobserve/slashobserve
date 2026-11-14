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

describe("exploreTarget", () => {
  it("opens a logs panel on Loki in the logs explorer with the dashboard range", () => {
    expect(
      exploreTarget(
        panel("logs", [{ query: '{app="checkout"} |= "error"' }], "loki"),
        resolve,
        typeOf,
        { from: "now-6h", to: "now" }
      )
    ).toEqual({
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
      exploreTarget(
        panel(
          "traces",
          [{ query: "{ span.http.status_code = 500 }" }],
          "tempo"
        ),
        resolve,
        typeOf,
        {}
      )
    ).toEqual({
      target: {
        to: "/traces/$datasourceUid",
        params: { datasourceUid: "tempo" },
        search: { q: "{ span.http.status_code = 500 }" },
      },
    })
  })

  it("takes the query's own datasource over the panel's and resolves a variable reference", () => {
    expect(
      exploreTarget(
        panel("logs", [{ query: "{app=x}", datasource: "${ds}" }], "tempo"),
        resolve,
        typeOf,
        {}
      )
    ).toEqual({
      target: {
        to: "/logs/$datasourceUid",
        params: { datasourceUid: "loki" },
        search: { q: "{app=x}" },
      },
    })
  })

  const problems: [string, DataPanel, ExploreProblem][] = [
    [
      "a timeseries panel",
      panel("timeseries", [{ query: "up" }], "loki"),
      "not-a-logs-or-traces-panel",
    ],
    [
      "two queries",
      panel("logs", [{ query: "{a=1}" }, { query: "{b=2}" }], "loki"),
      "several-queries",
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
      "no-datasource",
    ],
    [
      "a $service variable in the query",
      panel("logs", [{ query: '{app="$service"}' }], "loki"),
      "uses-variables",
    ],
    [
      "a ${service} variable in the query",
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
      exploreTarget(
        panel("logs", [{ query: '{app="shop"} |= "$12"' }], "loki"),
        resolve,
        typeOf,
        {}
      )
    ).toEqual({
      target: {
        to: "/logs/$datasourceUid",
        params: { datasourceUid: "loki" },
        search: { q: '{app="shop"} |= "$12"' },
      },
    })
  })

  it("does not send a ClickHouse logs query through the trace rule", () => {
    expect(
      exploreTarget(
        panel("logs", [{ query: "SELECT Body FROM otel_logs" }], "ch"),
        resolve,
        typeOf,
        {}
      )
    ).toEqual({
      target: {
        to: "/logs/$datasourceUid",
        params: { datasourceUid: "ch" },
        search: { q: "SELECT Body FROM otel_logs" },
      },
    })
  })
})

describe("exploreProblemText", () => {
  it.each<[ExploreProblem, string]>([
    ["several-queries", "several queries"],
    ["no-datasource", "no datasource"],
    ["uses-variables", "uses variables"],
    ["clickhouse-traces-query", "SQL trace search is not supported"],
  ])("words %s as %s", (problem, text) => {
    expect(exploreProblemText(problem)).toBe(text)
  })
})
