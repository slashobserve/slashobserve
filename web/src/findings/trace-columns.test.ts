import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import type { DataPanel } from "@/spec"

import { traceColumnOverrides, withTraceColumns } from "./trace-columns"

function datasource(uid: string, type: string): DatasourceStatus {
  return { uid, type, name: uid } as DatasourceStatus
}

function panel(uid: string, overrides?: DataPanel["overrides"]): DataPanel {
  return {
    id: "evidence-1",
    type: "table",
    title: "Failing traces",
    datasource: uid,
    queries: [{ query: "{ status = error }" }],
    overrides,
  } as DataPanel
}

function frame(...columns: [string, string][]): WireFrame {
  return {
    ref: "A",
    fields: columns.map(([name, type]) => ({ name, type, values: [] })),
  } as unknown as WireFrame
}

const tempoSearch = frame(
  ["TraceId", "string"],
  ["SpanId", "string"],
  ["Duration", "number"]
)

describe("traceColumnOverrides", () => {
  it("links the trace id column and gives a Tempo duration a unit", () => {
    const overrides = traceColumnOverrides(
      panel("tempo-1"),
      [tempoSearch],
      [datasource("tempo-1", "tempo")]
    )
    expect(overrides).toEqual([
      {
        match: { field: "TraceId" },
        links: [{ title: "Open trace", url: "/t/${__value.raw}?ds=tempo-1" }],
      },
      { match: { field: "Duration" }, unit: "ns" },
    ])
  })

  it("matches trace id columns by name the way the logs page does", () => {
    const overrides = traceColumnOverrides(
      panel("loki-1"),
      [frame(["trace_id", "string"], ["Duration", "number"])],
      [datasource("loki-1", "loki"), datasource("tempo-1", "tempo")]
    )
    expect(overrides).toEqual([
      {
        match: { field: "trace_id" },
        links: [{ title: "Open trace", url: "/t/${__value.raw}?ds=tempo-1" }],
      },
    ])
  })

  it("links to the ClickHouse datasource itself", () => {
    const overrides = traceColumnOverrides(
      panel("ch-1"),
      [frame(["TraceId", "string"])],
      [datasource("ch-1", "clickhouse"), datasource("tempo-1", "tempo")]
    )
    expect(overrides[0].links?.[0].url).toBe("/t/${__value.raw}?ds=ch-1")
  })

  it("adds nothing without a trace backend or a known datasource", () => {
    const frames = [tempoSearch]
    expect(
      traceColumnOverrides(panel("loki-1"), frames, [
        datasource("loki-1", "loki"),
      ])
    ).toEqual([])
    expect(
      traceColumnOverrides(panel("gone"), frames, [
        datasource("tempo-1", "tempo"),
      ])
    ).toEqual([])
  })

  it("leaves panels that are not tables alone", () => {
    const stat = { ...panel("tempo-1"), type: "stat" } as DataPanel
    expect(
      traceColumnOverrides(
        stat,
        [tempoSearch],
        [datasource("tempo-1", "tempo")]
      )
    ).toEqual([])
  })
})

describe("withTraceColumns", () => {
  it("puts its overrides before the panel's own so the panel's win", () => {
    const own = [{ match: { field: "Duration" }, unit: "ms" }]
    const shown = withTraceColumns(
      panel("tempo-1", own as DataPanel["overrides"]),
      [tempoSearch],
      [datasource("tempo-1", "tempo")]
    )
    expect(shown.overrides?.at(-1)).toEqual(own[0])
    expect(shown.overrides).toHaveLength(3)
  })

  it("returns the same panel when nothing applies", () => {
    const original = panel("loki-1")
    expect(
      withTraceColumns(original, [tempoSearch], [datasource("loki-1", "loki")])
    ).toBe(original)
  })
})
