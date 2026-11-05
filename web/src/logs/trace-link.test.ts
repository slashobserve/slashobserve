import { describe, expect, it } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"
import type { LogEntry } from "@/panels/logs/logs-model"

import { traceDatasourceFor, traceIdOf, traceRangeAround } from "./trace-link"

const id32 = "4BF92F3577B34DA6A3CE929D0E0E4736"

function entryWithLabel(name: string, text: string): LogEntry {
  return {
    key: "k",
    time: 1,
    level: null,
    message: "m",
    labels: {},
    repeats: 1,
    details: [{ name, text }],
  }
}

function entryWithField(name: string, value: unknown): LogEntry {
  const field = { name, type: "string" as const, values: [null, value] }
  return {
    key: "k",
    time: 1,
    level: null,
    message: "m",
    labels: {},
    repeats: 1,
    details: [{ name, field, frame: { ref: "A", fields: [field] }, index: 1 }],
  }
}

describe("traceIdOf", () => {
  it.each(["TraceId", "traceID", "trace_id", "traceId"])(
    "reads the %s label",
    (name) => {
      expect(traceIdOf(entryWithLabel(name, id32))).toBe(id32.toLowerCase())
    }
  )

  it("reads a ClickHouse TraceId field", () => {
    expect(traceIdOf(entryWithField("TraceId", ` ${id32} `))).toBe(
      id32.toLowerCase()
    )
  })

  it("accepts a 16 character id", () => {
    expect(traceIdOf(entryWithLabel("trace_id", "a3ce929d0e0e4736"))).toBe(
      "a3ce929d0e0e4736"
    )
  })

  it.each([
    ["all zeros", "0".repeat(32)],
    ["not hex", "z".repeat(32)],
    ["the wrong length", "abc123"],
    ["empty", ""],
  ])("rejects an id that is %s", (_, text) => {
    expect(traceIdOf(entryWithLabel("trace_id", text))).toBeNull()
  })

  it("ignores other names", () => {
    expect(traceIdOf(entryWithLabel("span_id", id32))).toBeNull()
  })
})

function datasource(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-06T00:00:00Z",
    dashboard_count: 0,
  }
}

describe("traceDatasourceFor", () => {
  const all = [
    datasource("loki", "loki"),
    datasource("tempo-a", "tempo"),
    datasource("tempo-b", "tempo"),
    datasource("ch", "clickhouse"),
  ]

  it("opens a ClickHouse line in the same ClickHouse", () => {
    expect(traceDatasourceFor(all[3], all)).toBe("ch")
  })

  it("opens a Loki line in the first Tempo", () => {
    expect(traceDatasourceFor(all[0], all)).toBe("tempo-a")
  })

  it("gives null for Loki without a Tempo", () => {
    expect(traceDatasourceFor(all[0], all.slice(0, 1))).toBeNull()
  })
})

describe("traceRangeAround", () => {
  it("spans an hour on each side", () => {
    expect(traceRangeAround(1_760_000_000_000)).toEqual({
      from: "1759996400000",
      to: "1760003600000",
    })
  })
})
