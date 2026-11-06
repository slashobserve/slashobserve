import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  countText,
  hasErrorCounts,
  hasSpanCounts,
  traceRows,
} from "./trace-rows"

describe("traceRows", () => {
  it("reads a Tempo search frame", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        {
          name: "TraceId",
          type: "string",
          values: ["4bf92f3577b34da6a3ce929d0e0e4736"],
        },
        { name: "SpanId", type: "string", values: ["a3ce929d0e0e4736"] },
        { name: "SpanName", type: "string", values: ["GET /checkout"] },
        { name: "ServiceName", type: "string", values: ["checkout"] },
        { name: "StartTime", type: "time", values: [1_760_000_000_000] },
        { name: "Duration", type: "number", values: [12_500_000] },
      ],
    }
    expect(traceRows([frame])).toEqual([
      {
        traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
        start: 1_760_000_000_000,
        service: "checkout",
        span: "GET /checkout",
        durationMs: 12.5,
        spans: null,
        errors: null,
      },
    ])
  })

  it("reads a ClickHouse frame with span counts, whatever the name case", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "traceid", type: "string", values: ["a", "b"] },
        { name: "starttime", type: "time", values: [2, 1] },
        { name: "servicename", type: "string", values: ["shop", "cart"] },
        { name: "spanname", type: "string", values: ["pay", "add"] },
        { name: "duration", type: "number", values: [1_000_000, 2_000_000] },
        { name: "Spans", type: "number", values: [7, 3] },
      ],
    }
    expect(
      traceRows([frame]).map((row) => [row.traceId, row.spans, row.durationMs])
    ).toEqual([
      ["a", 7, 1],
      ["b", 3, 2],
    ])
  })

  it("keeps rows with missing columns and skips rows without a trace id", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [{ name: "TraceId", type: "string", values: ["a", "", null] }],
    }
    expect(traceRows([frame])).toEqual([
      {
        traceId: "a",
        start: null,
        service: "",
        span: "",
        durationMs: null,
        spans: null,
        errors: null,
      },
    ])
  })

  it("ignores frames without a trace id column", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [{ name: "Value", type: "number", values: [1] }],
    }
    expect(traceRows([frame])).toEqual([])
  })

  it("reads error counts from a ClickHouse search frame", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "TraceId", type: "string", values: ["a", "b"] },
        { name: "Spans", type: "number", values: [7, 3] },
        { name: "Errors", type: "number", values: ["2", "0"] },
      ],
    }
    expect(traceRows([frame]).map((row) => [row.traceId, row.errors])).toEqual([
      ["a", 2],
      ["b", 0],
    ])
  })

  it("reads error counts from a Tempo search frame", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "TraceId", type: "string", values: ["4bf92f35", "a3ce929d"] },
        { name: "SpanName", type: "string", values: ["GET /", "POST /pay"] },
        { name: "Duration", type: "number", values: [1_000_000, 2_000_000] },
        { name: "Spans", type: "number", values: [1, 8] },
        { name: "Errors", type: "number", values: [0, 3] },
      ],
    }
    expect(
      traceRows([frame]).map((row) => [row.traceId, row.spans, row.errors])
    ).toEqual([
      ["4bf92f35", 1, 0],
      ["a3ce929d", 8, 3],
    ])
  })
})

describe("hasErrorCounts", () => {
  const row = {
    traceId: "a",
    start: null,
    service: "",
    span: "",
    durationMs: null,
    spans: null,
  }

  it("is true when every row carries an error count", () => {
    expect(
      hasErrorCounts([
        { ...row, errors: 0 },
        { ...row, errors: 3 },
      ])
    ).toBe(true)
  })

  it("is false when any row has no error count", () => {
    expect(
      hasErrorCounts([
        { ...row, errors: 0 },
        { ...row, errors: null },
      ])
    ).toBe(false)
  })

  it("is false for no rows", () => {
    expect(hasErrorCounts([])).toBe(false)
  })
})

describe("hasSpanCounts", () => {
  const row = {
    traceId: "a",
    start: null,
    service: "",
    span: "",
    durationMs: null,
    errors: null,
  }

  it("is true only when every row carries a span count", () => {
    expect(hasSpanCounts([{ ...row, spans: 4 }])).toBe(true)
    expect(
      hasSpanCounts([
        { ...row, spans: 4 },
        { ...row, spans: null },
      ])
    ).toBe(false)
    expect(hasSpanCounts([])).toBe(false)
  })
})

describe("countText", () => {
  it("counts with a singular or plural noun", () => {
    expect(countText(1, "error")).toBe("1 error")
    expect(countText(3, "error")).toBe("3 errors")
    expect(countText(1200, "span")).toBe("1,200 spans")
  })
})
