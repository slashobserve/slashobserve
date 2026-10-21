import { describe, expect, it } from "vitest"

import {
  contrastRatio,
  darkText,
  lightText,
  parseRgb,
  readableText,
  type Rgb,
} from "./contrast"

const tokens = [
  ["light ok", "#0ca30c"],
  ["light info", "#256abf"],
  ["light warning", "#fab219"],
  ["light critical", "#d03b3b"],
  ["dark ok", "#0ca30c"],
  ["dark info", "#5598e7"],
  ["dark warning", "#fab219"],
  ["dark critical", "#d03b3b"],
] as const
const white: Rgb = [255, 255, 255]
const nearBlack: Rgb = [10, 10, 10]

function rgbOf(hex: string): Rgb {
  const rgb = parseRgb(hex)
  if (!rgb) throw new Error(`not a color: ${hex}`)
  return rgb
}

describe("contrast", () => {
  it("rates black on white 21 and a color on itself 1", () => {
    expect(contrastRatio([0, 0, 0], white)).toBeCloseTo(21, 5)
    expect(contrastRatio([12, 163, 12], [12, 163, 12])).toBe(1)
  })

  it("picks white on dark backgrounds and near-black on light ones", () => {
    expect(readableText([0, 0, 0])).toBe(lightText)
    expect(readableText([255, 255, 0])).toBe(darkText)
  })

  it.each(tokens)(
    "gives text on the %s threshold color %s at least 4.5:1",
    (_name, hex) => {
      const background = rgbOf(hex)
      const text = readableText(background) === lightText ? white : nearBlack
      expect(contrastRatio(background, text)).toBeGreaterThanOrEqual(4.5)
    }
  )

  it("parses the forms a canvas fillStyle returns", () => {
    expect(parseRgb("#0ca30c")).toEqual([12, 163, 12])
    expect(parseRgb("rgba(12, 163, 12, 0.5)")).toEqual([12, 163, 12])
    expect(parseRgb("rgb(12 163 12)")).toEqual([12, 163, 12])
    expect(parseRgb("not a color")).toBeNull()
  })
})
