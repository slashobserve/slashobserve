import { describe, expect, it, vi } from "vitest"
import defaultsText from "../../../spec/schema/defaults.json?raw"
import {
  canvasConnectionDefaults,
  canvasElementDefaults,
  displayWithDefaults,
  fieldPropDefaults,
  instantPanelTypes,
  isDefaultDisplayValue,
  linkDefaults,
  queryDefaults,
  rowDefaults,
  validateSpecSchema,
  variableDefaults,
} from "./index"
import { specDefaults } from "./defaults.gen"
import type { DataPanelType } from "./panels"

const panelTypes = Object.keys(specDefaults.display) as Array<
  keyof typeof specDefaults.display
>

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

function staticPanel(type: string, extras: Record<string, unknown> = {}) {
  return {
    id: "p",
    type,
    title: "t",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
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
    expectValid(
      dashboard([
        type === "dashlist" || type === "news"
          ? staticPanel(type, extras)
          : dataPanel(type, extras),
      ])
    )
  })

  it("canvas element and connection defaults are schema valid", () => {
    const element = {
      id: "a",
      kind: "metric",
      left: 0,
      top: 0,
      width: 10,
      height: 10,
      ...canvasElementDefaults,
    }
    const connection = { from: "a", to: "a", ...canvasConnectionDefaults }
    expectValid(
      dashboard([
        dataPanel("canvas", {
          display: { elements: [element], connections: [connection] },
        }),
      ])
    )
  })

  it("instant panel types are schema data panel types", () => {
    expect(instantPanelTypes).toContain("table")
    expect(instantPanelTypes).toContain("alertlist")
    expect(instantPanelTypes).not.toContain("timeseries")
    const types = Object.keys(specDefaults.display)
    for (const type of instantPanelTypes) expect(types).toContain(type)
  })

  it("dashlist defaults apply through displayWithDefaults", () => {
    expect(displayWithDefaults("dashlist", { limit: 5 })).toEqual({
      tags: [],
      limit: 5,
      keepTime: false,
      includeVars: false,
    })
    expect(isDefaultDisplayValue("dashlist", "limit", 10)).toBe(true)
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

  it("displayWithDefaults lets a cleared option fall back to its default", () => {
    expect(
      displayWithDefaults("timeseries", { legend: undefined, tooltip: "all" })
    ).toEqual({ legend: "bottom", legendValues: [], tooltip: "all" })
  })

  it("displayWithDefaults keeps a __proto__ key as plain data", () => {
    const given = JSON.parse(
      '{"__proto__": {"polluted": true}, "tooltip": "all"}'
    )
    const merged = displayWithDefaults("timeseries", given)
    expect(Object.getPrototypeOf(merged)).toBe(Object.prototype)
    expect(Object.hasOwn(merged, "__proto__")).toBe(true)
    expect(Object.hasOwn(merged, "polluted")).toBe(false)
    expect(merged.tooltip).toBe("all")
    expect(merged.legend).toBe("bottom")
  })

  it("an unknown panel type has no defaults and never throws", () => {
    const unknownType = "nope" as DataPanelType
    expect(displayWithDefaults(unknownType, { legend: "right" })).toEqual({
      legend: "right",
    })
    expect(displayWithDefaults(unknownType, undefined)).toEqual({})
    expect(isDefaultDisplayValue(unknownType, "legend", "bottom")).toBe(false)
    const inherited = "constructor" as DataPanelType
    expect(displayWithDefaults(inherited, undefined)).toEqual({})
    expect(isDefaultDisplayValue(inherited, "name", "Object")).toBe(false)
  })

  it("copies and compares object defaults deeply", async () => {
    vi.resetModules()
    vi.doMock("./defaults.gen", () => ({
      specDefaults: {
        fieldProps: {},
        query: {},
        variable: {},
        row: {},
        link: {},
        display: { timeseries: { range: { min: 0, tags: [{ id: 1 }] } } },
      },
    }))
    const mod = await import("./defaults")
    vi.doUnmock("./defaults.gen")
    vi.resetModules()

    const first = mod.displayWithDefaults("timeseries", {}) as {
      range: { min: number; tags: { id: number }[] }
    }
    first.range.min = 5
    first.range.tags[0].id = 9
    expect(mod.displayWithDefaults("timeseries", {})).toEqual({
      range: { min: 0, tags: [{ id: 1 }] },
    })

    const same = { tags: [{ id: 1 }], min: 0 }
    expect(mod.isDefaultDisplayValue("timeseries", "range", same)).toBe(true)
    expect(
      mod.isDefaultDisplayValue("timeseries", "range", {
        min: 0,
        tags: [{ id: 2 }],
      })
    ).toBe(false)
    expect(
      mod.isDefaultDisplayValue("timeseries", "range", { min: 0, tags: [] })
    ).toBe(false)
    expect(mod.isDefaultDisplayValue("timeseries", "range", [])).toBe(false)
  })
})
