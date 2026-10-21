import { describe, expect, it } from "vitest"

import { stackDirection, stackSeries } from "./stack"

describe("stackSeries", () => {
  it("sums normal stacked series in order", () => {
    const { drawn } = stackSeries(
      [
        [1, 2],
        [10, 20],
        [100, 200],
      ],
      ["normal", "normal", "normal"],
      ["y", "y", "y"]
    )
    expect(drawn).toEqual([
      [1, 2],
      [11, 22],
      [111, 222],
    ])
  })

  it("keeps a gap as null while later series still stack on the others", () => {
    const { drawn } = stackSeries(
      [
        [1, null],
        [10, 20],
      ],
      ["normal", "normal"],
      ["y", "y"]
    )
    expect(drawn).toEqual([
      [1, null],
      [11, 20],
    ])
  })

  it("keeps an unmatched slot undefined so the line spans it", () => {
    const { drawn } = stackSeries(
      [
        [1, undefined, 3],
        [10, 20, 30],
      ],
      ["normal", "normal"],
      ["y", "y"]
    )
    expect(drawn).toEqual([
      [1, undefined, 3],
      [11, 20, 33],
    ])
  })

  it("normalises percent stacks so the last series reaches 1", () => {
    const { drawn } = stackSeries(
      [
        [1, 3],
        [3, 1],
      ],
      ["percent", "percent"],
      ["y", "y"]
    )
    expect(drawn).toEqual([
      [0.25, 0.75],
      [1, 1],
    ])
  })

  it("keeps the axes in separate groups", () => {
    const { drawn } = stackSeries(
      [
        [1, 1],
        [2, 2],
        [4, 4],
      ],
      ["normal", "normal", "normal"],
      ["y", "y2", "y"]
    )
    expect(drawn).toEqual([
      [1, 1],
      [2, 2],
      [5, 5],
    ])
  })

  it("keeps normal and percent groups on one axis apart", () => {
    const { drawn } = stackSeries(
      [
        [2, 2],
        [2, 2],
        [6, 6],
      ],
      ["normal", "percent", "normal"],
      ["y", "y", "y"]
    )
    expect(drawn).toEqual([
      [2, 2],
      [1, 1],
      [8, 8],
    ])
  })

  it("leaves unstacked series as given", () => {
    const values = [[5, null, 7]]
    expect(stackSeries(values, ["none"], ["y"]).drawn).toEqual(values)
  })

  it("stacks negative series downwards from zero, apart from the positive ones", () => {
    const { drawn } = stackSeries(
      [
        [10, 10],
        [-5, -5],
        [3, 3],
        [-2, -2],
      ],
      ["normal", "normal", "normal", "normal"],
      ["y", "y", "y", "y"]
    )
    expect(drawn).toEqual([
      [10, 10],
      [-5, -5],
      [13, 13],
      [-7, -7],
    ])
  })

  it("divides each direction of a percent stack by its own total", () => {
    const { drawn } = stackSeries(
      [
        [6, 6],
        [-1, -1],
        [2, 2],
        [-3, -3],
      ],
      ["percent", "percent", "percent", "percent"],
      ["y", "y", "y", "y"]
    )
    expect(drawn).toEqual([
      [0.75, 0.75],
      [-0.25, -0.25],
      [1, 1],
      [-1, -1],
    ])
  })

  it("draws a lone negative percent series at minus one", () => {
    const { drawn } = stackSeries(
      [
        [10, 10],
        [-4, -4],
      ],
      ["percent", "percent"],
      ["y", "y"]
    )
    expect(drawn).toEqual([
      [1, 1],
      [-1, -1],
    ])
  })

  it("points each stacked series at the one below it in its own group", () => {
    const { below } = stackSeries(
      [[1], [2], [3], [-1], [4], [-2], [5]],
      ["normal", "none", "normal", "normal", "percent", "normal", "normal"],
      ["y", "y", "y", "y", "y", "y", "y2"]
    )
    expect(below).toEqual([null, null, 0, null, null, 3, null])
  })

  it("reports each series' stack direction", () => {
    const { directions } = stackSeries(
      [[1], [-1], [2]],
      ["normal", "normal", "none"],
      ["y", "y", "y"]
    )
    expect(directions.slice(0, 2)).toEqual([1, -1])
  })
})

describe("stackDirection", () => {
  it("goes down when negative samples outnumber positive ones", () => {
    expect(stackDirection([-1, -2, 3])).toBe(-1)
    expect(stackDirection([1, 2, -3])).toBe(1)
  })

  it("goes up on a tie and when there are no samples", () => {
    expect(stackDirection([1, -1])).toBe(1)
    expect(stackDirection([])).toBe(1)
    expect(stackDirection([null, undefined, null])).toBe(1)
  })

  it("counts negative zero as negative and skips leading and trailing gaps", () => {
    expect(stackDirection([null, -0, null])).toBe(-1)
    expect(stackDirection([null, null, -1, 1, 2, null])).toBe(1)
  })

  it("looks at no more than about a hundred samples", () => {
    const values = Array.from({ length: 200 }, (_, at) =>
      at % 2 === 0 ? -1 : 1
    )
    expect(stackDirection(values)).toBe(-1)
  })
})
