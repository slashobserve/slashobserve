import { describe, expect, it } from "vitest"

import { labelsToFields } from "./labels-to-fields"
import type { PreparedFrame } from "./names"

const series: PreparedFrame = {
  ref: "A",
  name: "up",
  labels: { pod: "web-1", namespace: "shop", job: "kube" },
  fields: [
    { name: "Time", type: "time", values: [1, 2] },
    { name: "Value", type: "number", values: [10, 11] },
  ],
}

describe("labelsToFields", () => {
  it("adds one constant string column per label in columns mode", () => {
    const [out] = labelsToFields([series], { type: "labelsToFields" })
    expect(out.labels).toBeUndefined()
    expect(out.fields.map((field) => field.name)).toEqual([
      "Time",
      "Value",
      "job",
      "namespace",
      "pod",
    ])
    expect(out.fields[4].values).toEqual(["web-1", "web-1"])
    expect(out.fields[1].seriesName).toBe("Value")
  })

  it("keeps only the listed labels in the order listed", () => {
    const [out] = labelsToFields([series], {
      type: "labelsToFields",
      include: ["pod", "missing", "namespace"],
    })
    expect(out.fields.slice(2).map((field) => field.name)).toEqual([
      "pod",
      "namespace",
    ])
  })

  it("names the value field after the value label and drops that column", () => {
    const [out] = labelsToFields([series], {
      type: "labelsToFields",
      valueLabel: "pod",
    })
    expect(out.fields[1].name).toBe("web-1")
    expect(out.fields[1].seriesName).toBe("web-1")
    expect(out.fields.map((field) => field.name)).not.toContain("pod")
  })

  it("writes label and value rows in rows mode", () => {
    const [out] = labelsToFields([series], {
      type: "labelsToFields",
      mode: "rows",
      include: ["pod", "namespace"],
    })
    expect(out.fields).toEqual([
      { name: "label", type: "string", values: ["pod", "namespace"] },
      { name: "value", type: "string", values: ["web-1", "shop"] },
    ])
  })

  it("passes frames without labels through and leaves its input alone", () => {
    const plain: PreparedFrame = {
      ref: "B",
      fields: [{ name: "x", type: "number", values: [1] }],
    }
    const input = Object.freeze({
      ...series,
      fields: Object.freeze([...series.fields]) as PreparedFrame["fields"],
    })
    const out = labelsToFields([plain, input], { type: "labelsToFields" })
    expect(out[0]).toEqual(plain)
    expect(out[0]).not.toBe(plain)
    expect(input.labels).toEqual({
      pod: "web-1",
      namespace: "shop",
      job: "kube",
    })
  })
})
