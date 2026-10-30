import { describe, expect, it } from "vitest"

import { displayKeysByType, type DisplayPanelType } from "@/spec"

import { displayOptions } from "./index"

const geomapKeysTheEditorPlansDeclareLater = [
  "fitLayer",
  "fitLastOnly",
  "sharedView",
  "layers",
]

const canvasKeysEditedInTheJsonView = ["backgroundImage", "backgroundSize"]

const editedElsewhere: Partial<Record<DisplayPanelType, string[]>> = {
  geomap: geomapKeysTheEditorPlansDeclareLater,
  xychart: ["series"],
  canvas: ["elements", "connections", ...canvasKeysEditedInTheJsonView],
}

const types = Object.keys(displayOptions) as DisplayPanelType[]

describe("displayOptions", () => {
  it("covers every type the schema gives display keys", () => {
    expect([...types].sort()).toEqual(Object.keys(displayKeysByType).sort())
  })

  it.each(types)("%s declares only keys the schema allows", (type) => {
    const declared = displayOptions[type].flatMap((option) =>
      "key" in option ? [option.key] : []
    )
    for (const key of declared) {
      expect(displayKeysByType[type]).toContain(key)
    }
    expect(new Set(declared).size).toBe(declared.length)
  })

  it.each(types)("%s declares every key the schema allows", (type) => {
    const declared = new Set(
      displayOptions[type].flatMap((option) =>
        "key" in option ? [option.key] : []
      )
    )
    const skipped = new Set(editedElsewhere[type] ?? [])
    const missing = displayKeysByType[type].filter(
      (key) => !declared.has(key) && !skipped.has(key)
    )
    expect(missing).toEqual([])
  })
})
