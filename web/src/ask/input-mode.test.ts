import { describe, expect, it } from "vitest"

import { initialMode, nextMode, parseStoredMode } from "./input-mode"

const opening = { stored: null, hasQuery: false, hasAsk: false }

describe("initialMode", () => {
  it("opens in query without a model even with ask stored", () => {
    expect(
      initialMode({ ...opening, agentAvailable: false, stored: "ask" })
    ).toBe("query")
  })

  it("opens in query for a link with a query and no question", () => {
    expect(
      initialMode({ ...opening, agentAvailable: true, hasQuery: true })
    ).toBe("query")
  })

  it("opens in query for a link with a query and a question", () => {
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        hasQuery: true,
        hasAsk: true,
      })
    ).toBe("query")
  })

  it("opens in ask for a link with only a question", () => {
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        stored: "query",
        hasAsk: true,
      })
    ).toBe("ask")
  })

  it("keeps the stored mode", () => {
    expect(
      initialMode({ ...opening, agentAvailable: true, stored: "query" })
    ).toBe("query")
  })

  it("opens in ask when nothing is stored", () => {
    expect(initialMode({ ...opening, agentAvailable: true })).toBe("ask")
  })
})

describe("parseStoredMode", () => {
  it.each(["ask", "query"] as const)("keeps %s", (mode) => {
    expect(parseStoredMode(mode)).toBe(mode)
  })

  it.each([null, "", "Ask", "sql"])("drops %s", (raw) => {
    expect(parseStoredMode(raw)).toBeNull()
  })
})

describe("nextMode", () => {
  it("flips between ask and query", () => {
    expect(nextMode("ask", true)).toBe("query")
    expect(nextMode("query", true)).toBe("ask")
  })

  it("stays in query without a model", () => {
    expect(nextMode("query", false)).toBe("query")
  })
})
