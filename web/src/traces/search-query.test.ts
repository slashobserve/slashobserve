import { describe, expect, it } from "vitest"

import {
  clickhouseTraceSearch,
  parseDuration,
  traceByIdParams,
  traceQLFor,
  traceSearchParams,
  type TraceForm,
} from "./search-query"

const empty: TraceForm = {
  service: "",
  span: "",
  min: "",
  max: "",
  errors: false,
}

describe("parseDuration", () => {
  it.each([
    ["250ns", 250],
    ["3us", 3_000],
    ["3µs", 3_000],
    ["250ms", 250_000_000],
    ["1.5s", 1_500_000_000],
    ["2m", 120_000_000_000],
    ["1h", 3_600_000_000_000],
  ])("reads %s as %i ns", (text, ns) => {
    expect(parseDuration(text)).toBe(ns)
  })

  it.each(["", "250", "ms", "-1s", "1 s", "1d", "1.s"])(
    "rejects %j",
    (text) => {
      expect(parseDuration(text)).toBeNull()
    }
  )
})

describe("traceQLFor", () => {
  it("matches every trace without conditions", () => {
    expect(traceQLFor(empty)).toBe("{}")
  })

  it.each([
    [{ service: "checkout" }, '{resource.service.name = "checkout"}'],
    [{ span: "GET /pay" }, '{name = "GET /pay"}'],
    [{ min: "250ms" }, "{duration >= 250ms}"],
    [{ max: "3µs" }, "{duration <= 3us}"],
    [{ errors: true }, "{status = error}"],
  ])("builds %j", (change, query) => {
    expect(traceQLFor({ ...empty, ...change })).toBe(query)
  })

  it("joins every condition in order and escapes quotes and backslashes", () => {
    expect(
      traceQLFor({
        service: 'a"b',
        span: "c\\d",
        min: "1ms",
        max: "2s",
        errors: true,
      })
    ).toBe(
      '{resource.service.name = "a\\"b" && name = "c\\\\d" && duration >= 1ms && duration <= 2s && status = error}'
    )
  })
})

describe("clickhouseTraceSearch", () => {
  it("lists the newest traces without conditions", () => {
    expect(clickhouseTraceSearch(empty))
      .toBe(`SELECT TraceId, min(Timestamp) AS StartTime, argMax(otel_traces.ServiceName, otel_traces.Duration) AS ServiceName, argMax(otel_traces.SpanName, otel_traces.Duration) AS SpanName, max(otel_traces.Duration) AS Duration, count() AS Spans, countIf(lower(StatusCode) IN ('error', 'status_code_error')) AS Errors
FROM otel_traces
WHERE $__timeFilter(Timestamp)
GROUP BY TraceId
ORDER BY StartTime DESC
LIMIT 100`)
  })

  it("adds every condition with durations in nanoseconds and escaped quotes", () => {
    expect(
      clickhouseTraceSearch({
        service: "o'neil",
        span: "pay",
        min: "250ms",
        max: "1.5s",
        errors: true,
      })
    )
      .toBe(`SELECT TraceId, min(Timestamp) AS StartTime, argMax(otel_traces.ServiceName, otel_traces.Duration) AS ServiceName, argMax(otel_traces.SpanName, otel_traces.Duration) AS SpanName, max(otel_traces.Duration) AS Duration, count() AS Spans, countIf(lower(StatusCode) IN ('error', 'status_code_error')) AS Errors
FROM otel_traces
WHERE $__timeFilter(Timestamp) AND otel_traces.ServiceName = 'o\\'neil' AND otel_traces.SpanName = 'pay' AND otel_traces.Duration >= 250000000 AND otel_traces.Duration <= 1500000000 AND lower(StatusCode) IN ('error', 'status_code_error')
GROUP BY TraceId
ORDER BY StartTime DESC
LIMIT 100`)
  })
})

describe("traceSearchParams", () => {
  it("uses the written TraceQL for Tempo over the fields", () => {
    expect(
      traceSearchParams("tempo", {
        q: "{ .http.status_code = 500 }",
        service: "x",
      })
    ).toEqual({
      query: "{ .http.status_code = 500 }",
      kind: "instant",
      from: "now-1h",
      to: "now",
      limit: 100,
    })
  })

  it("builds TraceQL from the fields when none is written", () => {
    expect(
      traceSearchParams("tempo", { service: "x", errors: "1" }).query
    ).toBe('{resource.service.name = "x" && status = error}')
  })
})

describe("traceByIdParams", () => {
  const range = { from: "now-24h", to: "now" }
  const id = "4bf92f3577b34da6a3ce929d0e0e4736"

  it("asks Tempo for the trace by id", () => {
    expect(traceByIdParams("tempo", id, range)).toEqual({
      query: id,
      kind: "instant",
      from: "now-24h",
      to: "now",
      limit: 1000,
    })
  })

  it("selects the trace's spans from ClickHouse", () => {
    expect(traceByIdParams("clickhouse", id, range)).toEqual({
      query: `SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp, Duration, StatusCode, SpanAttributes, ResourceAttributes
FROM otel_traces
WHERE $__timeFilter(Timestamp) AND TraceId = '${id}'
ORDER BY Timestamp
LIMIT 1000`,
      kind: "instant",
      from: "now-24h",
      to: "now",
      limit: 1000,
    })
  })
})
