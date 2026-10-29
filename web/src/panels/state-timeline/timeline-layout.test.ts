import { describe, expect, it } from "vitest"

import {
  labelWidth,
  legendHeight,
  legendItems,
  rowHeight,
  showsText,
  timeToX,
} from "./timeline-layout"

describe("timeline layout", () => {
  it("sizes the label column between 80 and 200 pixels", () => {
    expect(labelWidth(100)).toBe(80)
    expect(labelWidth(600)).toBe(120)
    expect(labelWidth(2000)).toBe(200)
  })

  it("caps rows at 32 pixels and shrinks them to fit", () => {
    expect(rowHeight(400, 2)).toBe(32)
    expect(rowHeight(90, 3)).toBe(30)
    expect(rowHeight(1, 5)).toBe(1)
    expect(rowHeight(100, 0)).toBe(0)
  })

  it("places a time inside the range across the plot width", () => {
    expect(timeToX(1500, 1000, 2000, 400)).toBe(200)
    expect(timeToX(1000, 1000, 2000, 400)).toBe(0)
    expect(timeToX(2000, 1000, 2000, 400)).toBe(400)
    expect(timeToX(999, 1000, 2000, 400)).toBeNull()
    expect(timeToX(2001, 1000, 2000, 400)).toBeNull()
    expect(timeToX(1000, 1000, 1000, 400)).toBeNull()
  })

  it("shows text by mode and segment width", () => {
    expect(showsText("never", 500, 30)).toBe(false)
    expect(showsText("auto", 39, 30)).toBe(false)
    expect(showsText("auto", 40, 30)).toBe(true)
    expect(showsText("always", 10, 30)).toBe(true)
    expect(showsText("always", 2, 30)).toBe(false)
  })

  it("hides text in bars too short to hold it", () => {
    expect(showsText("always", 200, 13)).toBe(false)
    expect(showsText("always", 200, 14)).toBe(true)
  })

  it("keeps twelve legend items and counts the rest", () => {
    const states = Array.from({ length: 15 }, (_, index) => ({
      text: `s${index}`,
      color: "red",
    }))
    const { shown, more, labels } = legendItems(states)
    expect(shown).toHaveLength(12)
    expect(more).toBe(3)
    expect(labels.at(-1)).toBe("and 3 more")
    expect(legendItems(states.slice(0, 4))).toMatchObject({ more: 0 })
    expect(legendItems(states.slice(0, 4)).labels).toHaveLength(4)
  })

  it("estimates the legend lines from the width", () => {
    expect(legendHeight([], 300)).toBe(0)
    expect(legendHeight(["Up", "Down"], 600)).toBe(26)
    expect(legendHeight(["Up", "Down", "Unknown"], 100)).toBe(62)
  })
})
