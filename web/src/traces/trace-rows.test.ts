import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  countText,
  defaultTraceSort,
  durationText,
  hasErrorCounts,
  hasSpanCounts,
  nextSort,
  openTraceLabel,
  percentile,
  sortHint,
  sortTraceRows,
  traceRows,
  type TraceRow,
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

describe("durationText", () => {
  it("formats milliseconds with one decimal at most", () => {
    expect(durationText(12.54)).toBe("12.5 ms")
    expect(durationText(2410)).toBe("2,410 ms")
    expect(durationText(null)).toBe("-")
  })
})

describe("openTraceLabel", () => {
  const row: TraceRow = {
    traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
    start: null,
    service: "checkout",
    span: "POST /checkout",
    durationMs: 2410,
    spans: null,
    errors: null,
  }

  it("names the short id, the span and the duration in seconds", () => {
    expect(openTraceLabel(row)).toBe(
      "Open trace 4bf92f35, POST /checkout, 2.41 seconds"
    )
  })

  it("leaves out the span and duration it does not have", () => {
    expect(openTraceLabel({ ...row, span: "", durationMs: null })).toBe(
      "Open trace 4bf92f35"
    )
  })
})

function row(overrides: Partial<TraceRow> & { traceId: string }): TraceRow {
  return {
    start: null,
    service: "",
    span: "",
    durationMs: null,
    spans: null,
    errors: null,
    ...overrides,
  }
}

describe("sortTraceRows", () => {
  const rows = [
    row({ traceId: "a", durationMs: 5, service: "cart" }),
    row({ traceId: "b", durationMs: null, service: "" }),
    row({ traceId: "c", durationMs: 50, service: "Billing" }),
    row({ traceId: "d", durationMs: 5, service: "auth" }),
  ]
  const ids = (sorted: TraceRow[]) => sorted.map((r) => r.traceId)

  it("puts rows without a value last in both directions and keeps ties in order", () => {
    expect(
      ids(sortTraceRows(rows, { key: "duration", direction: "descending" }))
    ).toEqual(["c", "a", "d", "b"])
    expect(
      ids(sortTraceRows(rows, { key: "duration", direction: "ascending" }))
    ).toEqual(["a", "d", "c", "b"])
  })

  it("compares names case-insensitively and treats empty names as missing", () => {
    expect(
      ids(sortTraceRows(rows, { key: "service", direction: "ascending" }))
    ).toEqual(["d", "c", "a", "b"])
  })

  it("returns a copy in the original order without a sort", () => {
    const unsorted = sortTraceRows(rows, null)
    expect(ids(unsorted)).toEqual(["a", "b", "c", "d"])
    expect(unsorted).not.toBe(rows)
  })
})

describe("nextSort", () => {
  it("opens slowest first and cycles duration through descending, ascending, none", () => {
    expect(defaultTraceSort).toEqual({ key: "duration", direction: "descending" })
    const first = nextSort(null, "duration")
    expect(first).toEqual({ key: "duration", direction: "descending" })
    const second = nextSort(first, "duration")
    expect(second).toEqual({ key: "duration", direction: "ascending" })
    expect(nextSort(second, "duration")).toBeNull()
  })

  it("cycles other columns through ascending, descending, none", () => {
    const first = nextSort(defaultTraceSort, "service")
    expect(first).toEqual({ key: "service", direction: "ascending" })
    const second = nextSort(first, "service")
    expect(second).toEqual({ key: "service", direction: "descending" })
    expect(nextSort(second, "service")).toBeNull()
  })
})

describe("sortHint", () => {
  it("names the active order before the sorting hint", () => {
    expect(sortHint(defaultTraceSort)).toBe(
      "Slowest first. Click a column to sort."
    )
    expect(sortHint({ key: "start", direction: "descending" })).toBe(
      "Newest first. Click a column to sort."
    )
    expect(sortHint(null)).toBe("Click a column to sort.")
  })
})

describe("percentile", () => {
  it("uses the nearest rank", () => {
    const values = Array.from({ length: 100 }, (_, i) => i + 1)
    expect(percentile(values, 95)).toBe(95)
    expect(percentile([30, 10, 20], 50)).toBe(20)
    expect(percentile([7], 95)).toBe(7)
  })

  it("is null for no values", () => {
    expect(percentile([], 95)).toBeNull()
  })
})
