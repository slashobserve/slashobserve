import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  maxSpans,
  missingColumnsReason,
  traceModel,
  visibleSpans,
  windowRows,
  type Span,
} from "./trace-model"

const panel = {
  id: "p1",
  title: "Panel",
  gridPos: { x: 0, y: 0, w: 12, h: 8 },
  type: "traces",
  datasource: "ch",
  queries: [{ query: "SELECT 1", format: "table" }],
} as DataPanel

function field(
  name: string,
  type: PreparedField["type"],
  values: unknown[]
): PreparedField {
  return { name, type, values }
}

function model(fields: PreparedField[]) {
  const frame: PreparedFrame = { ref: "A", fields }
  return traceModel([frame], createFieldResolver(panel))
}

function spansOf(result: ReturnType<typeof model>): Span[] {
  if (result.reason !== null) throw new Error(result.reason)
  return result.spans
}

const grafana = () => [
  field("traceID", "string", ["t1", "t1", "t1"]),
  field("spanID", "string", ["a", "b", "c"]),
  field("parentSpanID", "string", ["", "a", "a"]),
  field("operationName", "string", ["GET /", "db", "cache"]),
  field("serviceName", "string", ["web", "db", "cache"]),
  field("startTime", "time", [1000, 1010, 1060]),
  field("duration", "number", [100, 40, 20]),
]

describe("traceModel columns", () => {
  it("builds the tree from Grafana's column names", () => {
    const result = model(grafana())
    expect(result.reason).toBeNull()
    const spans = spansOf(result)
    expect(spans.map((span) => [span.id, span.depth])).toEqual([
      ["a", 0],
      ["b", 1],
      ["c", 1],
    ])
    expect(spans[1].offset).toBeCloseTo(0.1)
    expect(spans[1].width).toBeCloseTo(0.4)
    if (result.reason === null) {
      expect(result.rootService).toBe("web")
      expect(result.rootOperation).toBe("GET /")
      expect(result.start).toBe(1000)
      expect(result.duration).toBe(100)
      expect(result.traceId).toBe("t1")
      expect(result.notes).toEqual([])
    }
  })

  it("accepts OpenTelemetry names and converts a Duration column from nanoseconds", () => {
    const result = model([
      field("TraceId", "string", ["t1", "t1"]),
      field("SpanId", "string", ["a", "b"]),
      field("ParentSpanId", "string", ["", "a"]),
      field("SpanName", "string", ["root", "child"]),
      field("ServiceName", "string", ["web", "web"]),
      field("Timestamp", "time", [5000, 5010]),
      field("Duration", "number", [100_000_000, 40_000_000]),
    ])
    const spans = spansOf(result)
    expect(spans.map((span) => span.duration)).toEqual([100, 40])
    expect(spans[1].operation).toBe("child")
    if (result.reason === null)
      expect(result.formatDuration(100)).toBe("100 ms")
  })

  it("keeps a lower-case duration column in milliseconds", () => {
    const fields = grafana()
    const spans = spansOf(model(fields))
    expect(spans[0].duration).toBe(100)
  })

  it("keeps a Duration column in milliseconds when it holds fractions", () => {
    const fields = grafana().map((item) =>
      item.name === "duration"
        ? field("Duration", "number", [100, 40.5, 20])
        : item
    )
    expect(spansOf(model(fields))[1].duration).toBe(40.5)
  })

  it("returns the reason when no frame has a span id and a start time", () => {
    const result = model([
      field("message", "string", ["a"]),
      field("Timestamp", "time", [1]),
    ])
    expect(result).toEqual({ reason: missingColumnsReason })
  })
})

describe("traceModel tree", () => {
  it("makes a span with a missing parent a root, roots ordered by start", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["late", "orphan", "kid"]),
        field("parentSpanID", "string", ["", "gone", "orphan"]),
        field("startTime", "time", [200, 100, 150]),
        field("duration", "number", [10, 10, 10]),
      ])
    )
    expect(spans.map((span) => [span.id, span.depth])).toEqual([
      ["orphan", 0],
      ["kid", 1],
      ["late", 0],
    ])
  })

  it("does not lose spans whose parents form a cycle", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["a", "b"]),
        field("parentSpanID", "string", ["b", "a"]),
        field("startTime", "time", [1, 2]),
        field("duration", "number", [1, 1]),
      ])
    )
    expect(spans.map((span) => span.id)).toEqual(["a", "b"])
  })

  it("shows the first trace and notes how many there are", () => {
    const result = model([
      field("traceID", "string", ["t1", "t2", "t3"]),
      field("spanID", "string", ["a", "b", "c"]),
      field("parentSpanID", "string", ["", "", ""]),
      field("startTime", "time", [1, 2, 3]),
      field("duration", "number", [1, 1, 1]),
    ])
    expect(spansOf(result).map((span) => span.id)).toEqual(["a"])
    if (result.reason === null) {
      expect(result.traceId).toBe("t1")
      expect(result.notes).toEqual(["Showing trace 1 of 3."])
    }
  })

  it("caps the spans of a trace and says so", () => {
    const count = maxSpans + 3
    const ids = Array.from({ length: count }, (_, index) => `s${index}`)
    const result = model([
      field("spanID", "string", ids),
      field(
        "parentSpanID",
        "string",
        ids.map(() => "")
      ),
      field(
        "startTime",
        "time",
        ids.map((_, index) => index)
      ),
      field(
        "duration",
        "number",
        ids.map(() => 1)
      ),
    ])
    expect(spansOf(result)).toHaveLength(maxSpans)
    if (result.reason === null) {
      expect(result.notes).toEqual(["Showing the first 5,000 of 5,003 spans."])
    }
  })

  it("flags error spans from every status encoding", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["a", "b", "c", "d"]),
        field("parentSpanID", "string", ["", "", "", ""]),
        field("startTime", "time", [1, 2, 3, 4]),
        field("duration", "number", [1, 1, 1, 1]),
        field("statusCode", "string", [
          "STATUS_CODE_ERROR",
          "Error",
          "Ok",
          null,
        ]),
      ])
    )
    expect(spans.map((span) => span.error)).toEqual([true, true, false, false])
    const numeric = spansOf(
      model([
        field("spanID", "string", ["a", "b"]),
        field("startTime", "time", [1, 2]),
        field("statusCode", "number", [2, 0]),
      ])
    )
    expect(numeric.map((span) => span.error)).toEqual([true, false])
  })

  it("gives each service one color slot in order of appearance", () => {
    const spans = spansOf(model(grafana()))
    expect(spans.map((span) => span.slot)).toEqual([0, 1, 2])
  })
})

