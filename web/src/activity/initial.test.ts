import { describe, expect, it } from "vitest"

import { initialOf } from "./initial"

describe("initialOf", () => {
  it("uppercases the first letter", () => {
    expect(initialOf("ada")).toBe("A")
  })

  it("skips leading whitespace", () => {
    expect(initialOf("  grace")).toBe("G")
  })

  it("keeps a whole emoji instead of half a surrogate pair", () => {
    expect(initialOf("🦊 Fox")).toBe("🦊")
  })

  it("falls back to a question mark for an empty name", () => {
    expect(initialOf("")).toBe("?")
    expect(initialOf("   ")).toBe("?")
  })
})
