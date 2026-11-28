import { describe, expect, it } from "vitest"

import type { Dashboard } from "@/spec"
import { validateSpecSchema } from "@/spec/validate"

import { evaluateSpecJson, formatSpec } from "./spec-json"

const saved: Dashboard = {
  specVersion: "0",
  title: "Shop",
  time: { from: "now-1h", to: "now" },
  panels: [
    {
      id: "a",
      type: "stat",
      title: "A",
      gridPos: { x: 0, y: 0, w: 6, h: 4 },
      datasource: "prom",
      queries: [{ query: "up" }],
    },
    {
      id: "r",
      type: "row",
      title: "Row",
      gridPos: { x: 0, y: 4, w: 24, h: 1 },
      collapsed: true,
      panels: [
        {
          id: "b",
          type: "stat",
          title: "B",
          gridPos: { x: 0, y: 5, w: 6, h: 4 },
          datasource: "prom",
          queries: [{ query: "up" }],
        },
      ],
    },
  ],
}

function evaluate(edit: (spec: Record<string, unknown>) => void) {
  const copy = JSON.parse(formatSpec(saved)) as Record<string, unknown>
  edit(copy)
  return evaluateSpecJson(JSON.stringify(copy), saved, validateSpecSchema)
}

function panelsOf(spec: Record<string, unknown>) {
  return spec.panels as Record<string, unknown>[]
}

describe("evaluateSpecJson", () => {
  it("says the Spec is unchanged when only formatting differs", () => {
    expect(
      evaluateSpecJson(JSON.stringify(saved), saved, validateSpecSchema)
    ).toEqual({ kind: "unchanged" })
  })

  it("accepts a valid edit such as a new variable", () => {
    const result = evaluate((spec) => {
      spec.variables = [{ name: "env", type: "custom", options: ["a", "b"] }]
    })
    expect(result.kind).toBe("valid")
  })

  it("reports only the type of a panel whose type is unknown, at the top or in a row", () => {
    const result = evaluate((spec) => {
      const panels = panelsOf(spec)
      panels[0]!.type = "timeseriesx"
      const row = panels[1]!.panels as Record<string, unknown>[]
      Reflect.deleteProperty(row[0]!, "type")
    })
    expect(result).toEqual({
      kind: "invalid",
      lines: [
        '/panels/0/type: "timeseriesx" is not a panel type.',
        "/panels/1/panels/0/type: The panel needs a type.",
      ],
    })
  })

  it("keeps one line per problem elsewhere", () => {
    const result = evaluate((spec) => {
      panelsOf(spec)[0]!.decimals = -3
    })
    expect(result).toEqual({
      kind: "invalid",
      lines: ["/panels/0/decimals: must be >= 0"],
    })
  })

  it("reports a syntax error and a non-object", () => {
    const broken = evaluateSpecJson("{\n  ,", saved, validateSpecSchema)
    expect(broken.kind).toBe("invalid")
    expect(evaluateSpecJson("[]", saved, validateSpecSchema)).toEqual({
      kind: "invalid",
      lines: ["The Spec must be a JSON object."],
    })
  })
})
