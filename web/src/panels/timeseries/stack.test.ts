import { describe, expect, it } from "vitest"

import { stackBelow, stackSeries } from "./stack"

describe("stackSeries", () => {
  it("sums normal stacked series in order", () => {
    const drawn = stackSeries(
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
    const drawn = stackSeries(
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

  it("normalises percent stacks so the last series reaches 1", () => {
    const drawn = stackSeries(
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
    const drawn = stackSeries(
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
    const drawn = stackSeries(
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
    expect(stackSeries(values, ["none"], ["y"])).toEqual(values)
  })
})

describe("stackBelow", () => {
  it("points each stacked series at the one below it in its group", () => {
    expect(
      stackBelow(
        ["normal", "none", "normal", "percent", "normal"],
        ["y", "y", "y", "y", "y2"]
      )
    ).toEqual([null, null, 0, null, null])
  })
})
