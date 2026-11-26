import { describe, expect, it } from "vitest"

import {
  inlineValuesWidth,
  legendListWidth,
  legendNameRoom,
  legendTableWidth,
  legendValueWidths,
  rightLegendWidth,
} from "./legend-width"

const tenPerCharacter = (text: string) => text.length * 10

describe("legendValueWidths", () => {
  it("sizes each reducer column to its header or its widest value, with padding", () => {
    expect(
      legendValueWidths(
        ["Mean", "Max"],
        [
          ["1.20", "10"],
          ["300.50", "9"],
        ],
        tenPerCharacter,
        16
      )
    ).toEqual([12 + 60, 12 + 30])
  })
})

describe("legendTableWidth", () => {
  it("adds the name column and one column per reducer at their widest text", () => {
    const width = legendTableWidth(
      ["checkout-0", "pod"],
      ["Mean", "Max"],
      [
        ["1.20", "10"],
        ["300.50", "9"],
      ],
      tenPerCharacter,
      16
    )
    const name = 12 + 16 + 6 + 100
    const mean = 12 + 60
    const max = 12 + 30
    expect(width).toBe(8 + name + mean + max)
  })

  it("lets a header set a column wider than its values", () => {
    const narrow = legendTableWidth(
      ["a"],
      ["Last"],
      [["1"]],
      tenPerCharacter,
      16
    )
    const wide = legendTableWidth(
      ["a"],
      ["Last"],
      [["12345678"]],
      tenPerCharacter,
      16
    )
    expect(narrow).toBe(8 + 64 + 12 + 40)
    expect(wide).toBe(8 + 64 + 12 + 80)
  })

  it("keeps the name column at least four rem wide", () => {
    expect(legendTableWidth(["a"], [], [[]], tenPerCharacter, 16)).toBe(8 + 64)
  })

  it("scales with the root font size", () => {
    expect(legendTableWidth(["a"], [], [[]], tenPerCharacter, 20)).toBe(10 + 80)
  })
})

describe("rightLegendWidth", () => {
  it("takes thirty percent of the panel between 160 and 280 px without values", () => {
    expect(rightLegendWidth(null, 400)).toBe(160)
    expect(rightLegendWidth(null, 700)).toBe(210)
    expect(rightLegendWidth(null, 1200)).toBe(280)
  })

  it("fits a value table to its natural width", () => {
    expect(rightLegendWidth(240, 640)).toBe(240)
  })

  it("caps a value table at half the panel", () => {
    expect(rightLegendWidth(520, 640)).toBe(320)
  })

  it("uses a configured width and leaves the chart 120 px", () => {
    expect(rightLegendWidth(240, 640, 200)).toBe(200)
    expect(rightLegendWidth(null, 640, 900)).toBe(520)
    expect(rightLegendWidth(null, 100, 300)).toBe(0)
  })
})

describe("legendNameRoom", () => {
  it("is what legendTableWidth leaves for the name text", () => {
    const labels = ["checkout-0", "pod"]
    const headers = ["Mean"]
    const cells = [["1.20"], ["300.50"]]
    const width = legendTableWidth(labels, headers, cells, tenPerCharacter, 16)
    const values = legendValueWidths(headers, cells, tenPerCharacter, 16)[0]
    expect(legendNameRoom(width, 16, values)).toBe(100)
  })
})

describe("inlineValuesWidth", () => {
  const liveDonut: Record<string, number> = {
    "119  0.2%": 49.27,
    "1,107  1.7%": 52.64,
    "62,103  98.1%": 71.72,
    UNSET: 38.89,
  }
  const measured = (text: string) => liveDonut[text]

  it("leaves a right list legend row exactly the room its button has for the name", () => {
    const values = inlineValuesWidth(
      ["119  0.2%", "1,107  1.7%", "62,103  98.1%"],
      measured,
      16
    )
    const room = legendNameRoom(160, 16, values)
    const rowContent = 160 - 2 * 4 - 2 * 6
    const swatchAndGaps = 16 + 6 + 6
    expect(room).toBeCloseTo(rowContent - swatchAndGaps - 71.72, 5)
    expect(room).toBeGreaterThanOrEqual(measured("UNSET"))
  })
})

describe("legendListWidth", () => {
  const geist: Record<string, number> = {
    STATUS_CODE_ERROR: 128.92,
    STATUS_CODE_OK: 105.2,
    STATUS_CODE_UNSET: 127.66,
    ERROR: 40.14,
    OK: 16.43,
    UNSET: 38.88,
    "155  0.2%": 53.06,
    "1,218  1.7%": 54.28,
    "68,538  98.0%": 79.25,
  }
  const measured = (text: string) => geist[text]
  const labels = ["STATUS_CODE_ERROR", "STATUS_CODE_OK", "STATUS_CODE_UNSET"]
  const values = ["155  0.2%", "1,218  1.7%", "68,538  98.0%"]
  const fixed = 8 + 12 + 16 + 6
  const valueColumn = 6 + 79.25

  it("is the widest full name and value when that leaves the plot its minimum", () => {
    expect(legendListWidth(labels, values, measured, 16, 498)).toBe(
      Math.ceil(fixed + 128.92 + valueColumn)
    )
  })

  it("falls back to the names without their shared prefix when full names crowd the plot", () => {
    expect(legendListWidth(labels, values, measured, 16, 298)).toBe(
      Math.ceil(fixed + 40.14 + valueColumn)
    )
  })

  it.each([
    { panel: 498, legend: 257, name: "STATUS_CODE_ERROR" },
    { panel: 298, legend: 168, name: "UNSET" },
  ])(
    "gives a $panel px pie a $legend px legend where $name and its value fit whole",
    ({ panel, legend, name }) => {
      const natural = legendListWidth(labels, values, measured, 16, panel)
      const width = rightLegendWidth(null, panel, undefined, natural)
      expect(width).toBe(legend)
      expect(panel - width).toBeGreaterThanOrEqual(120)
      expect(legendNameRoom(width, 16, valueColumn)).toBeGreaterThanOrEqual(
        measured(name)
      )
    }
  )

  it("never narrows a list legend below the plain rule", () => {
    expect(rightLegendWidth(null, 600, undefined, 150)).toBe(180)
  })

  it("leaves the width to the plain rule when even the short names crowd the plot", () => {
    expect(legendListWidth(labels, values, measured, 16, 280)).toBeNull()
    expect(rightLegendWidth(null, 280, undefined, null)).toBe(160)
  })
})
