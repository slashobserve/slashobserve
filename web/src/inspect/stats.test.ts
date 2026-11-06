import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "@/panels/transforms/names"

import { frameStats, queryVariables } from "./stats"

describe("frameStats", () => {
  it("counts nothing for no frames", () => {
    expect(frameStats([])).toEqual({
      frames: 0,
      series: 0,
      points: 0,
      rows: 0,
      minTime: null,
      maxTime: null,
    })
  })

  it("counts series, points without gaps, rows and the time span", () => {
    const frames: PreparedFrame[] = [
      {
        ref: "A",
        fields: [
          { name: "Time", type: "time", values: [300, 100, 200] },
          { name: "a", type: "number", values: [1, null, 3] },
          { name: "b", type: "number", values: [4, 5, 6] },
        ],
      },
      {
        ref: "B",
        fields: [
          { name: "Time", type: "time", values: [50, null] },
          { name: "pod", type: "string", values: ["x", "y"] },
        ],
      },
    ]
    expect(frameStats(frames)).toEqual({
      frames: 2,
      series: 2,
      points: 5,
      rows: 5,
      minTime: 50,
      maxTime: 300,
    })
  })
})

describe("queryVariables", () => {
  const names = ["pod", "pods", "env", "a.b"]

  it.each([
    ['rate(x{pod=~"$pod"}[5m])', ["pod"]],
    ["sum by ($pods) (x)", ["pods"]],
    ['x{env="${env}"}', ["env"]],
    ['x{env=~"${env:regex}"}', ["env"]],
    ['x{pod="[[pod]]"}', ["pod"]],
    ['x{pod="[[pod:csv]]"}', ["pod"]],
    ["select 1", []],
    ["$podx and $envy", []],
    ["${a.b}", ["a.b"]],
  ])("finds the variables in %s", (text, expected) => {
    expect(queryVariables(text, names)).toEqual(expected)
  })
})
