import { describe, expect, it } from "vitest"

import {
  attributeFilter,
  clickhouseTraceSearch,
  durationField,
  keepSelectionSearch,
  parseDuration,
  traceQLAttribute,
  traceQLFor,
  traceSearchParams,
  traceStatsFilter,
  withDurationBounds,
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

describe("traceStatsFilter", () => {
  it("filters Tempo spans with the TraceQL the search runs", () => {
    expect(
      traceStatsFilter("tempo", { service: "checkout", min: "250ms" })
    ).toBe('{resource.service.name = "checkout" && duration >= 250ms}')
  })

  it("prefers the written TraceQL on Tempo", () => {
    expect(
      traceStatsFilter("tempo", { q: "{ .http.status_code = 500 }", min: "1s" })
    ).toBe("{ .http.status_code = 500 }")
  })

  it("filters ClickHouse spans with a boolean expression", () => {
    expect(traceStatsFilter("clickhouse", { errors: "1" })).toBe(
      "lower(StatusCode) IN ('error', 'status_code_error')"
    )
    expect(
      traceStatsFilter("clickhouse", {
        service: "o'neil",
        span: "pay",
        min: "250ms",
        max: "1.5s",
        errors: "1",
      })
    ).toBe(
      "ServiceName = 'o\\'neil' AND SpanName = 'pay' AND Duration >= 250000000 AND Duration <= 1500000000 AND lower(StatusCode) IN ('error', 'status_code_error')"
    )
  })

  it.each([
    ["tempo", "{}"],
    ["clickhouse", ""],
  ])("matches every span on %s with an empty form", (type, filter) => {
    expect(traceStatsFilter(type, {})).toBe(filter)
  })
})

describe("traceQLAttribute", () => {
  it("matches a span attribute with a quoted value", () => {
    expect(traceQLAttribute("span", "http.request.method", "POST", false)).toBe(
      '{ span.http.request.method = "POST" }'
    )
  })

  it.each([
    ["503", "503"],
    ["-1.5", "-1.5"],
    ["true", "true"],
    ["false", "false"],
    ["1e3", '"1e3"'],
    ["12abc", '"12abc"'],
    ["", '""'],
  ])("writes %j as the typed value %s", (value, written) => {
    expect(
      traceQLAttribute("span", "http.response.status_code", value, false)
    ).toBe(`{ span.http.response.status_code = ${written} }`)
  })

  it("excludes a resource attribute", () => {
    expect(traceQLAttribute("resource", "service.name", "payments", true)).toBe(
      '{ resource.service.name != "payments" }'
    )
  })

  it("quotes keys and values that need it", () => {
    expect(traceQLAttribute("span", "my key", 'a"b\\c', false)).toBe(
      '{ span."my key" = "a\\"b\\\\c" }'
    )
  })
})

describe("durationField", () => {
  it.each([
    [250, "down", "250ms"],
    [812.34, "down", "812.3ms"],
    [812.34, "up", "812.4ms"],
    [1500, "up", "1.5s"],
    [1512.34, "down", "1.512s"],
    [1512.34, "up", "1.513s"],
    [999.96, "up", "1s"],
  ] as const)("writes %s ms rounded %s as %s", (ms, round, text) => {
    expect(durationField(ms, round)).toBe(text)
    expect(parseDuration(text)).not.toBeNull()
  })
})

describe("withDurationBounds", () => {
  it("appends both bounds before the closing brace of the first spanset", () => {
    expect(withDurationBounds("{ status = error }", "250ms", "1.5s")).toBe(
      "{ status = error && duration >= 250ms && duration <= 1.5s }"
    )
  })

  it("appends only the floor without a ceiling and leaves later spansets alone", () => {
    expect(
      withDurationBounds('{status = error} >> {name = "db"}', "3µs", undefined)
    ).toBe('{status = error && duration >= 3us} >> {name = "db"}')
  })

  it("fills an empty spanset", () => {
    expect(withDurationBounds("{}", "250ms", "2s")).toBe(
      "{duration >= 250ms && duration <= 2s}"
    )
    expect(withDurationBounds("{ }", "250ms", undefined)).toBe(
      "{duration >= 250ms}"
    )
  })

  it("is null when the query has no spanset", () => {
    expect(withDurationBounds("status = error", "250ms", undefined)).toBeNull()
    expect(
      withDurationBounds("{ status = error", "250ms", undefined)
    ).toBeNull()
  })
})

describe("keepSelectionSearch", () => {
  const from = Date.UTC(2026, 10, 6, 14, 13)
  const to = Date.UTC(2026, 10, 6, 14, 17)

  it("writes the selection's range and duration bounds into the fields", () => {
    expect(
      keepSelectionSearch(
        { service: "checkout" },
        { from, to, minMs: 812.34, maxMs: 1500 }
      )
    ).toEqual({
      from: "2026-11-06T14:13:00.000Z",
      to: "2026-11-06T14:17:00.000Z",
      min: "812.3ms",
      max: "1.5s",
      q: undefined,
    })
  })

  it("keeps the search's own bound when it is tighter and keeps a ceiling the selection lacks", () => {
    expect(
      keepSelectionSearch(
        { min: "1s", max: "5s" },
        { from, to, minMs: 800, maxMs: null }
      )
    ).toEqual({
      from: "2026-11-06T14:13:00.000Z",
      to: "2026-11-06T14:17:00.000Z",
      min: "1s",
      max: "5s",
      q: undefined,
    })
    expect(
      keepSelectionSearch(
        { min: "100ms", max: "5s" },
        { from, to, minMs: 800, maxMs: 2000 }
      )
    ).toMatchObject({ min: "800ms", max: "2s" })
  })

  it("appends the bounds to a written TraceQL query", () => {
    expect(
      keepSelectionSearch(
        { q: "{ status = error }" },
        { from, to, minMs: 800, maxMs: null }
      )
    ).toMatchObject({ q: "{ status = error && duration >= 800ms }" })
  })

  it("is null when the written query cannot take duration bounds", () => {
    expect(
      keepSelectionSearch(
        { q: "status = error" },
        { from, to, minMs: 800, maxMs: null }
      )
    ).toBeNull()
  })
})

describe("attributeFilter", () => {
  it.each([
    [
      "span.payments.provider",
      "acquirer-b",
      'span.payments.provider = "acquirer-b"',
    ],
    [
      "resource.k8s.pod.name",
      "payments-0",
      'resource.k8s.pod.name = "payments-0"',
    ],
    [
      "span.http.response.status_code",
      "503",
      "span.http.response.status_code = 503",
    ],
    ["name", "POST /checkout", 'name = "POST /checkout"'],
    ["kind", "Server", "kind = server"],
    ["status", "Error", "status = error"],
  ])(
    "appends %s to the written TraceQL on Tempo",
    (attribute, value, condition) => {
      expect(
        attributeFilter("tempo", { q: "{ status = error }" }, attribute, value)
      ).toEqual({ q: `{ status = error && ${condition} }` })
    }
  )

  it("builds the TraceQL from the fields when none is written", () => {
    expect(
      attributeFilter(
        "tempo",
        { service: "checkout", min: "250ms" },
        "span.payments.provider",
        "acquirer-b"
      )
    ).toEqual({
      q: '{resource.service.name = "checkout" && duration >= 250ms && span.payments.provider = "acquirer-b"}',
    })
  })

  it("is null on Tempo when the query has no spanset or the attribute no scope", () => {
    expect(
      attributeFilter("tempo", { q: "status = error" }, "span.x", "1")
    ).toBeNull()
    expect(attributeFilter("tempo", {}, "event.name", "x")).toBeNull()
  })

  it("maps only the span name and the service name to the ClickHouse fields", () => {
    expect(attributeFilter("clickhouse", {}, "name", "GET /pay")).toEqual({
      span: "GET /pay",
    })
    expect(
      attributeFilter("clickhouse", {}, "resource.service.name", "checkout")
    ).toEqual({ service: "checkout" })
    expect(
      attributeFilter("clickhouse", {}, "span.payments.provider", "acquirer-b")
    ).toBeNull()
  })
})
