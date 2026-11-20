import { describe, expect, it } from "vitest"

import {
  placedPanel,
  traceByIdQuery,
  tracePanelQuery,
  tracePanelTitle,
  tracesPanelFor,
  traceSqlPanelFor,
} from "./trace-panel"

describe("tracePanelQuery", () => {
  it("uses the written TraceQL on Tempo", () => {
    expect(tracePanelQuery("tempo", { q: '{ name = "a" }' })).toBe(
      '{ name = "a" }'
    )
  })

  it("writes TraceQL from the fields on Tempo", () => {
    expect(
      tracePanelQuery("tempo", { service: "checkout", min: "1s", errors: "1" })
    ).toBe(
      '{resource.service.name = "checkout" && duration >= 1s && status = error}'
    )
  })

  it("selects the spans of the latest matching trace on ClickHouse", () => {
    const query = tracePanelQuery("clickhouse", {
      service: "checkout",
      min: "250ms",
    })
    expect(query).toContain("FROM otel_traces WHERE TraceId = (SELECT TraceId")
    expect(query).toContain("$__timeFilter(Timestamp)")
    expect(query).toContain("ServiceName = 'checkout'")
    expect(query).toContain("Duration >= 250000000")
    expect(query).toContain("ORDER BY Timestamp DESC LIMIT 1)")
    expect(query).toMatch(/ORDER BY startTime LIMIT 1000$/)
  })
})

describe("traceByIdQuery", () => {
  const id = "16880087f697ad564d0c227a6ed0efd4"

  it("is the trace id on Tempo", () => {
    expect(traceByIdQuery("tempo", id)).toBe(id)
  })

  it("selects that trace's spans on ClickHouse", () => {
    expect(traceByIdQuery("clickhouse", id)).toContain(
      `WHERE TraceId = '${id}' AND $__timeFilter(Timestamp)`
    )
  })
})

describe("tracesPanelFor", () => {
  it("is a traces panel on the datasource with the query", () => {
    expect(
      tracesPanelFor({
        id: "traces",
        title: "Traces",
        datasource: "tempo",
        type: "tempo",
        query: "{}",
        y: 0,
      })
    ).toMatchObject({
      type: "traces",
      datasource: "tempo",
      queries: [{ query: "{}" }],
    })
  })

  it("asks ClickHouse for a table", () => {
    expect(
      tracesPanelFor({
        id: "traces",
        title: "Traces",
        datasource: "ch",
        type: "clickhouse",
        query: "SELECT 1",
        y: 0,
      }).queries
    ).toEqual([{ query: "SELECT 1", format: "table" }])
  })
})

describe("tracePanelTitle", () => {
  it("names the service and span", () => {
    expect(tracePanelTitle({ service: "checkout", span: "POST /pay" })).toBe(
      "Traces: checkout POST /pay"
    )
    expect(tracePanelTitle({})).toBe("Traces")
  })
})

describe("placedPanel", () => {
  it("takes the id, title and row the dashboard gives and keeps the query", () => {
    const panel = tracesPanelFor({
      id: "traces",
      title: "Slow",
      datasource: "tempo",
      type: "tempo",
      query: "{ }",
      y: 0,
    })
    const placed = placedPanel(panel, { id: "traces-2", title: "Mine", y: 14 })
    expect(placed).toEqual({
      ...panel,
      id: "traces-2",
      title: "Mine",
      gridPos: { ...panel.gridPos, y: 14 },
    })
    expect(panel.id).toBe("traces")
  })
})

describe("traceSqlPanelFor", () => {
  it("makes a full-width table panel that keeps the SQL as written", () => {
    expect(
      traceSqlPanelFor({
        id: "p",
        title: "Trace query",
        datasource: "ch",
        query: "SELECT TraceId FROM otel_traces",
        y: 4,
      })
    ).toEqual({
      id: "p",
      type: "table",
      title: "Trace query",
      datasource: "ch",
      gridPos: { x: 0, y: 4, w: 24, h: 10 },
      queries: [{ query: "SELECT TraceId FROM otel_traces", format: "table" }],
    })
  })
})
