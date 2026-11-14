import { beforeEach, describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"

import {
  lineCountText,
  logsDatasourceFor,
  matchNote,
  previewTime,
  rememberLogsDatasource,
  traceLogLines,
  traceLogsAddress,
  traceLogsWindow,
} from "./trace-logs"

const traceId = "4bf92f3577b34da6a3ce929d0e0e4736"
const spanId = "00f067aa0ba902b7"

function datasource(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "",
    dashboard_count: 0,
  }
}

const tempo = datasource("tempo", "tempo")
const loki = datasource("loki", "loki")
const clickhouse = datasource("ch", "clickhouse")
const otherLoki = datasource("loki-2", "loki")

describe("logsDatasourceFor", () => {
  beforeEach(() => localStorage.clear())

  it("returns null without a logs datasource", () => {
    expect(logsDatasourceFor("tempo", [tempo])).toBeNull()
  })

  it("returns the only logs datasource whatever its family", () => {
    expect(logsDatasourceFor("tempo", [tempo, clickhouse])).toBe(clickhouse)
  })

  it.each([
    ["tempo", "loki"],
    ["ch", "ch"],
  ])("pairs the %s trace datasource with %s", (traceUid, expected) => {
    const all = [tempo, clickhouse, loki]
    expect(logsDatasourceFor(traceUid, all)?.uid).toBe(expected)
  })

  it("falls back to the first candidate when no family matches", () => {
    expect(logsDatasourceFor("ch", [clickhouse, loki, otherLoki])).toBe(
      clickhouse
    )
    expect(logsDatasourceFor("tempo", [tempo, clickhouse])).toBe(clickhouse)
  })

  it("remembers the user's pick over the family rule", () => {
    rememberLogsDatasource("loki-2")
    expect(logsDatasourceFor("tempo", [tempo, loki, otherLoki])).toBe(otherLoki)
  })

  it("ignores a remembered pick that no longer exists", () => {
    rememberLogsDatasource("gone")
    expect(logsDatasourceFor("tempo", [tempo, clickhouse, loki])).toBe(loki)
  })
})

describe("traceLogsWindow", () => {
  it("pads the trace by a minute on each side in epoch milliseconds", () => {
    expect(traceLogsWindow(1_700_000_000_000, 2_500)).toEqual({
      from: "1699999940000",
      to: "1700000062500",
    })
  })

  it("rounds a fractional span duration outwards to whole milliseconds", () => {
    expect(traceLogsWindow(1_700_000_000_000.25, 92.51)).toEqual({
      from: "1699999940000",
      to: "1700000060093",
    })
  })
})

describe("traceLogsAddress", () => {
  const window = { from: "1", to: "2" }

  function queryOf(address: string): string | null {
    return new URLSearchParams(address.slice(address.indexOf("?"))).get("q")
  }

  it("builds the Loki address with the broadest selector", () => {
    const address = traceLogsAddress("loki", "loki", traceId, window)
    expect(address.startsWith("/logs/loki?q=")).toBe(true)
    expect(address.endsWith("&from=1&to=2")).toBe(true)
    expect(queryOf(address)).toBe(
      `{service_name=~".+"} | trace_id="${traceId}"`
    )
  })

  it("adds the span id filter on Loki", () => {
    expect(
      queryOf(traceLogsAddress("loki", "loki", traceId, window, spanId))
    ).toBe(`{service_name=~".+"} | trace_id="${traceId}" | span_id="${spanId}"`)
  })

  it("builds the ClickHouse address from the template SQL", () => {
    const q = queryOf(traceLogsAddress("ch", "clickhouse", traceId, window))
    expect(q).toContain("FROM otel_logs\n")
    expect(q).toContain(
      `WHERE $__timeFilter(Timestamp) AND TraceId = '${traceId}'\n`
    )
    expect(q).not.toContain("SpanId =")
  })

  it("adds the span id condition on ClickHouse", () => {
    const q = queryOf(
      traceLogsAddress("ch", "clickhouse", traceId, window, spanId)
    )
    expect(q).toContain(`AND TraceId = '${traceId}' AND SpanId = '${spanId}'\n`)
  })
})

describe("traceLogLines", () => {
  const stream = (times: number[], bodies: string[]): WireFrame => ({
    ref: "A",
    fields: [
      { name: "Time", type: "time", values: times },
      { name: "Body", type: "string", values: bodies },
      { name: "level", type: "string", values: bodies.map(() => "info") },
    ],
  })

  it("flattens streams oldest first and reads the Body column", () => {
    const frames = [stream([30, 10], ["c", "a"]), stream([20], ["b"])]
    expect(traceLogLines(frames)).toEqual([
      { time: 10, body: "a" },
      { time: 20, body: "b" },
      { time: 30, body: "c" },
    ])
  })

  it("falls back to the first string column without a body name", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "Timestamp", type: "time", values: [5] },
        { name: "SeverityText", type: "string", values: ["warn"] },
      ],
    }
    expect(traceLogLines([frame])).toEqual([{ time: 5, body: "warn" }])
  })

  it("returns nothing for no frames", () => {
    expect(traceLogLines([])).toEqual([])
  })
})

describe("line text", () => {
  it.each([
    [0, false, "0 lines"],
    [1, false, "1 line"],
    [3, false, "3 lines"],
    [1234, false, "1,234 lines"],
    [500, true, "500+ lines"],
  ])("counts %i lines (truncated %s) as %s", (count, truncated, text) => {
    expect(lineCountText(count, truncated)).toBe(text)
  })

  it("shows the seconds and milliseconds of a line", () => {
    expect(previewTime(Date.UTC(2026, 10, 6, 14, 19, 55, 781))).toBe("55.781")
    expect(previewTime(Date.UTC(2026, 10, 6, 14, 19, 5, 7))).toBe("05.007")
    expect(previewTime(null)).toBe("-")
  })

  it.each([
    [
      "loki",
      { service: "payments" },
      "Matched on trace_id and span_id in the payments stream.",
    ],
    [
      "loki",
      { service: "" },
      "Matched on trace_id and span_id in every stream.",
    ],
    ["loki", undefined, "Matched on trace_id in every stream."],
    [
      "clickhouse",
      { service: "payments" },
      "Matched on TraceId and SpanId in otel_logs.",
    ],
    ["clickhouse", undefined, "Matched on TraceId in otel_logs."],
  ])("explains the %s match for %o", (type, span, note) => {
    expect(matchNote(type, span)).toBe(note)
  })
})
