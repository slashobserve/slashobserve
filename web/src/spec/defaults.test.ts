import { describe, expect, it } from "vitest"
import defaultsText from "../../../spec/schema/defaults.json?raw"
import {
  displayWithDefaults,
  fieldPropDefaults,
  isDefaultDisplayValue,
  linkDefaults,
  queryDefaults,
  rowDefaults,
  validateSpecSchema,
  variableDefaults,
} from "./index"
import { specDefaults } from "./defaults.gen"
import type { DataPanel } from "./observespec.gen"

const panelTypes = Object.keys(specDefaults.display) as DataPanel["type"][]

function dashboard(panels: unknown[], extras: Record<string, unknown> = {}) {
  return {
    specVersion: "0",
    title: "t",
    time: { from: "now-1h", to: "now" },
    panels,
    ...extras,
  }
}

function dataPanel(type: string, extras: Record<string, unknown> = {}) {
  return {
    id: "p",
    type,
    title: "t",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extras,
  }
}

function expectValid(value: unknown) {
  const result = validateSpecSchema(value)
  expect(result.valid ? [] : result.issues).toEqual([])
}

describe("spec defaults", () => {
  it("generated copy equals spec/schema/defaults.json", () => {
    expect(specDefaults).toEqual(JSON.parse(defaultsText))
  })

  it.each(panelTypes)("%s display defaults are schema valid", (type) => {
    const display = specDefaults.display[type]
    const extras = Object.keys(display).length > 0 ? { display } : {}
    expectValid(dashboard([dataPanel(type, extras)]))
  })

  it("field property defaults are schema valid", () => {
    expectValid(dashboard([dataPanel("timeseries", { ...fieldPropDefaults })]))
  })

  it("query defaults are schema valid", () => {
    expectValid(
      dashboard([
        dataPanel("timeseries", {
          queries: [{ query: "up", ...queryDefaults }],
        }),
      ])
    )
  })

  it("variable defaults are schema valid", () => {
    expectValid(
      dashboard([dataPanel("timeseries")], {
        variables: [
          {
            name: "job",
            type: "query",
            datasource: "prom",
            query: "label_values(up, job)",
            ...variableDefaults,
          },
        ],
      })
    )
  })

  it("row defaults are schema valid", () => {
    expectValid(
      dashboard([
        {
          id: "r",
          type: "row",
          title: "Row",
          gridPos: { x: 0, y: 0, w: 24, h: 1 },
          ...rowDefaults,
          panels: [
            dataPanel("timeseries", { gridPos: { x: 0, y: 1, w: 12, h: 8 } }),
          ],
        },
      ])
    )
  })

  it("link defaults are schema valid", () => {
    expectValid(
      dashboard([dataPanel("timeseries")], {
        links: [{ title: "Docs", url: "https://example.com", ...linkDefaults }],
      })
    )
  })

  it("isDefaultDisplayValue compares against the default", () => {
    expect(isDefaultDisplayValue("timeseries", "legend", "bottom")).toBe(true)
    expect(isDefaultDisplayValue("timeseries", "legend", "right")).toBe(false)
    expect(isDefaultDisplayValue("stat", "textMode", "value")).toBe(false)
    expect(isDefaultDisplayValue("timeseries", "legendValues", [])).toBe(true)
    expect(isDefaultDisplayValue("timeseries", "legendValues", ["max"])).toBe(
      false
    )
  })

  it("displayWithDefaults fills absent keys and keeps given ones", () => {
    expect(displayWithDefaults("timeseries", undefined)).toEqual(
      specDefaults.display.timeseries
    )
    expect(displayWithDefaults("timeseries", { legend: "right" })).toEqual({
      legend: "right",
      legendValues: [],
      tooltip: "single",
    })
  })

  it("displayWithDefaults hands out copies of default arrays", () => {
    const first = displayWithDefaults("timeseries", {})
    ;(first.legendValues as string[]).push("max")
    expect(displayWithDefaults("timeseries", {}).legendValues).toEqual([])
  })
})
