import { describe, expect, it } from "vitest"

import { displayKeysByType, type DisplayPanelType } from "@/spec"

import schemaText from "../../../../spec/schema/observespec.v0.json?raw"

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

type SchemaNumber = {
  minimum?: number
  exclusiveMinimum?: number
  maximum?: number
  type?: string
}

type SchemaBranch = {
  if?: { properties?: { type?: { const?: string } } }
  then?: { properties?: { display?: { properties?: Record<string, unknown> } } }
}

const schemaBranches = (
  JSON.parse(schemaText) as { $defs: { dataPanel: { allOf: SchemaBranch[] } } }
).$defs.dataPanel.allOf

function schemaDisplay(type: string): Record<string, unknown> {
  const branch = schemaBranches.find(
    (item) => item.if?.properties?.type?.const === type
  )
  return branch?.then?.properties?.display?.properties ?? {}
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

  it.each(types)("%s number inputs match the schema ranges", (type) => {
    const display = schemaDisplay(type)
    for (const option of displayOptions[type]) {
      if (option.kind !== "number") continue
      const bounds = display[option.key] as SchemaNumber | undefined
      if (bounds?.maximum === undefined) continue
      expect(
        {
          key: option.key,
          min: option.rule.minExclusive ? undefined : option.rule.min,
          max: option.rule.max,
        },
        option.key
      ).toEqual({
        key: option.key,
        min: bounds.exclusiveMinimum === undefined ? bounds.minimum : undefined,
        max: bounds.maximum,
      })
    }
  })

  it.each(types)(
    "%s offers the text color wherever it offers a color",
    (type) => {
      const display = schemaDisplay(type)
      for (const option of displayOptions[type]) {
        if (option.kind !== "choice") continue
        const property = display[option.key] as { $ref?: string } | undefined
        if (property?.$ref !== "#/$defs/color") continue
        expect(
          option.options.map((item) => item.value),
          option.key
        ).toContain("text")
      }
    }
  )
})
