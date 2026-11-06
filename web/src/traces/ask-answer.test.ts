import { describe, expect, it } from "vitest"

import { traceSearchFromAnswer } from "./ask-answer"

describe("traceSearchFromAnswer", () => {
  it("keeps the filters and drops the query on ClickHouse", () => {
    expect(
      traceSearchFromAnswer(
        {
          query: "SELECT 1",
          traceFilters: {
            service: "checkout",
            span: "POST /pay",
            min: "2s",
            max: "10s",
          },
        },
        "clickhouse"
      )
    ).toEqual({
      service: "checkout",
      span: "POST /pay",
      min: "2s",
      max: "10s",
      errors: undefined,
      q: undefined,
    })
  })

  it("maps errors true to the URL flag", () => {
    expect(
      traceSearchFromAnswer(
        { query: "", traceFilters: { errors: true } },
        "clickhouse"
      ).errors
    ).toBe("1")
    expect(
      traceSearchFromAnswer(
        { query: "", traceFilters: { errors: false } },
        "clickhouse"
      ).errors
    ).toBeUndefined()
  })

  it("drops a duration the form cannot read", () => {
    const search = traceSearchFromAnswer(
      { query: "", traceFilters: { min: "fast", max: "2s" } },
      "clickhouse"
    )
    expect(search.min).toBeUndefined()
    expect(search.max).toBe("2s")
  })

  it("puts a TraceQL query into the TraceQL field on Tempo", () => {
    expect(
      traceSearchFromAnswer(
        {
          query: '{resource.service.name = "checkout" && duration > 2s}',
          traceFilters: null,
        },
        "tempo"
      )
    ).toEqual({
      service: undefined,
      span: undefined,
      min: undefined,
      max: undefined,
      errors: undefined,
      q: '{resource.service.name = "checkout" && duration > 2s}',
    })
  })

  it("leaves the TraceQL field empty on Tempo when the answer is only filters", () => {
    expect(
      traceSearchFromAnswer(
        { query: "", traceFilters: { service: "cart" } },
        "tempo"
      )
    ).toMatchObject({ service: "cart", q: undefined })
  })
})
