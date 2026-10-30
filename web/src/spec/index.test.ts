import { describe, expect, it } from "vitest"

import variableOptionsText from "../../../spec/examples/variables/options.json?raw"
import { validateSpecSchema } from "./index"

function dashboardWith(panelExtras: Record<string, unknown>) {
  return {
    specVersion: "0",
    title: "Exclusive members",
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "a",
        type: "stat",
        title: "A",
        gridPos: { x: 0, y: 0, w: 6, h: 4 },
        datasource: "prom",
        queries: [{ query: "up" }],
        ...panelExtras,
      },
    ],
  }
}

function issuesFor(panelExtras: Record<string, unknown>) {
  const result = validateSpecSchema(dashboardWith(panelExtras))
  return result.valid ? [] : result.issues
}

describe("exclusive member issues", () => {
  const matchMessage =
    "match must have exactly one of query, series, seriesRegex, field, fieldType, value"
  const mappingMessage =
    "mapping must have exactly one of value, range, regex, special"

  it.each([
    [
      "match with two keys",
      { overrides: [{ match: { query: "A", field: "Value" }, color: "ok" }] },
      "/panels/0/overrides/0/match",
      matchMessage,
    ],
    [
      "empty match",
      { overrides: [{ match: {}, color: "ok" }] },
      "/panels/0/overrides/0/match",
      matchMessage,
    ],
    [
      "mapping with two keys",
      { mappings: [{ value: "a", regex: "b" }] },
      "/panels/0/mappings/0",
      mappingMessage,
    ],
    [
      "mapping with only text",
      { mappings: [{ text: "a" }] },
      "/panels/0/mappings/0",
      mappingMessage,
    ],
  ])("%s collapses to one issue", (_name, extras, path, message) => {
    expect(issuesFor(extras)).toEqual([{ path, message }])
  })

  it("keeps only the real issue for a match with one key and a bad value", () => {
    const issues = issuesFor({
      overrides: [{ match: { query: "1x" }, color: "ok" }],
    })
    expect(issues).toHaveLength(1)
    expect(issues[0].path).toBe("/panels/0/overrides/0/match/query")
  })

  it("keeps only the real issue for a mapping with one key and a bad color", () => {
    const issues = issuesFor({ mappings: [{ value: "1", color: "red" }] })
    expect(issues).toHaveLength(1)
    expect(issues[0].path).toBe("/panels/0/mappings/0/color")
  })

  it("keeps an unknown member of the one key's branch", () => {
    expect(
      issuesFor({
        overrides: [{ match: { query: "A", bogus: 1 }, color: "ok" }],
      })
    ).toEqual([
      {
        path: "/panels/0/overrides/0/match",
        message: 'unknown property "bogus"',
      },
    ])
  })
})

describe("query and expression issues", () => {
  const expression = { type: "reduce", input: "A", reducer: "max" }

  it("accepts a datasource query next to an expression", () => {
    expect(
      issuesFor({
        queries: [
          { ref: "A", query: "up" },
          { ref: "B", expression },
        ],
      })
    ).toEqual([])
  })

  it.each([
    [
      "a query with a bad option",
      [{ query: "up", intervalFactor: 1 }],
      "/panels/0/queries/0/intervalFactor",
    ],
    [
      "an expression with a bad reducer",
      [
        { ref: "A", query: "up" },
        { ref: "B", expression: { ...expression, reducer: "mode" } },
      ],
      "/panels/0/queries/1/expression/reducer",
    ],
    [
      "an expression without a ref",
      [{ query: "up" }, { expression }],
      "/panels/0/queries/1",
    ],
    [
      "an item with neither a query nor an expression",
      [{ query: "up" }, { ref: "E" }],
      "/panels/0/queries/1",
    ],
  ])("reports only the chosen branch for %s", (_name, queries, path) => {
    const issues = issuesFor({ queries })
    expect(issues.length).toBeGreaterThan(0)
    expect(issues.every((issue) => issue.path === path)).toBe(true)
  })

  it("does not blame the query side for an expression with a query key", () => {
    const issues = issuesFor({
      queries: [{ ref: "B", expression, query: "up" }],
    })
    expect(issues).toEqual([
      { path: "/panels/0/queries/0", message: 'unknown property "query"' },
    ])
  })
})

describe("variable sort, custom values and saved filters", () => {
  const variableOptions = JSON.parse(variableOptionsText)

  it("accepts the shared example", () => {
    expect(validateSpecSchema(variableOptions).valid).toBe(true)
  })

  it.each([
    ["an operator the request parameter refuses", { operator: "<" }],
    ["an empty key", { key: "" }],
    ["a value over 500 characters", { value: "x".repeat(501) }],
  ])("refuses a saved filter with %s", (_name, change) => {
    const adhoc = structuredClone(variableOptions)
    Object.assign(adhoc.variables[3].default[0], change)
    expect(validateSpecSchema(adhoc).valid).toBe(false)
  })

  it("refuses more than 20 saved filters", () => {
    const adhoc = structuredClone(variableOptions)
    adhoc.variables[3].default = Array.from({ length: 21 }, () => ({
      key: "job",
      operator: "=",
      value: "api",
    }))
    expect(validateSpecSchema(adhoc).valid).toBe(false)
  })

  it("refuses allowCustomValue on a variable type without values to pick", () => {
    const textbox = structuredClone(variableOptions)
    textbox.variables[2] = {
      name: "free",
      type: "textbox",
      allowCustomValue: true,
    }
    expect(validateSpecSchema(textbox).valid).toBe(false)
  })
})
