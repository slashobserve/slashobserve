import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { timeSeriesTable } from "./time-series-table"

function series(
  ref: string,
  labels: Record<string, string>,
  values: (number | null)[]
): PreparedFrame {
  return {
    ref,
    labels,
    fields: [
      { name: "Time", type: "time", values: values.map((_, i) => i) },
      { name: "Value", type: "number", values },
    ],
  }
}

describe("timeSeriesTable", () => {
  it("makes one row per series with its trend and last value by default", () => {
    const [table] = timeSeriesTable(
      [
        series("A", { pod: "p1" }, [1, 2, null]),
        series("A", { pod: "p2" }, [5, 6, 7]),
      ],
      { type: "timeSeriesTable" }
    )
    expect(table.fields).toEqual([
      { name: "pod", type: "string", values: ["p1", "p2"] },
      {
        name: "Trend #A",
        type: "other",
        values: [
          [1, 2, null],
          [5, 6, 7],
        ],
      },
      { name: "Value #A", type: "number", values: [2, 7] },
    ])
  })

  it("reduces each query with its own stat", () => {
    const out = timeSeriesTable(
      [
        series("A", { pod: "p1" }, [1, 9, 2]),
        series("B", { pod: "p1" }, [1, 9, 2]),
      ],
      { type: "timeSeriesTable", stats: { A: "max", B: "mean" } }
    )
    expect(out.map((frame) => frame.ref)).toEqual(["A", "B"])
    expect(out[0].fields[2].values).toEqual([9])
    expect(out[1].fields[2].values).toEqual([4])
  })

  it("names rows by series when there are no labels and passes tables through", () => {
    const plain: PreparedFrame = {
      ref: "A",
      name: "cpu",
      fields: [
        { name: "Time", type: "time", values: [1] },
        { name: "Value", type: "number", values: [3] },
      ],
    }
    const table: PreparedFrame = {
      ref: "C",
      fields: [{ name: "x", type: "string", values: ["k"] }],
    }
    const out = timeSeriesTable([plain, table], { type: "timeSeriesTable" })
    expect(out[0].fields[0]).toEqual({
      name: "Series",
      type: "string",
      values: ["cpu"],
    })
    expect(out[1]).toEqual(table)
  })
})
