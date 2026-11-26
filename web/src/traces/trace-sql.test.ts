import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import { zonedDateTimeMs } from "@/lib/time"

import {
  frameRows,
  readableCell,
  traceIdField,
  traceIdOf,
  traceSqlParams,
  traceSqlSearch,
  traceSqlStart,
} from "./trace-sql"

function frame(names: string[], values: unknown[][] = []): WireFrame {
  return {
    ref: "A",
    name: "",
    labels: {},
    fields: names.map((name, index) => ({
      name,
      type: "string",
      values: values[index] ?? [],
    })),
  } as WireFrame
}

describe("traceIdField", () => {
  it("finds the trace id column whatever its case or underscore", () => {
    expect(traceIdField(frame(["ServiceName", "TraceId"]))).toBe("TraceId")
    expect(traceIdField(frame(["trace_id"]))).toBe("trace_id")
    expect(traceIdField(frame(["TRACEID"]))).toBe("TRACEID")
  })

  it("finds none in a result without one", () => {
    expect(traceIdField(frame(["ServiceName", "SpanId"]))).toBeUndefined()
  })
})

describe("traceIdOf", () => {
  it("accepts a hex trace id and lowers its case", () => {
    expect(traceIdOf("0AF7651916CD43DD8448EB211C80319C")).toBe(
      "0af7651916cd43dd8448eb211c80319c"
    )
  })

  it("refuses anything that is not a trace id", () => {
    expect(traceIdOf("checkout")).toBeNull()
    expect(traceIdOf(42)).toBeNull()
    expect(traceIdOf(null)).toBeNull()
  })
})

describe("traceSqlStart", () => {
  it("starts from the SQL that ran", () => {
    expect(traceSqlStart({ q: "SELECT 1" })).toBe("SELECT 1")
  })

  it("writes the search of the fields as SQL when none ran", () => {
    const sql = traceSqlStart({ service: "checkout" })
    expect(sql).toContain("FROM otel_traces")
    expect(sql).toContain("$__timeFilter(otel_traces.Timestamp)")
    expect(sql).toContain("'checkout'")
  })
})

describe("traceSqlSearch", () => {
  it("keeps only the SQL and clears the fields, the sort and the question", () => {
    expect(traceSqlSearch("SELECT 1")).toEqual({
      q: "SELECT 1",
      service: undefined,
      span: undefined,
      min: undefined,
      max: undefined,
      errors: undefined,
      sort: undefined,
      ask: undefined,
    })
  })
})

describe("traceSqlParams", () => {
  it("runs the SQL as an instant query over the range and page", () => {
    expect(
      traceSqlParams("SELECT 1", { from: "now-6h", page: 3, size: 100 })
    ).toEqual({
      query: "SELECT 1",
      kind: "instant",
      from: "now-6h",
      to: "now",
      pageSize: 100,
      page: 3,
      asWritten: true,
    })
  })

  it("sends no sort, so the SQL decides the order", () => {
    expect(
      traceSqlParams("SELECT 1", { sort: "duration:desc" })
    ).not.toHaveProperty("sort")
  })
})

describe("frameRows", () => {
  it("counts the longest column", () => {
    expect(frameRows(frame(["a", "b"], [[1], [1, 2, 3]]))).toBe(3)
    expect(frameRows(undefined)).toBe(0)
  })
})

describe("readableCell", () => {
  it("shows a time field as a local time and keeps the epoch value", () => {
    const at = Date.UTC(2026, 10, 20, 12, 30, 15, 832)
    const cell = readableCell("StartTime", "time", at)
    expect(cell?.text).toBe(zonedDateTimeMs(at, undefined))
    expect(cell?.raw).toBe(`${at} ms`)
  })

  it("shows a Duration number as a human unit and keeps the nanoseconds", () => {
    expect(readableCell("Duration", "number", 17_277_411)).toEqual({
      text: "17.28 ms",
      raw: "17277411 ns",
    })
    expect(readableCell("duration", "number", 2_500_000_000)?.text).toBe(
      "2.5 s"
    )
  })

  it("leaves other numbers, strings and gaps as they are", () => {
    expect(readableCell("Spans", "number", 2)).toBeNull()
    expect(readableCell("Duration", "string", "17277411")).toBeNull()
    expect(readableCell("Timestamp", "time", null)).toBeNull()
  })
})
