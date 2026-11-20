import { describe, expect, it } from "vitest"

import { imagePixelRatio } from "./image"

describe("imagePixelRatio", () => {
  it("draws a normal dashboard at twice the size", () => {
    expect(imagePixelRatio(680, 2400)).toBe(2)
  })

  it("lowers the ratio so the longest side stays within the canvas limit", () => {
    const ratio = imagePixelRatio(680, 12000)
    expect(ratio).toBeLessThan(2)
    expect(12000 * ratio).toBeLessThanOrEqual(16384)
  })

  it("lowers the ratio so the area stays within the canvas limit", () => {
    const ratio = imagePixelRatio(2000, 9000)
    expect(ratio).toBeLessThan(2)
    expect(2000 * ratio * 9000 * ratio).toBeLessThanOrEqual(67_108_864)
  })

  it("goes below one for a dashboard too tall to draw in full size", () => {
    expect(imagePixelRatio(680, 20000)).toBeLessThan(1)
  })
})
