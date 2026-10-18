import { describe, expect, it } from "vitest"
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
    "match must have exactly one of query, series, seriesRegex, field, fieldType"
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
