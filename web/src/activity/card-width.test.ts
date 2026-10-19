import { describe, expect, it } from "vitest"

import {
  activityWidth,
  clampWidth,
  editWidth,
  isStacked,
  maxWidthFor,
} from "./card-width"

describe("clampWidth", () => {
  it("keeps a width inside the bounds", () => {
    expect(clampWidth(480, activityWidth, 1440)).toBe(480)
  })

  it("raises a width below the minimum", () => {
    expect(clampWidth(100, activityWidth, 1440)).toBe(320)
  })

  it("lowers a width above the maximum", () => {
    expect(clampWidth(900, activityWidth, 1440)).toBe(640)
    expect(clampWidth(2000, editWidth, 3000)).toBe(1040)
  })

  it("leaves room for the dashboard on a narrower viewport", () => {
    expect(clampWidth(760, editWidth, 1280)).toBe(710)
    expect(clampWidth(640, activityWidth, 1000)).toBe(430)
  })

  it("keeps the minimum when the viewport is too small for any card", () => {
    expect(clampWidth(500, activityWidth, 700)).toBe(320)
    expect(clampWidth(900, editWidth, 400)).toBe(560)
  })

  it("falls back to the initial width for a width that is not a number", () => {
    expect(clampWidth(Number.NaN, activityWidth, 1440)).toBe(400)
    expect(clampWidth(Number.POSITIVE_INFINITY, editWidth, 1440)).toBe(760)
  })

  it("rounds to whole pixels", () => {
    expect(clampWidth(400.6, activityWidth, 1440)).toBe(401)
  })
})

describe("maxWidthFor", () => {
  it("never goes below the minimum", () => {
    expect(maxWidthFor(activityWidth, 100)).toBe(320)
  })
})

describe("isStacked", () => {
  it("stacks the card under the dashboard when even the narrowest card does not fit", () => {
    expect(isStacked(activityWidth, 889)).toBe(true)
    expect(isStacked(activityWidth, 890)).toBe(false)
    expect(isStacked(editWidth, 1129)).toBe(true)
    expect(isStacked(editWidth, 1130)).toBe(false)
  })
})
