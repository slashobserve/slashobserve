// @vitest-environment node
import { describe, expect, it } from "vitest"

import {
  attributeFilter,
  clickhouseTraceSearch,
  durationField,
  keepSelectionSearch,
  parseDuration,
  traceQLAttribute,
  traceQLFor,
  jaegerSearchFor,
  traceSearchParams,
  traceSearchParamsWithCursor,
  withJaegerDurationBounds,
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

const unfilteredSql = (
  await (
    await fetch(
      new URL(
        "../../../internal/datasource/clickhouse/testdata/trace_list_unfiltered.sql",
        import.meta.url
      )
    )
  ).text()
).trimEnd()

const filteredSql = (
  await (
    await fetch(
      new URL(
        "../../../internal/datasource/clickhouse/testdata/trace_list_filtered.sql",
        import.meta.url
      )
    )
  ).text()
).trimEnd()

describe("clickhouseTraceSearch", () => {
  it("selects the per-trace start as Timestamp and leaves ordering and limits to the pager", () => {
    const query = clickhouseTraceSearch(empty)
    expect(query).toContain("min(otel_traces.Timestamp) AS Timestamp")
    expect(query).not.toMatch(/ORDER BY|LIMIT/)
  })

  it("lists the traces without conditions, the same SQL the datasource tests run", () => {
    expect(clickhouseTraceSearch(empty)).toBe(unfilteredSql)
  })

  it("reads every span of the traces that match, the same SQL the datasource tests run", () => {
    expect(
      clickhouseTraceSearch({ ...empty, service: "tl-front", min: "50ms" })
    ).toBe(filteredSql)
  })

  it("counts the matching spans next to all of them only when a condition is set", () => {
    expect(clickhouseTraceSearch(empty)).not.toContain("AS Matching")
    expect(clickhouseTraceSearch({ ...empty, service: "a" })).toContain(
      "countIf(otel_traces.ServiceName = 'a') AS Matching"
    )
  })

  it("adds every condition with durations in nanoseconds and escaped quotes", () => {
    const query = clickhouseTraceSearch({
      service: "o'neil",
      span: "pay",
      min: "250ms",
      max: "1.5s",
      errors: true,
    })
    const conditions =
      "otel_traces.ServiceName = 'o\\'neil' AND otel_traces.SpanName = 'pay' AND otel_traces.Duration >= 250000000 AND otel_traces.Duration <= 1500000000 AND lower(StatusCode) IN ('error', 'status_code_error')"
    expect(query).toContain(
      `AND TraceId IN (SELECT TraceId FROM otel_traces WHERE $__timeFilter(otel_traces.Timestamp) AND ${conditions})`
    )
    expect(query).toContain(`countIf(${conditions}) AS Matching`)
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
      pageSize: 50,
      page: 1,
    })
  })

  it("asks for the page and size in the address and never a limit", () => {
    const params = traceSearchParams("clickhouse", { page: 3, size: 250 })
    expect(params.sort).toBe("-Duration")
    expect(traceSearchParams("clickhouse", { sort: "spans" }).sort).toBe(
      "Spans"
    )
    expect(traceSearchParams("tempo", { sort: "spans" }).sort).toBeUndefined()
    expect(params).toMatchObject({ pageSize: 250, page: 3 })
    expect(params).not.toHaveProperty("limit")
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
    ["true", "span.http.response.status_code = true"],
    ["false", "span.http.response.status_code = false"],
    ["1e3", 'span.http.response.status_code = "1e3"'],
    ["12abc", 'span.http.response.status_code = "12abc"'],
    ["", 'span.http.response.status_code = ""'],
  ])("writes %j as one typed comparison", (value, written) => {
    expect(
      traceQLAttribute("span", "http.response.status_code", value, false)
    ).toBe(`{ ${written} }`)
  })

  it.each(["503", "-1.5"])(
    "matches %s as a number and as text, because the attribute may hold either",
    (value) => {
      expect(traceQLAttribute("span", "order.id", value, false)).toBe(
        `{ (span.order.id = ${value} || span.order.id = "${value}") }`
      )
      expect(traceQLAttribute("span", "order.id", value, true)).toBe(
        `{ (span.order.id != ${value} && span.order.id != "${value}") }`
      )
    }
  )

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
        "tempo",
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
        "tempo",
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
        "tempo",
        { min: "100ms", max: "5s" },
        { from, to, minMs: 800, maxMs: 2000 }
      )
    ).toMatchObject({ min: "800ms", max: "2s" })
  })

  it("appends the bounds to a written TraceQL query", () => {
    expect(
      keepSelectionSearch(
        "tempo",
        { q: "{ status = error }" },
        { from, to, minMs: 800, maxMs: null }
      )
    ).toMatchObject({ q: "{ status = error && duration >= 800ms }" })
  })

  it("is null when the written query cannot take duration bounds", () => {
    expect(
      keepSelectionSearch(
        "tempo",
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
      '(span.http.response.status_code = 503 || span.http.response.status_code = "503")',
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

describe("traceSearchParamsWithCursor", () => {
  const params = traceSearchParams("tempo", { page: 2 })

  it("adds the cursor of the page before", () => {
    expect(traceSearchParamsWithCursor(params, "abc")).toEqual({
      ...params,
      cursor: "abc",
    })
  })

  it("leaves the first page without a cursor", () => {
    expect(traceSearchParamsWithCursor(params, undefined)).toBe(params)
  })
})

describe("jaegerSearchFor", () => {
  it("writes only the set filters as key=value pairs", () => {
    expect(
      jaegerSearchFor({
        service: "checkout",
        span: "POST /pay",
        min: "250µs",
        max: "2s",
        errors: true,
      })
    ).toBe(
      'service=checkout operation="POST /pay" minDuration=250us maxDuration=2s error=true'
    )
  })

  it("quotes a value with a quote or a backslash", () => {
    expect(
      jaegerSearchFor({
        service: "",
        span: 'say "hi"',
        min: "",
        max: "",
        errors: false,
      })
    ).toBe('operation="say \\"hi\\""')
  })

  it("sends a pair that matches everything when no filter is set", () => {
    expect(
      jaegerSearchFor({
        service: "",
        span: "",
        min: "",
        max: "",
        errors: false,
      })
    ).toBe("minDuration=0s")
  })
})

describe("Jaeger searches", () => {
  const from = Date.UTC(2026, 10, 6, 14, 13)
  const to = Date.UTC(2026, 10, 6, 14, 17)

  it("sends the written search, or the one built from the fields", () => {
    expect(
      traceSearchParams("jaeger", { q: "http.status_code=500" }).query
    ).toBe("http.status_code=500")
    expect(
      traceSearchParams("jaeger", { service: "x", errors: "1" }).query
    ).toBe("service=x error=true")
    expect(traceSearchParams("jaeger", { sort: "spans" }).sort).toBeUndefined()
  })

  it("replaces the bounds of a written search with the selection's", () => {
    expect(
      withJaegerDurationBounds(
        'service=checkout minDuration=1s maxDuration="5s" http.method=GET',
        "800ms",
        "2s"
      )
    ).toBe("service=checkout http.method=GET minDuration=800ms maxDuration=2s")
    expect(withJaegerDurationBounds("minDuration=0s", "800ms", undefined)).toBe(
      "minDuration=800ms"
    )
    expect(
      keepSelectionSearch(
        "jaeger",
        { q: "service=checkout" },
        { from, to, minMs: 800, maxMs: null }
      )
    ).toMatchObject({ q: "service=checkout minDuration=800ms" })
  })
})
