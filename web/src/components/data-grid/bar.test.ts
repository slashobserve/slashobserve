import { describe, expect, it } from "vitest"

import { barColumnMinWidth, barShare, textWidthCh } from "./bar"

describe("textWidthCh", () => {
  it("fits the longest text", () => {
    expect(textWidthCh(["12 ms", "3,020,004.4 ms", "-"])).toBe(14)
  })

  it("keeps one character for no texts", () => {
    expect(textWidthCh([])).toBe(1)
  })
})

describe("barShare", () => {
  it("scales a value against the largest", () => {
    expect(barShare(25, 100)).toBe(0.25)
  })

  it("clamps to the bar", () => {
    expect(barShare(150, 100)).toBe(1)
    expect(barShare(-5, 100)).toBe(0)
  })

  it("draws nothing without a value or a scale", () => {
    expect(barShare(null, 100)).toBe(0)
    expect(barShare(10, 0)).toBe(0)
    expect(barShare(Number.NaN, 100)).toBe(0)
  })
})

describe("barColumnMinWidth", () => {
  it("leaves room for the text, the gap, a visible bar and the padding", () => {
    expect(barColumnMinWidth(14)).toBe(14 * 8 + 10 + 40 + 28)
  })
})
