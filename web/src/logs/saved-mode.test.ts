import { describe, expect, it } from "vitest"

import { modeOnSavedSearch } from "./saved-mode"

describe("modeOnSavedSearch", () => {
  it("leaves the Ask tab for the query when a saved search opens", () => {
    expect(modeOnSavedSearch("ask", undefined, "s1")).toBe("code")
    expect(modeOnSavedSearch("ask", "s1", "s2")).toBe("code")
  })

  it.each(["code", "builder"] as const)("keeps %s", (mode) => {
    expect(modeOnSavedSearch(mode, undefined, "s1")).toBe(mode)
  })

  it("keeps the Ask tab when no saved search is newly open", () => {
    expect(modeOnSavedSearch("ask", "s1", "s1")).toBe("ask")
    expect(modeOnSavedSearch("ask", "s1", undefined)).toBe("ask")
    expect(modeOnSavedSearch("ask", undefined, undefined)).toBe("ask")
  })
})
