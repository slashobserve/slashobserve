import { describe, expect, it } from "vitest"

import { matchPrompts, pickerQuery, stepIndex } from "./match"
import type { SavedPrompt } from "./queries"

function prompt(name: string, body: string): SavedPrompt {
  return {
    id: name,
    name,
    body,
    shared: false,
    owner: { id: "u1", name: "Ada" },
    can_edit: true,
    created_at: "2026-10-30T00:00:00Z",
    updated_at: "2026-10-30T00:00:00Z",
  }
}

describe("pickerQuery", () => {
  const cases: { input: string; expected: string | null }[] = [
    { input: "/", expected: "" },
    { input: "/chk", expected: "chk" },
    { input: "/check out", expected: "check out" },
    { input: "a/", expected: null },
    { input: "/x\ny", expected: null },
    { input: "", expected: null },
  ]

  it.each(cases)("reads $input as $expected", ({ input, expected }) => {
    expect(pickerQuery(input)).toBe(expected)
  })
})

describe("matchPrompts", () => {
  const prompts = [
    prompt("Latency by route", "p99 for checkout"),
    prompt("Checkout health", "pods by status"),
    prompt("Payments errors", "5xx on the CHECKOUT path"),
    prompt("Pre-checkout cache", "hit ratio"),
    prompt("Disk", "node disk usage"),
  ]

  it("returns every prompt unchanged for an empty query", () => {
    expect(matchPrompts(prompts, "")).toBe(prompts)
    expect(matchPrompts(prompts, "   ")).toBe(prompts)
  })

  it("lists name prefixes, then name matches, then body matches, ignoring case", () => {
    expect(matchPrompts(prompts, "CheckOut").map((p) => p.name)).toEqual([
      "Checkout health",
      "Pre-checkout cache",
      "Latency by route",
      "Payments errors",
    ])
  })

  it("lists a prompt once and finds nothing for an unknown query", () => {
    expect(matchPrompts(prompts, "disk").map((p) => p.name)).toEqual(["Disk"])
    expect(matchPrompts(prompts, "zzz")).toEqual([])
  })
})

describe("stepIndex", () => {
  it("wraps in both directions and stays at 0 without items", () => {
    expect(stepIndex(0, 3, 1)).toBe(1)
    expect(stepIndex(2, 3, 1)).toBe(0)
    expect(stepIndex(0, 3, -1)).toBe(2)
    expect(stepIndex(1, 3, -1)).toBe(0)
    expect(stepIndex(0, 0, 1)).toBe(0)
    expect(stepIndex(0, 0, -1)).toBe(0)
  })
})
