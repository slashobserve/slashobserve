import { describe, expect, it } from "vitest"

import {
  arrowEnds,
  connectionPath,
  dashArray,
  lineCap,
  scaledWidth,
} from "./connections"

const p = (x: number, y: number) => ({ x, y })

describe("connectionPath", () => {
  it("draws a straight line between two points", () => {
    expect(connectionPath([p(0, 0), p(100, 50)], "straight")).toBe(
      "M 0 0 L 100 50"
    )
  })

  it("bends a straight line at its vertices", () => {
    expect(connectionPath([p(0, 0), p(50, 50), p(100, 0)], "straight")).toBe(
      "M 0 0 L 50 50 L 100 0"
    )
  })

  it("draws a curve with no vertices as a straight cubic", () => {
    expect(connectionPath([p(0, 0), p(90, 30)], "curved")).toBe(
      "M 0 0 C 30 10 60 20 90 30"
    )
  })

  it("passes a curve through its vertices with a Catmull-Rom spline", () => {
    expect(connectionPath([p(0, 0), p(60, 60), p(120, 0)], "curved")).toBe(
      "M 0 0 C 20 20 40 60 60 60 C 80 60 100 20 120 0"
    )
  })

  it("draws nothing for fewer than two points", () => {
    expect(connectionPath([p(1, 1)], "straight")).toBe("")
    expect(connectionPath([], "curved")).toBe("")
  })

  it("rounds coordinates to two decimals", () => {
    expect(connectionPath([p(0, 0), p(1 / 3, 2 / 3)], "straight")).toBe(
      "M 0 0 L 0.33 0.67"
    )
  })
})

describe("line styles", () => {
  it("dashes with 6 4 and dots with 1 3 and round caps", () => {
    expect(dashArray("solid")).toBeNull()
    expect(dashArray("dashed")).toBe("6 4")
    expect(dashArray("dotted")).toBe("1 3")
    expect(lineCap("solid")).toBe("butt")
    expect(lineCap("dashed")).toBe("butt")
    expect(lineCap("dotted")).toBe("round")
  })
})

describe("arrowEnds", () => {
  it.each([
    ["none", false, false],
    ["forward", false, true],
    ["reverse", true, false],
    ["both", true, true],
  ] as const)("%s", (arrow, start, end) => {
    expect(arrowEnds(arrow)).toEqual({ start, end })
  })
})

describe("scaledWidth", () => {
  const range = { min: 0, max: 100 }

  it("maps the field range linearly onto the width limits", () => {
    expect(scaledWidth(0, range, { min: 1, max: 10 })).toBe(1)
    expect(scaledWidth(100, range, { min: 1, max: 10 })).toBe(10)
    expect(scaledWidth(50, range, { min: 1, max: 10 })).toBe(5.5)
    expect(scaledWidth(50, range, { min: 2, max: 6 })).toBe(4)
  })

  it("defaults the limits to 1 and 10", () => {
    expect(scaledWidth(0, range, {})).toBe(1)
    expect(scaledWidth(100, range, {})).toBe(10)
  })

  it("holds values outside the range at the limits", () => {
    expect(scaledWidth(-20, range, { min: 2, max: 6 })).toBe(2)
    expect(scaledWidth(500, range, { min: 2, max: 6 })).toBe(6)
  })

  it("uses the narrow width for a constant field", () => {
    expect(scaledWidth(5, { min: 5, max: 5 }, { min: 2, max: 6 })).toBe(2)
  })

  it("tolerates limits written the wrong way round", () => {
    expect(scaledWidth(100, range, { min: 8, max: 3 })).toBe(8)
    expect(scaledWidth(0, range, { min: 8, max: 3 })).toBe(3)
  })
})
