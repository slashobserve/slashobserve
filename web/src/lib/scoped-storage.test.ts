import { afterEach, describe, expect, it, vi } from "vitest"

const principal = vi.hoisted(() => ({ id: undefined as string | undefined }))
vi.mock("@/lib/auth", () => ({ signedInPrincipalId: () => principal.id }))

import { clearUnscopedHistory, scopedKey } from "./scoped-storage"

afterEach(() => {
  localStorage.clear()
  principal.id = undefined
})

describe("scopedKey", () => {
  it("has no key while nobody is signed in", () => {
    expect(scopedKey("slashobserve-prompt-history")).toBeNull()
  })

  it("clears the unscoped keys on first use", () => {
    localStorage.setItem("slashobserve-ask-history-traces", '["old"]')
    principal.id = "u1"
    scopedKey("slashobserve-prompt-history")
    expect(localStorage.getItem("slashobserve-ask-history-traces")).toBeNull()
  })

  it("adds the principal id", () => {
    principal.id = "u1"
    expect(scopedKey("slashobserve-prompt-history")).toBe(
      "slashobserve-prompt-history:u1"
    )
  })
})

describe("clearUnscopedHistory", () => {
  it("keeps scoped keys and other storage", () => {
    localStorage.setItem("slashobserve-recent-observations", "[]")
    localStorage.setItem("slashobserve-recent-observations:u1", "[]")
    localStorage.setItem("slashobserve-theme", "dark")
    clearUnscopedHistory()
    expect(localStorage.getItem("slashobserve-recent-observations")).toBeNull()
    expect(localStorage.getItem("slashobserve-recent-observations:u1")).toBe(
      "[]"
    )
    expect(localStorage.getItem("slashobserve-theme")).toBe("dark")
  })
})