describe("traceModel attributes", () => {
  it("reads Grafana tag lists, objects and JSON strings as sorted text pairs", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["a", "b", "c"]),
        field("startTime", "time", [1, 2, 3]),
        field("tags", "other", [
          [
            { key: "z", value: 1 },
            { key: "a", value: "x" },
          ],
          null,
          null,
        ]),
        field("SpanAttributes", "other", [
          { http: "GET", ok: true },
          { nested: { deep: 1 } },
          null,
        ]),
        field("serviceTags", "string", [
          '{"region":"eu"}',
          "not json",
          '[{"key":"k","value":false}]',
        ]),
      ])
    )
    expect(spans[0].attributes).toEqual([
      { key: "a", text: "x" },
      { key: "http", text: "GET" },
      { key: "ok", text: "true" },
      { key: "region", text: "eu" },
      { key: "z", text: "1" },
    ])
    expect(spans[1].attributes).toEqual([{ key: "nested", text: '{"deep":1}' }])
    expect(spans[2].attributes).toEqual([{ key: "k", text: "false" }])
  })

  it("keeps markup in attributes as plain text", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["a"]),
        field("startTime", "time", [1]),
        field("tags", "other", [
          [{ key: "x", value: "<img src=x onerror=1>" }],
        ]),
      ])
    )
    expect(spans[0].attributes[0].text).toBe("<img src=x onerror=1>")
  })
})

describe("visibleSpans", () => {
  it("hides the descendants of a collapsed span", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["a", "b", "c", "d"]),
        field("parentSpanID", "string", ["", "a", "b", "a"]),
        field("startTime", "time", [1, 2, 3, 4]),
        field("duration", "number", [1, 1, 1, 1]),
      ])
    )
    const ids = (collapsed: string[]) =>
      visibleSpans(spans, new Set(collapsed)).map((span) => span.id)
    expect(ids([])).toEqual(["a", "b", "c", "d"])
    expect(ids(["b"])).toEqual(["a", "b", "d"])
    expect(ids(["a"])).toEqual(["a"])
    expect(ids(["a", "b"])).toEqual(["a"])
  })
})

describe("windowRows", () => {
  const base = {
    count: 1000,
    rowHeight: 28,
    viewport: 280,
    openIndex: -1,
    detailHeight: 0,
    overscan: 5,
  }

  it("windows plain rows by scroll offset", () => {
    expect(windowRows({ ...base, scrollTop: 0 })).toEqual({
      start: 0,
      end: 16,
      before: 0,
      after: (1000 - 16) * 28,
    })
    const middle = windowRows({ ...base, scrollTop: 2800 })
    expect(middle.start).toBe(95)
    expect(middle.end).toBe(116)
    expect(
      middle.before + (middle.end - middle.start) * 28 + middle.after
    ).toBe(1000 * 28)
  })

  it("accounts for an open row's detail above, inside and below the window", () => {
    const open = { ...base, detailHeight: 100 }
    const total = (input: ReturnType<typeof windowRows>) =>
      input.before + (input.end - input.start) * 28 + input.after
    const above = windowRows({ ...open, openIndex: 2, scrollTop: 2800 })
    expect(above.before).toBe(above.start * 28 + 100)
    expect(total(above)).toBe(1000 * 28 + 100)
    const below = windowRows({ ...open, openIndex: 900, scrollTop: 0 })
    expect(below.after).toBe((1000 - below.end) * 28 + 100)
    expect(total(below)).toBe(1000 * 28 + 100)
    const inside = windowRows({ ...open, openIndex: 5, scrollTop: 0 })
    expect(total(inside)).toBe(1000 * 28)
    expect(inside.start).toBeLessThanOrEqual(5)
    expect(inside.end).toBeGreaterThan(5)
  })
})
