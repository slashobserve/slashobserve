import { describe, expect, it, vi } from "vitest"

import { bottomEdge, edgeHeight, revealKey, visibleBand } from "./list-anchors"

const view = { x: 0, y: 20, width: 300, height: 100 }

describe("visibleBand", () => {
  it("keeps a row inside the view as it is", () => {
    const rect = { x: 0, y: 40, width: 300, height: 20 }
    expect(visibleBand(rect, view)).toEqual(rect)
  })

  it("cuts a row that is half scrolled out at the view's edge", () => {
    expect(visibleBand({ x: 0, y: 10, width: 300, height: 20 }, view)).toEqual({
      x: 0,
      y: 20,
      width: 300,
      height: 10,
    })
    expect(visibleBand({ x: 0, y: 110, width: 300, height: 20 }, view)).toEqual(
      { x: 0, y: 110, width: 300, height: 10 }
    )
  })

  it("drops a row whose middle is outside the view", () => {
    expect(visibleBand({ x: 0, y: 5, width: 300, height: 20 }, view)).toBeNull()
    expect(
      visibleBand({ x: 0, y: 125, width: 300, height: 20 }, view)
    ).toBeNull()
  })
})

describe("bottomEdge", () => {
  it("is a thin strip along the bottom of the body", () => {
    expect(bottomEdge({ width: 300, height: 140 })).toEqual({
      x: 0,
      y: 140 - edgeHeight,
      width: 300,
      height: edgeHeight,
    })
  })
})

describe("revealKey", () => {
  it("hands the key to reveal and says it did", () => {
    const reveal = vi.fn()
    expect(revealKey(reveal, "k1")).toBe(true)
    expect(reveal).toHaveBeenCalledWith("k1")
  })

  it("does nothing without a reveal or a key", () => {
    const reveal = vi.fn()
    expect(revealKey(undefined, "k1")).toBe(false)
    expect(revealKey(reveal, undefined)).toBe(false)
    expect(reveal).not.toHaveBeenCalled()
  })
})
