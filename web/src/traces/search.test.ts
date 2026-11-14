import { describe, expect, it } from "vitest"

import {
  normaliseTraceId,
  tracesTimeRange,
  traceViewRange,
  validateTraceEntrySearch,
  validateTraceSearch,
  validateTraceViewSearch,
} from "./search"

describe("validateTraceEntrySearch", () => {
  it("keeps a valid datasource uid next to the page keys", () => {
    expect(
      validateTraceEntrySearch({ ds: "tempo", service: "checkout" })
    ).toMatchObject({ ds: "tempo", service: "checkout" })
  })

  it("drops an invalid datasource uid", () => {
    expect(validateTraceEntrySearch({ ds: "a/b" }).ds).toBeUndefined()
  })

  it("leaves the datasource out of the page search", () => {
    expect(validateTraceSearch({ ds: "tempo" })).not.toHaveProperty("ds")
  })
})

describe("validateTraceSearch", () => {
  it("keeps a valid search", () => {
    const search = {
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

  it.each([
    ["a page and size from the address", { page: "3", size: "100" }, 3, 100],
    ["a page and size already numbers", { page: 2, size: 250 }, 2, 250],
    [
      "page 1 and the default size as absent",
      { page: "1", size: "50" },
      undefined,
      undefined,
    ],
    [
      "an invalid page and size",
      { page: "0", size: "75" },
      undefined,
      undefined,
    ],
    [
      "a page that is not a whole number",
      { page: "1.5", size: "x" },
      undefined,
      undefined,
    ],
  ])("reads %s", (_, raw, page, size) => {
    const search = validateTraceSearch(raw)
    expect(search.page).toBe(page)
    expect(search.size).toBe(size)
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

  it.each([
    ["keeps a 16 hex span id", "7f3a9c12e4b0d5a1", "7f3a9c12e4b0d5a1"],
    ["lowercases the span id", "7F3A9C12E4B0D5A1", "7f3a9c12e4b0d5a1"],
    ["drops a short span id", "7f3a9c12e4", undefined],
    ["drops a span id with other characters", "7f3a9c12e4b0d5zz", undefined],
  ])("%s", (_, raw, span) => {
    expect(validateTraceViewSearch({ ds: "ch", span: raw }).span).toBe(span)
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
