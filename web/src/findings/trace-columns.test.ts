import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { createFieldResolver, resolveLink } from "@/panels/field-config"
import { prepareFrames } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import { traceColumnOverrides } from "./trace-columns"

const traceId = "ef177585544c8e5e1e23c6ca4d1232bb"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function datasource(uid: string, type: string): DatasourceStatus {
  return { uid, type, name: uid } as DatasourceStatus
}

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "evidence-1",
    type: "table",
    title: "Failing traces",
    datasource: "tempo-1",
    queries: [{ query: "{ status = error }" }],
    ...extra,
  } as DataPanel
}

function frame(ref: string, ...columns: [string, string][]): WireFrame {
  return {
    ref,
    fields: columns.map(([name, type]) => ({ name, type, values: [] })),
  } as unknown as WireFrame
}

const search = (ref = "A") =>
  frame(ref, ["TraceId", "string"], ["Duration", "number"])

function overridesOf(
  source: DataPanel,
  frames: WireFrame[],
  all: DatasourceStatus[]
) {
  const prepared = prepareFrames(source, frames)
  return traceColumnOverrides(
    source,
    prepared,
    all,
    createFieldResolver(source, {})
  )
}

const tempo = datasource("tempo-1", "tempo")

describe("traceColumnOverrides", () => {
  it("links the trace id column with the evidence range and gives a Tempo duration a unit", () => {
    expect(overridesOf(panel(), [search()], [tempo])).toEqual([
      {
        match: { field: "TraceId" },
        links: [
          {
            title: "Open trace",
            url: "/t/${__value.traceId}?ds=tempo-1&from=${__from}&to=${__to}",
          },
        ],
      },
      { match: { field: "Duration" }, unit: "ns" },
    ])
  })

  it("links only values that are trace ids", () => {
    const [override] = overridesOf(panel(), [search()], [tempo])
    const [link] = override.links ?? []
    const open = (value: unknown) =>
      resolveLink(link, { vars: {}, value, fromMs: 1000, toMs: 2000 })?.href
    expect(open(traceId.toUpperCase())).toBe(
      `/t/${traceId}?ds=tempo-1&from=1000&to=2000`
    )
    expect(open("")).toBeUndefined()
    expect(open("0".repeat(32))).toBeUndefined()
    expect(open("not-a-trace")).toBeUndefined()
    expect(open(null)).toBeUndefined()
  })

  it("matches trace id columns by name the way the logs page does", () => {
    const loki = datasource("loki-1", "loki")
    const found = overridesOf(
      panel({ datasource: "loki-1" }),
      [frame("A", ["trace_id", "string"], ["Duration", "number"])],
      [loki, tempo]
    )
    expect(found).toEqual([
      {
        match: { field: "trace_id" },
        links: [
          {
            title: "Open trace",
            url: "/t/${__value.traceId}?ds=tempo-1&from=${__from}&to=${__to}",
          },
        ],
      },
    ])
  })

  it("links to the ClickHouse datasource itself and gives its Duration a unit", () => {
    const found = overridesOf(
      panel({ datasource: "ch-1" }),
      [search()],
      [datasource("ch-1", "clickhouse"), tempo]
    )
    expect(found[0].links?.[0].url).toContain("?ds=ch-1&")
    expect(found[1]).toEqual({ match: { field: "Duration" }, unit: "ns" })
  })

  it("adds nothing for other panels, columns or unknown datasources", () => {
    expect(overridesOf(panel({ type: "stat" }), [search()], [tempo])).toEqual(
      []
    )
    expect(
      overridesOf(panel({ datasource: "gone" }), [search()], [tempo])
    ).toEqual([])
    expect(
      overridesOf(
        panel(),
        [frame("A", ["TraceId", "number"], ["Duration", "string"])],
        [tempo]
      )
    ).toEqual([])
  })

  it("keeps a unit or links the panel already sets", () => {
    const setUnit = panel({ unit: "ms" } as Partial<DataPanel>)
    expect(
      overridesOf(setUnit, [search()], [tempo]).map((o) => o.match)
    ).toEqual([{ field: "TraceId" }])
    const setLinks = panel({
      overrides: [
        {
          match: { field: "TraceId" },
          links: [{ title: "Mine", url: "https://example.com/${__value.raw}" }],
        },
        { match: { field: "Duration" }, unit: "s" },
      ],
    })
    expect(overridesOf(setLinks, [search()], [tempo])).toEqual([])
  })

  it("skips a column whose frames come from different datasources", () => {
    const mixed = panel({
      datasource: undefined,
      queries: [
        { ref: "A", query: "q", datasource: "tempo-1" },
        { ref: "B", query: "q", datasource: "tempo-2" },
      ],
    })
    const all = [tempo, datasource("tempo-2", "tempo")]
    expect(
      overridesOf(mixed, [search("A"), frame("B", ["TraceId", "string"])], all)
    ).toEqual([
      expect.objectContaining({ match: { field: "Duration" }, unit: "ns" }),
    ])
  })

  it("skips frames that cannot be tied to one datasource", () => {
    const mixed = panel({
      datasource: undefined,
      queries: [
        { ref: "A", query: "q", datasource: "tempo-1" },
        { ref: "B", query: "q", datasource: "loki-1" },
      ],
    })
    const all = [tempo, datasource("loki-1", "loki")]
    expect(overridesOf(mixed, [search("C")], all)).toEqual([])
  })

  it("finds columns renamed by a transform", () => {
    const renamed = panel({
      transform: [
        {
          type: "organize",
          rename: { TraceId: "trace_id", Duration: "Duration" },
        },
      ],
    })
    expect(overridesOf(renamed, [search()], [tempo])[0].match).toEqual({
      field: "trace_id",
    })
  })
})
