import { describe, expect, it } from "vitest"

import { dataPanelTypes } from "@/spec"

import {
  editInJsonKeys,
  formSchema,
  optionGroupsFor,
  optionLabels,
  optionSchemas,
  ownEditorKeys,
  valueLabel,
} from "./groups"

const formKinds = ["toggle", "enum", "number", "text", "color"]

describe("option groups", () => {
  it("places every Spec field and panel option in a group, its own editor or the JSON view", () => {
    const grouped = new Set(
      dataPanelTypes.flatMap((type) =>
        optionGroupsFor(type).flatMap((group) => [...group.keys])
      )
    )
    const handled = new Set<string>([
      ...grouped,
      ...ownEditorKeys,
      ...editInJsonKeys,
    ])
    expect(
      Object.keys(optionSchemas).filter((key) => !handled.has(key))
    ).toEqual([])
  })

  it("offers only keys the form can draw", () => {
    for (const type of dataPanelTypes) {
      for (const group of optionGroupsFor(type)) {
        for (const key of group.keys) {
          expect(formKinds, `${type} ${key}`).toContain(formSchema(key).kind)
          expect(optionLabels[key], `${type} ${key}`).not.toBe("")
        }
      }
    }
  })

  it("leaves the JSON-only keys out of the form", () => {
    for (const key of editInJsonKeys) {
      expect(optionSchemas[key].kind).toBe("json")
    }
  })

  it("never shows a key twice for one panel type", () => {
    for (const type of dataPanelTypes) {
      const keys = optionGroupsFor(type).flatMap((group) => [...group.keys])
      expect(new Set(keys).size, type).toBe(keys.length)
    }
  })

  it("gates options by what each renderer draws", () => {
    const keysOf = (type: (typeof dataPanelTypes)[number]) =>
      optionGroupsFor(type).flatMap((group) => [...group.keys])
    expect(keysOf("timeseries")).toContain("colorBy")
    expect(keysOf("trend")).not.toContain("colorBy")
    expect(keysOf("gauge")).toContain("neutral")
    expect(keysOf("bargauge")).not.toContain("neutral")
    expect(keysOf("heatmap")).not.toContain("color")
    expect(keysOf("table")).toContain("filterable")
    expect(keysOf("stat")).not.toContain("axisLabel")
    expect(keysOf("timeseries")).toContain("timezone")
    expect(keysOf("stat")).not.toContain("timezone")
    expect(keysOf("logs")).toEqual(keysOf("alertlist"))
  })
})

describe("valueLabel", () => {
  it("turns Spec enum values into words", () => {
    expect(valueLabel("greenYellowRed")).toBe("Green yellow red")
    expect(valueLabel("stepBefore")).toBe("Step before")
    expect(valueLabel("line+area")).toBe("Line and area")
    expect(valueLabel("h")).toBe("Horizontal")
    expect(valueLabel(10)).toBe("10")
  })
})
