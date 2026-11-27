import { describe, expect, it } from "vitest"

import { followCursor } from "./follow-cursor"

describe("followCursor", () => {
  it("reads every page until the cursor runs out", async () => {
    const seen: (string | undefined)[] = []
    const pages: Record<string, { items: number[]; next: string | null }> = {
      first: { items: [1, 2], next: "b" },
      b: { items: [3], next: "c" },
      c: { items: [4], next: null },
    }
    const items = await followCursor(async (cursor) => {
      seen.push(cursor)
      return pages[cursor ?? "first"]
    })
    expect(items).toEqual([1, 2, 3, 4])
    expect(seen).toEqual([undefined, "b", "c"])
  })

  it("stops after ten pages", async () => {
    let calls = 0
    const items = await followCursor(async () => {
      calls++
      return { items: [calls], next: "more" }
    })
    expect(calls).toBe(10)
    expect(items).toHaveLength(10)
  })
})
