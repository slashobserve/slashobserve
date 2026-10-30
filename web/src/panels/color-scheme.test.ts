import { describe, expect, it } from "vitest"

import { rangePosition, schemeColor, schemes } from "./color-scheme"

const anchors = {
  greenYellowRed: [
    "--threshold-ok",
    "--threshold-warning",
    "--threshold-critical",
  ],
  redYellowGreen: [
    "--threshold-critical",
    "--threshold-warning",
    "--threshold-ok",
  ],
  blueYellowRed: ["--series-1", "--threshold-warning", "--threshold-critical"],
  yellowRed: ["--threshold-warning", "--threshold-critical"],
  bluePurple: ["--series-1", "--series-7"],
  yellowBlue: ["--threshold-warning", "--series-1"],
  viridis: ["--series-7", "--series-3", "--threshold-warning"],
  magma: ["--series-7", "--threshold-critical", "--threshold-warning"],
  plasma: ["--series-7", "--series-5", "--threshold-warning"],
  inferno: ["--series-7", "--threshold-critical", "--threshold-warning"],
  cividis: ["--series-1", "--muted-foreground", "--threshold-warning"],
  warm: ["--series-7", "--series-2", "--threshold-warning"],
} as const

const hues = {
  blues: "--series-1",
  reds: "--threshold-critical",
  greens: "--threshold-ok",
  purples: "--series-7",
  oranges: "--series-2",
} as const

describe("schemeColor", () => {
  it.each(Object.entries(anchors))(
    "%s starts and ends on its anchor tokens and passes through the middle one",
    (scheme, stops) => {
      const name = scheme as keyof typeof anchors
      expect(schemeColor(name, 0)).toBe(`var(${stops[0]})`)
      expect(schemeColor(name, 1)).toBe(`var(${stops[stops.length - 1]})`)
      if (stops.length === 3) {
        expect(schemeColor(name, 0.5)).toBe(`var(${stops[1]})`)
      }
    }
  )

  it.each(Object.entries(hues))(
    "%s runs from a faint tint over the background to the full hue",
    (scheme, hue) => {
      const name = scheme as keyof typeof hues
      expect(schemeColor(name, 1)).toBe(`var(${hue})`)
      expect(schemeColor(name, 0)).toBe(
        `color-mix(in oklab, var(${hue}) 15%, var(--background))`
      )
    }
  )

  it("mixes between two anchors in oklab by the position inside the segment", () => {
    expect(schemeColor("yellowRed", 0.25)).toBe(
      "color-mix(in oklab, var(--threshold-critical) 25%, var(--threshold-warning))"
    )
    expect(schemeColor("greenYellowRed", 0.75)).toBe(
      "color-mix(in oklab, var(--threshold-critical) 50%, var(--threshold-warning))"
    )
  })

  it("clamps the position into 0..1 and treats a missing one as 0", () => {
    expect(schemeColor("yellowRed", -3)).toBe(schemeColor("yellowRed", 0))
    expect(schemeColor("yellowRed", 7)).toBe(schemeColor("yellowRed", 1))
    expect(schemeColor("yellowRed", Number.NaN)).toBe(
      schemeColor("yellowRed", 0)
    )
  })

  it("resolves every scheme in the list to a token-based color", () => {
    expect(schemes).toHaveLength(Object.keys(anchors).length + 5)
    for (const scheme of schemes) {
      for (const t of [0, 0.2, 0.5, 0.8, 1]) {
        expect(schemeColor(scheme, t)).toMatch(
          /^(var\(--[a-z0-9-]+\)|color-mix\(in oklab, var\(--[a-z0-9-]+\) \d+%, var\(--[a-z0-9-]+\)\))$/
        )
      }
    }
  })
})

describe("rangePosition", () => {
  it("places a value between the minimum and the maximum", () => {
    expect(rangePosition(25, 0, 100)).toBe(0.25)
    expect(rangePosition(-5, 0, 100)).toBe(0)
    expect(rangePosition(150, 0, 100)).toBe(1)
  })

  it("puts every value at the top of an empty range", () => {
    expect(rangePosition(3, 3, 3)).toBe(1)
  })
})
