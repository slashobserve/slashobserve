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
