import { describe, expect, it } from "vitest"

import { initialMode, nextMode, parseStoredMode } from "./input-mode"

const opening = {
  stored: null,
  hasQuery: false,
  hasAsk: false,
  builderCanShow: false,
}

describe("initialMode", () => {
  it("opens in code without a model even with ask stored", () => {
    expect(
      initialMode({ ...opening, agentAvailable: false, stored: "ask" })
    ).toBe("code")
  })

  it("opens in code for a link with a query the builder cannot show", () => {
    expect(
      initialMode({ ...opening, agentAvailable: true, hasQuery: true })
    ).toBe("code")
  })

  it("opens in the builder for a link with a query it can show", () => {
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        hasQuery: true,
        builderCanShow: true,
      })
    ).toBe("builder")
  })

  it("opens a query and a question in the query's mode", () => {
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        hasQuery: true,
        hasAsk: true,
      })
    ).toBe("code")
  })

  it("opens in ask for a link with only a question", () => {
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        stored: "code",
        hasAsk: true,
      })
    ).toBe("ask")
  })

  it("keeps the stored mode", () => {
    expect(
      initialMode({ ...opening, agentAvailable: true, stored: "code" })
    ).toBe("code")
    expect(
      initialMode({
        ...opening,
        agentAvailable: true,
        builderCanShow: true,
        stored: "builder",
      })
    ).toBe("builder")
  })

  it("falls back to code when the stored builder cannot show", () => {
    expect(
      initialMode({ ...opening, agentAvailable: true, stored: "builder" })
    ).toBe("code")
  })

  it("opens in ask when nothing is stored", () => {
    expect(initialMode({ ...opening, agentAvailable: true })).toBe("ask")
  })
})

describe("parseStoredMode", () => {
  it.each(["ask", "builder", "code"] as const)("keeps %s", (mode) => {
    expect(parseStoredMode(mode)).toBe(mode)
  })

  it("reads the old query mode as code", () => {
    expect(parseStoredMode("query")).toBe("code")
  })

  it.each([null, "", "Ask", "sql"])("drops %s", (raw) => {
    expect(parseStoredMode(raw)).toBeNull()
  })
})

describe("nextMode", () => {
  it("cycles builder, code and ask", () => {
    expect(nextMode("builder", true, true)).toBe("code")
    expect(nextMode("code", true, true)).toBe("ask")
    expect(nextMode("ask", true, true)).toBe("builder")
  })

  it("skips the builder when it cannot show", () => {
    expect(nextMode("ask", true, false)).toBe("code")
    expect(nextMode("code", true)).toBe("ask")
  })

  it("skips ask without a model", () => {
    expect(nextMode("builder", false, true)).toBe("code")
    expect(nextMode("code", false, true)).toBe("builder")
  })

  it("stays in code without a model or a builder", () => {
    expect(nextMode("code", false)).toBe("code")
  })
})
