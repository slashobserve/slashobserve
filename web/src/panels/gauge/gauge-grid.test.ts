import { describe, expect, it } from "vitest"

import { gaugeGrid } from "./gauge-grid"

describe("gaugeGrid", () => {
  it.each([
    ["one gauge in the body of an h4 w3 panel", 1, 150, 101, 1, 1],
    ["one gauge in a wide body", 1, 800, 200, 1, 1],
    ["two gauges side by side in a wide short body", 2, 400, 100, 2, 1],
    ["two gauges stacked in a tall narrow body", 2, 100, 400, 1, 2],
    ["four gauges in a square body", 4, 400, 400, 2, 2],
    ["three gauges in a wide short body", 3, 600, 200, 3, 1],
    ["six gauges in a 3 by 2 body", 6, 300, 200, 3, 2],
    ["five gauges in a very wide body", 5, 1000, 300, 5, 1],
    ["five gauges in a tall body", 5, 200, 800, 1, 5],
    ["seven gauges that leave a short last row", 7, 360, 240, 4, 2],
    ["many gauges in a small body", 20, 150, 101, 5, 4],
    ["no gauge", 0, 300, 200, 1, 1],
  ])("lays %s out", (_name, count, width, height, columns, rows) => {
    expect(gaugeGrid(count, width, height)).toEqual({ columns, rows })
  })
})

describe("gaugeGrid with size limits", () => {
  it.each([
    [
      "no limit that bites",
      4,
      400,
      400,
      { minWidth: 100, minHeight: 100 },
      2,
      2,
    ],
    [
      "a minimum width that rules out the widest grid",
      4,
      400,
      100,
      { minWidth: 150 },
      2,
      2,
    ],
    [
      "a minimum height that rules out many rows",
      4,
      400,
      400,
      { minHeight: 300 },
      4,
      1,
    ],
    [
      "limits that fit nowhere fill the width and scroll",
      6,
      300,
      200,
      { minWidth: 120, minHeight: 150 },
      2,
      3,
    ],
    [
      "a minimum wider than the body keeps one column",
      3,
      100,
      200,
      { minWidth: 300, minHeight: 300 },
      1,
      3,
    ],
    [
      "more columns than gauges are never used",
      2,
      1000,
      100,
      { minWidth: 10, minHeight: 300 },
      2,
      1,
    ],
  ])("lays out %s", (_name, count, width, height, limits, columns, rows) => {
    expect(gaugeGrid(count, width, height, limits)).toEqual({ columns, rows })
  })
})
