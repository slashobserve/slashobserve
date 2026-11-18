import { describe, expect, it } from "vitest"

import { matcherInsert, metricInsert } from "./insert-text"

describe("metricInsert", () => {
  it("inserts a Prometheus metric name as written", () => {
    expect(metricInsert("prometheus", "http_requests_total")).toBe(
      "http_requests_total"
    )
  })

  it("treats an unknown type like Prometheus", () => {
    expect(metricInsert(undefined, "up")).toBe("up")
    expect(metricInsert("other", "up")).toBe("up")
  })

  it("inserts a ClickHouse metric as a MetricName comparison", () => {
    expect(metricInsert("clickhouse", "http.server.duration")).toBe(
      "MetricName = 'http.server.duration'"
    )
  })

  it("quotes a Prometheus metric name that is not a plain identifier", () => {
    expect(metricInsert("prometheus", "http.server.duration")).toBe(
      '{"http.server.duration"}'
    )
  })

  it("escapes quotes and backslashes in a ClickHouse metric name", () => {
    expect(metricInsert("clickhouse", "a'b\\c")).toBe(
      "MetricName = 'a\\'b\\\\c'"
    )
  })
})

describe("matcherInsert", () => {
  it("writes a Prometheus matcher with double quotes", () => {
    expect(matcherInsert("prometheus", "route", "/cart")).toBe('route="/cart"')
  })

  it("escapes double quotes and backslashes for Prometheus", () => {
    expect(matcherInsert("prometheus", "path", 'a"b\\c')).toBe(
      'path="a\\"b\\\\c"'
    )
  })

  it("writes a newline in a value as an escape for both stores", () => {
    expect(matcherInsert("prometheus", "msg", "a\nb")).toBe('msg="a\\nb"')
    expect(matcherInsert("clickhouse", "Body", "a\nb")).toBe("Body = 'a\\nb'")
  })

  it("quotes a Prometheus label name that is not a plain identifier", () => {
    expect(matcherInsert("prometheus", "service.name", "checkout")).toBe(
      '"service.name"="checkout"'
    )
  })

  it("keeps a plain label name with a colon or underscore unquoted", () => {
    expect(matcherInsert("prometheus", "_a:b1", "x")).toBe('_a:b1="x"')
  })

  it("leaves a single quote alone for Prometheus", () => {
    expect(matcherInsert("prometheus", "pod", "it's")).toBe('pod="it\'s"')
  })

  it("treats an unknown type like Prometheus", () => {
    expect(matcherInsert(undefined, "route", "/cart")).toBe('route="/cart"')
  })

  it("writes a ClickHouse matcher with single quotes", () => {
    expect(matcherInsert("clickhouse", "ServiceName", "checkout")).toBe(
      "ServiceName = 'checkout'"
    )
  })

  it("escapes single quotes and backslashes for ClickHouse", () => {
    expect(matcherInsert("clickhouse", "Body", "it's a\\b")).toBe(
      "Body = 'it\\'s a\\\\b'"
    )
  })

  it("leaves a double quote alone for ClickHouse", () => {
    expect(matcherInsert("clickhouse", "Body", 'say "hi"')).toBe(
      "Body = 'say \"hi\"'"
    )
  })
})
