import { describe, expect, it } from "vitest"

import { displayWithDefaults } from "@/spec"

import {
  barSpan,
  blendOver,
  borderWidth,
  labelWidth,
  rowHeight,
  segmentOpacity,
  segmentSpan,
  showsText,
  textPlacement,
  timelineDisplay,
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
})

describe("barSpan", () => {
  it("leaves a 2 pixel gap under the bar when no row height is written", () => {
    expect(barSpan(32, undefined)).toEqual({ offset: 0, size: 30 })
    expect(barSpan(2, undefined)).toEqual({ offset: 0, size: 1 })
  })

  it("takes the written share of the row and centers the bar in it", () => {
    expect(barSpan(30, 0.5)).toEqual({ offset: 7.5, size: 15 })
    expect(barSpan(30, 1)).toEqual({ offset: 0, size: 30 })
    expect(barSpan(4, 0.1)).toEqual({ offset: 1.5, size: 1 })
  })
})

describe("segmentSpan", () => {
  it("keeps the 1 pixel trailing gap of a segment and the 1 pixel inset of a cell", () => {
    expect(segmentSpan(100, 40, undefined, false)).toEqual({
      offset: 100,
      size: 39,
    })
    expect(segmentSpan(100, 40, undefined, true)).toEqual({
      offset: 101,
      size: 37,
    })
    expect(segmentSpan(100, 1, undefined, true).size).toBe(1)
  })

  it("takes the written share of the width and centers it", () => {
    expect(segmentSpan(100, 40, 0.5, true)).toEqual({ offset: 110, size: 20 })
    expect(segmentSpan(100, 40, 1, true)).toEqual({ offset: 100, size: 40 })
    expect(segmentSpan(100, 2, 0.1, true).size).toBe(1)
  })
})

describe("segmentOpacity and borderWidth", () => {
  it("draws opaque unless a fill above 0 is written", () => {
    expect(segmentOpacity(undefined)).toBe(1)
    expect(segmentOpacity(0)).toBe(1)
    expect(segmentOpacity(70)).toBe(0.7)
    expect(segmentOpacity(100)).toBe(1)
  })

  it("draws no border unless a line width is written", () => {
    expect(borderWidth(undefined)).toBe(0)
    expect(borderWidth(0)).toBe(0)
    expect(borderWidth(3)).toBe(3)
  })
})

describe("blendOver", () => {
  it("mixes a color with the backdrop by opacity", () => {
    expect(blendOver([0, 100, 200], [255, 255, 255], 1)).toEqual([0, 100, 200])
    expect(blendOver([0, 100, 200], [255, 255, 255], 0)).toEqual([
      255, 255, 255,
    ])
    expect(blendOver([0, 100, 200], [200, 100, 0], 0.5)).toEqual([
      100, 100, 100,
    ])
  })
})

describe("textPlacement", () => {
  it("anchors text to the left, middle or right of a segment", () => {
    expect(textPlacement("left", 100)).toEqual({ x: 6, anchor: "start" })
    expect(textPlacement("center", 100)).toEqual({ x: 50, anchor: "middle" })
    expect(textPlacement("right", 100)).toEqual({ x: 94, anchor: "end" })
  })
})

describe("timelineDisplay", () => {
  it("reads the defaults of a state timeline", () => {
    expect(timelineDisplay(displayWithDefaults("state-timeline", {}))).toEqual({
      showValues: "auto",
      rowHeight: undefined,
      columnWidth: undefined,
      alignValue: "left",
      legend: "bottom",
      legendMode: undefined,
      legendWidth: undefined,
      tooltip: "single",
      tooltipSort: "desc",
      tooltipHideZeros: false,
      tooltipMaxWidth: undefined,
      tooltipMaxHeight: undefined,
    })
  })

  it("falls back to left alignment where the type has none", () => {
    expect(
      timelineDisplay(displayWithDefaults("status-history", {})).alignValue
    ).toBe("left")
  })

  it("passes the written options through", () => {
    const written = timelineDisplay(
      displayWithDefaults("state-timeline", {
        rowHeight: 0.5,
        alignValue: "right",
        legend: "right",
        legendMode: "table",
        tooltip: "all",
        tooltipSort: "asc",
      })
    )
    expect(written).toMatchObject({
      rowHeight: 0.5,
      alignValue: "right",
      legend: "right",
      legendMode: "table",
      tooltip: "all",
      tooltipSort: "asc",
    })
  })
})
