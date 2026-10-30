import { describe, expect, it } from "vitest"

import {
  initialView,
  maxZoom,
  minZoom,
  wheelFactor,
  zoomView,
  type View,
} from "./zoom"

describe("zoomView", () => {
  it("scales the pan with the zoom when zooming about the center", () => {
    const view: View = { zoom: 1, panX: 40, panY: -20 }
    expect(zoomView(view, 2)).toEqual({ zoom: 2, panX: 80, panY: -40 })
  })

  it("keeps the point under the cursor fixed", () => {
    const view: View = { zoom: 1, panX: 10, panY: 0 }
    const anchor = { x: 100, y: 50 }
    const next = zoomView(view, 2, anchor)
    const before = { x: anchor.x - view.panX, y: anchor.y - view.panY }
    const after = { x: anchor.x - next.panX, y: anchor.y - next.panY }
    expect(after.x).toBeCloseTo(before.x * 2)
    expect(after.y).toBeCloseTo(before.y * 2)
  })

  it("clamps the zoom and uses the clamped ratio for the pan", () => {
    const view: View = { zoom: 3, panX: 30, panY: 0 }
    const next = zoomView(view, 10)
    expect(next.zoom).toBe(maxZoom)
    expect(next.panX).toBeCloseTo(30 * (maxZoom / 3))
    expect(zoomView(initialView, 0.0001).zoom).toBe(minZoom)
  })
})

describe("wheelFactor", () => {
  it("zooms in when scrolling up and out when scrolling down", () => {
    expect(wheelFactor(-100, 0)).toBeGreaterThan(1)
    expect(wheelFactor(100, 0)).toBeLessThan(1)
    expect(wheelFactor(0, 0)).toBe(1)
  })

  it("treats a line of scroll as more than a pixel", () => {
    expect(wheelFactor(-1, 1)).toBeGreaterThan(wheelFactor(-1, 0))
    expect(wheelFactor(-1, 2)).toBeGreaterThan(wheelFactor(-1, 1))
  })
})
