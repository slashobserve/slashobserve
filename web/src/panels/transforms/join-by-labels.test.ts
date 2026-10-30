import { describe, expect, it } from "vitest"

import { joinByLabels } from "./join-by-labels"
import type { PreparedFrame } from "./names"

function series(
  labels: Record<string, string>,
  values: (number | null)[],
  time = [1, 2]
): PreparedFrame {
  return {
    ref: "A",
    labels,
    fields: [
      { name: "Time", type: "time", values: time },
      { name: "Value", type: "number", values },
    ],
  }
}

const frames = [
  series({ instance: "a", mode: "user" }, [1, 2]),
  series({ instance: "a", mode: "system" }, [3, 4]),
  series({ instance: "b", mode: "user" }, [5, 6]),
]

describe("joinByLabels", () => {
  it("makes one column per value label and one row per join key and time", () => {
    const [out] = joinByLabels(frames, {
      type: "joinByLabels",
      value: "mode",
      on: ["instance"],
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "Time",
      "instance",
      "user",
      "system",
    ])
    expect(out.fields[1].values).toEqual(["a", "b", "a", "b"])
    expect(out.fields[2].values).toEqual([1, 5, 2, 6])
    expect(out.fields[3].values).toEqual([3, null, 4, null])
  })

  it("joins on every other label when on is absent", () => {
    const [out] = joinByLabels(frames.slice(0, 2), {
      type: "joinByLabels",
      value: "mode",
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "Time",
      "instance",
      "user",
      "system",
    ])
  })

  it("has no time column for frames without a time field", () => {
    const rows: PreparedFrame[] = [
      {
        ref: "A",
        labels: { job: "x", env: "p" },
        fields: [{ name: "Value", type: "number", values: [7] }],
      },
      {
        ref: "A",
        labels: { job: "y", env: "p" },
        fields: [{ name: "Value", type: "number", values: [8] }],
      },
    ]
    const [out] = joinByLabels(rows, {
      type: "joinByLabels",
      value: "job",
      on: ["env"],
    })
    expect(out.fields).toEqual([
      { name: "env", type: "string", values: ["p"] },
      { name: "x", type: "number", values: [7] },
      { name: "y", type: "number", values: [8] },
    ])
  })

  it("leaves frames that lack the value label as they are", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [{ name: "x", type: "number", values: [1] }],
    }
    const out = joinByLabels([other], { type: "joinByLabels", value: "mode" })
    expect(out).toEqual([other])
  })
})
