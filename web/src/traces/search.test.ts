import { describe, expect, it } from "vitest"

import {
  normaliseTraceId,
  tracesTimeRange,
  traceViewRange,
  validateTraceSearch,
  validateTraceViewSearch,
} from "./search"

describe("validateTraceSearch", () => {
  it("keeps a valid search", () => {
    const search = {
      ds: "tempo",
      service: "checkout",
      span: "GET /pay",
      min: "250ms",
      max: "2s",
      errors: "1",
      q: "{}",
      ask: "checkout requests slower than 2s",
      from: "now-6h",
      to: "now",
    }
    expect(validateTraceSearch(search)).toEqual(search)
  })

  it.each([
    ["a service with a control character", { service: "a\nb" }, "service"],
    ["a span name over 200 characters", { span: "x".repeat(201) }, "span"],
    ["an unreadable duration", { min: "fast" }, "min"],
    ["errors other than 1", { errors: "true" }, "errors"],
    ["an empty TraceQL", { q: "" }, "q"],
    ["a question with a control character", { ask: "slow\u0000" }, "ask"],
    ["an empty question", { ask: "" }, "ask"],
  ] as const)("drops %s", (_, raw, key) => {
    expect(validateTraceSearch(raw)[key]).toBeUndefined()
  })

  it("defaults the search range to the last hour", () => {
    expect(tracesTimeRange(validateTraceSearch({ from: "now-1d" }))).toEqual({
      from: "now-1h",
      to: "now",
    })
  })
})

describe("validateTraceViewSearch", () => {
  it("keeps the datasource and range and defaults to the last 24 hours", () => {
    expect(validateTraceViewSearch({ ds: "ch", from: "1", to: "2" })).toEqual({
      ds: "ch",
      from: undefined,
      to: undefined,
    })
    expect(traceViewRange({})).toEqual({ from: "now-24h", to: "now" })
  })
})

describe("normaliseTraceId", () => {
  it.each([
    ["4BF92F3577B34DA6A3CE929D0E0E4736", "4bf92f3577b34da6a3ce929d0e0e4736"],
    ["a3ce929d0e0e4736", "a3ce929d0e0e4736"],
    ["xyz", null],
    ["a3ce929d0e0e47", null],
  ])("reads %s as %s", (param, id) => {
    expect(normaliseTraceId(param)).toBe(id)
  })
})
