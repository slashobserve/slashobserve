import { describe, expect, it } from "vitest"

import {
  askAfterSubmit,
  changesQuery,
  linkWithoutPage,
  normaliseTraceId,
  rangeInFuture,
  searchRange,
  traceViewDatasource,
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
  it("keeps a saved search id and drops anything else", () => {
    const id = "5f0c1e0e-0000-4000-8000-000000000001"
    expect(validateTraceSearch({ saved: id }).saved).toBe(id)
    expect(validateTraceSearch({ saved: "not-an-id" }).saved).toBeUndefined()
  })

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
    expect(searchRange(validateTraceSearch({ from: "now-1d" }))).toEqual({
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

describe("the observation a trace search was entered from", () => {
  const observation = "8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5b"

  it.each([
    ["the list", validateTraceSearch],
    ["the entry page", validateTraceEntrySearch],
    ["a single trace", validateTraceViewSearch],
  ])("keeps a valid observation id on %s", (_, validate) => {
    expect(validate({ observation }).observation).toBe(observation)
  })

  it.each([
    ["the list", validateTraceSearch],
    ["a single trace", validateTraceViewSearch],
  ])("drops an observation that is not a uuid on %s", (_, validate) => {
    expect(
      validate({ observation: "checkout-slow" }).observation
    ).toBeUndefined()
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

describe("askAfterSubmit", () => {
  const search = {
    service: "payments",
    min: "500ms",
    ask: "payments requests slower than 500ms",
  }

  it("keeps the question while the form still holds the answer", () => {
    expect(askAfterSubmit(search, { service: "payments", min: "500ms" })).toBe(
      search.ask
    )
  })

  it("drops the question once a field was changed or cleared", () => {
    expect(askAfterSubmit(search, { min: "500ms" })).toBeUndefined()
    expect(
      askAfterSubmit(search, { service: "payments", min: "1s" })
    ).toBeUndefined()
    expect(
      askAfterSubmit(search, { service: "payments", min: "500ms", errors: "1" })
    ).toBeUndefined()
  })
})

describe("rangeInFuture", () => {
  const now = Date.parse("2026-11-18T19:59:00Z")

  it("is true when the range starts after now", () => {
    expect(
      rangeInFuture(
        { from: "2026-11-18T20:10:00Z", to: "2026-11-18T20:20:00Z" },
        now
      )
    ).toBe(true)
  })

  it("is false for a past or relative range, or one that reaches now", () => {
    expect(rangeInFuture({ from: "now-1h", to: "now" }, now)).toBe(false)
    expect(
      rangeInFuture(
        { from: "2026-11-18T19:00:00Z", to: "2026-11-18T20:00:00Z" },
        now
      )
    ).toBe(false)
    expect(rangeInFuture({ from: "now", to: "now+1h" }, now)).toBe(false)
  })
})

describe("linkWithoutPage", () => {
  it("drops the page and keeps the rest of the address", () => {
    expect(
      linkWithoutPage("http://x.test/traces/tempo?service=a&page=3&size=100")
    ).toBe("http://x.test/traces/tempo?service=a&size=100")
  })

  it("leaves an address without a page as it is", () => {
    expect(linkWithoutPage("http://x.test/traces/tempo")).toBe(
      "http://x.test/traces/tempo"
    )
  })
})

describe("traceViewDatasource", () => {
  const tempo = { uid: "tempo" }
  const ch = { uid: "ch" }

  it("opens the datasource the address names", () => {
    expect(traceViewDatasource([tempo, ch], "ch")).toBe(ch)
    expect(traceViewDatasource([tempo], "gone")).toBeUndefined()
  })

  it("opens the only trace datasource when the address names none", () => {
    expect(traceViewDatasource([tempo], undefined)).toBe(tempo)
  })

  it("asks when several could hold the trace", () => {
    expect(traceViewDatasource([tempo, ch], undefined)).toBeUndefined()
    expect(traceViewDatasource([], undefined)).toBeUndefined()
  })
})

describe("changesQuery", () => {
  it("is true when a filter key is written, even to clear it", () => {
    expect(changesQuery({ q: "{}" })).toBe(true)
    expect(changesQuery({ service: undefined })).toBe(true)
    expect(changesQuery({ min: "1s", from: "now-1h" })).toBe(true)
  })

  it("is false for paging and range changes", () => {
    expect(changesQuery({ page: 2 })).toBe(false)
    expect(changesQuery({ size: 100 })).toBe(false)
    expect(changesQuery({ from: "now-6h", to: "now" })).toBe(false)
  })
})

describe("the trace sort", () => {
  it("keeps a sort other than the default and drops the rest", () => {
    expect(validateTraceSearch({ sort: "-spans" }).sort).toBe("-spans")
    expect(validateTraceSearch({ sort: "none" }).sort).toBe("none")
    expect(validateTraceSearch({ sort: "-duration" }).sort).toBeUndefined()
    expect(validateTraceSearch({ sort: "Duration; drop" }).sort).toBeUndefined()
  })
})

describe("the keys a redirect carries to the trace search", () => {
  const raw = {
    foo: "1",
    service: "cart",
    q: "{}",
    from: "now-1h",
    to: "now",
    ds: "tempo",
  }

  it("are the ones the page reads, the time range pair included", () => {
    const kept = validateTraceSearch(raw)
    expect(Object.keys(kept)).not.toContain("foo")
    expect(Object.keys(kept)).not.toContain("ds")
    expect(kept).toMatchObject({ service: "cart", from: "now-1h", to: "now" })
  })

  it("add the source key on the index", () => {
    const kept = validateTraceEntrySearch(raw)
    expect(Object.keys(kept)).not.toContain("foo")
    expect(kept.ds).toBe("tempo")
  })
})
