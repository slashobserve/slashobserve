import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  eventTicks,
  maxEventTicks,
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

function flatFields(count: number): PreparedField[] {
  const ids = Array.from({ length: count }, (_, index) => `s${index}`)
  return [
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
  ]
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

  it("records the parent of each span, null for a root or a missing parent", () => {
    const spans = spansOf(
      model([
        field("spanID", "string", ["late", "orphan", "kid", "self"]),
        field("parentSpanID", "string", ["", "gone", "orphan", "self"]),
        field("startTime", "time", [200, 100, 150, 300]),
        field("duration", "number", [10, 10, 10, 10]),
      ])
    )
    expect(spans.map((span) => [span.id, span.parentId])).toEqual([
      ["orphan", null],
      ["kid", "orphan"],
      ["late", null],
      ["self", null],
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
    const result = model(flatFields(maxSpans + 3))
    expect(spansOf(result)).toHaveLength(maxSpans)
    if (result.reason === null) {
      expect(result.notes).toEqual(["Showing the first 5,000 of 5,003 spans."])
    }
  })

  it("keeps every span up to the cap the caller passes", () => {
    const frame: PreparedFrame = { ref: "A", fields: flatFields(6000) }
    const result = traceModel([frame], createFieldResolver(panel), {
      maxSpans: 20000,
    })
    expect(spansOf(result)).toHaveLength(6000)
    if (result.reason === null) expect(result.notes).toEqual([])
  })

  it("builds a 20,000-span tree quickly", () => {
    const count = 20000
    const ids = Array.from({ length: count }, (_, index) => `s${index}`)
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        field("spanID", "string", ids),
        field(
          "parentSpanID",
          "string",
          ids.map((_, index) => (index === 0 ? "" : `s${index % 7}`))
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
      ],
    }
    const began = performance.now()
    const result = traceModel([frame], createFieldResolver(panel), {
      maxSpans: 20000,
    })
    const took = performance.now() - began
    expect(spansOf(result)).toHaveLength(count)
    expect(took).toBeLessThan(500)
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
      { key: "a", text: "x", source: "span" },
      { key: "http", text: "GET", source: "span" },
      { key: "ok", text: "true", source: "span" },
      { key: "region", text: "eu", source: "resource" },
      { key: "z", text: "1", source: "span" },
    ])
    expect(spans[1].attributes).toEqual([
      { key: "nested", text: '{"deep":1}', source: "span" },
    ])
    expect(spans[2].attributes).toEqual([
      { key: "k", text: "false", source: "resource" },
    ])
  })

  it("tells span attributes from resource attributes by their column", () => {
    const spans = spansOf(
      model([
        field("SpanId", "string", ["a"]),
        field("Timestamp", "time", [1]),
        field("SpanAttributes", "other", [{ "http.route": "/pay" }]),
        field("ResourceAttributes", "other", [{ "service.name": "checkout" }]),
      ])
    )
    expect(spans[0].attributes).toEqual([
      { key: "http.route", text: "/pay", source: "span" },
      { key: "service.name", text: "checkout", source: "resource" },
    ])
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

describe("traceModel kind and events", () => {
  const events = JSON.stringify([
    {
      time_ms: 1862,
      name: "exception",
      attributes: { "exception.type": "UpstreamUnavailable", retry: 2 },
    },
    { time_ms: 1866, name: "retry.scheduled", attributes: {} },
  ])

  it("reads the span kind and its events from the trace endpoint columns", () => {
    const spans = spansOf(
      model([
        field("SpanId", "string", ["a"]),
        field("StartTime", "time", [1000]),
        field("SpanKind", "string", ["Client"]),
        field("SpanEvents", "string", [events]),
      ])
    )
    expect(spans[0].kind).toBe("Client")
    expect(spans[0].events).toEqual([
      {
        time: 1862,
        name: "exception",
        attributes: { "exception.type": "UpstreamUnavailable", retry: "2" },
      },
      { time: 1866, name: "retry.scheduled", attributes: {} },
    ])
  })

  it("gives a frame without the columns no kind and no events", () => {
    const spans = spansOf(model(grafana()))
    expect(spans.map((span) => [span.kind, span.events])).toEqual([
      [null, []],
      [null, []],
      [null, []],
    ])
  })

  it("gives malformed events JSON and events without a time no events", () => {
    const spans = spansOf(
      model([
        field("SpanId", "string", ["a", "b", "c"]),
        field("StartTime", "time", [1, 2, 3]),
        field("SpanEvents", "string", ["[{", '[{"name": "no time"}]', "[]"]),
      ])
    )
    expect(spans.map((span) => span.events)).toEqual([[], [], []])
  })
})

describe("traceModel links", () => {
  const links = JSON.stringify([
    {
      trace_id: "0af7651916cd43dd8448eb211c80319c",
      span_id: "b7ad6b7169203331",
      attributes: { "messaging.operation": "process", attempt: 3 },
    },
    { trace_id: "", span_id: "x" },
    { span_id: "no trace" },
    "not a link",
  ])

  it("reads the span links from the trace endpoint column", () => {
    const spans = spansOf(
      model([
        field("SpanId", "string", ["a"]),
        field("StartTime", "time", [1000]),
        field("SpanLinks", "string", [links]),
      ])
    )
    expect(spans[0].links).toEqual([
      {
        traceId: "0af7651916cd43dd8448eb211c80319c",
        spanId: "b7ad6b7169203331",
        attributes: { "messaging.operation": "process", attempt: "3" },
      },
    ])
  })

  it("gives a missing column, malformed JSON and an empty list no links", () => {
    expect(spansOf(model(grafana())).map((span) => span.links)).toEqual([
      [],
      [],
      [],
    ])
    const spans = spansOf(
      model([
        field("SpanId", "string", ["a", "b"]),
        field("StartTime", "time", [1, 2]),
        field("SpanLinks", "string", ["[{", "[]"]),
      ])
    )
    expect(spans.map((span) => span.links)).toEqual([[], []])
  })
})

describe("eventTicks", () => {
  const trace = { start: 1000, duration: 200 }
  const event = (time: number) => ({ time, name: "e", attributes: {} })

  it("places each event as a fraction of the zoomed window", () => {
    expect(
      eventTicks([event(1000), event(1100), event(1200)], trace, {
        start: 0,
        end: 1,
      })
    ).toEqual([0, 0.5, 1])
    expect(
      eventTicks([event(1050), event(1100), event(1150)], trace, {
        start: 0.5,
        end: 1,
      })
    ).toEqual([0, 0.5])
  })

  it("draws at most the cap and nothing for an empty trace", () => {
    const many = Array.from({ length: 50 }, (_, index) => event(1000 + index))
    expect(eventTicks(many, trace, { start: 0, end: 1 })).toHaveLength(
      maxEventTicks
    )
    expect(
      eventTicks(many, { start: 0, duration: 0 }, { start: 0, end: 1 })
    ).toEqual([])
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
