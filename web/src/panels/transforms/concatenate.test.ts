import { describe, expect, it } from "vitest"

import { concatenate } from "./concatenate"
import type { PreparedFrame } from "./names"

const frames: PreparedFrame[] = [
  {
    ref: "A",
    name: "cpu",
    fields: [
      { name: "Time", type: "time", values: [1, 2, 3] },
      { name: "Value", type: "number", values: [4, 5, 6] },
    ],
  },
  {
    ref: "B",
    name: "mem",
    fields: [{ name: "Value", type: "number", values: [7, 8] }],
  },
]

describe("concatenate", () => {
  it("puts every field side by side and pads short ones with null", () => {
    const [out] = concatenate(frames, { type: "concatenate" })
    expect(out.ref).toBe("A")
    expect(out.fields.map((field) => field.name)).toEqual([
      "Time",
      "Value",
      "Value",
    ])
    expect(out.fields[2].values).toEqual([7, 8, null])
  })

  it("prefixes field names with the frame name", () => {
    const [out] = concatenate(frames, {
      type: "concatenate",
      nameMode: "field",
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "cpu Time",
      "cpu Value",
      "mem Value",
    ])
    expect(out.fields[2].seriesName).toBe("mem Value")
  })

  it("adds the frame name as a label on the series name", () => {
    const [out] = concatenate(frames, {
      type: "concatenate",
      nameMode: "label",
    })
    expect(out.fields[1].seriesName).toBe('cpu{frame="cpu"}')
    const [custom] = concatenate(frames, {
      type: "concatenate",
      nameMode: "label",
      nameLabel: "query",
    })
    expect(custom.fields[2].seriesName).toBe('mem{query="mem"}')
  })

  it("gives nothing for no frames", () => {
    expect(concatenate([], { type: "concatenate" })).toEqual([])
  })
})
