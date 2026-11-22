import { describe, expect, it } from "vitest"

import {
  appliedKey,
  appliedOf,
  frameOf,
  needsCheck,
  syncFrame,
  type Applied,
} from "./frame"
import type { AuditView } from "./search"

const view: AuditView = {
  q: "actor:admin",
  from: "now-24h",
  to: "now",
  event: null,
}

const applied: Applied = appliedOf(view)

describe("syncFrame", () => {
  it("keeps the frame while the address has not moved", () => {
    const frame = frameOf(applied, appliedKey(applied), 100)
    expect(syncFrame(frame, view, 200)).toBe(frame)
  })

  it("keeps the anchor when the address catches up with the applied search", () => {
    const next: Applied = { ...applied, q: "origin:mcp" }
    const frame = frameOf(next, appliedKey(applied), 100)
    const synced = syncFrame(frame, { ...view, q: "origin:mcp" }, 200)
    expect(synced.anchor).toBe(100)
    expect(synced.seen).toBe(appliedKey(next))
  })

  it("starts over when the address moves to a search the page did not apply", () => {
    const frame = frameOf(applied, appliedKey(applied), 100)
    const synced = syncFrame(frame, { ...view, q: "outcome:failed" }, 200)
    expect(synced.anchor).toBe(200)
    expect(synced.applied.q).toBe("outcome:failed")
    expect(synced.live).toEqual([])
  })
})

describe("needsCheck", () => {
  it("asks the server about a changed query over the same range", () => {
    expect(needsCheck(applied, { ...view, q: "origin:mcp" })).toBe(true)
  })

  it.each([
    ["an unchanged query", { q: applied.q }],
    ["an emptied query", { q: "  " }],
    ["a changed range", { q: "origin:mcp", from: "now-7d" }],
  ])("skips the check for %s", (_name, change) => {
    expect(needsCheck(applied, { ...view, ...change })).toBe(false)
  })
})
