import { describe, expect, it } from "vitest"

import {
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
