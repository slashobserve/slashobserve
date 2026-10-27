import { describe, expect, it } from "vitest"

import type { Dashboard, DataPanel } from "@/spec"

import {
  appliedKey,
  evaluateJson,
  issueRows,
  maxShownIssues,
  textForDraft,
} from "./json-edit"
import { formatPanel, type JsonIssue } from "./panel-json"

const panel: DataPanel = {
  id: "latency",
  type: "timeseries",
  title: "Latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

const spec: Dashboard = {
  specVersion: "0",
  title: "Editor",
  time: { from: "now-1h", to: "now" },
  panels: [panel],
}

describe("evaluateJson", () => {
  it("accepts the formatted draft without changing it", () => {
    const result = evaluateJson(formatPanel(panel), spec, panel)
    expect(result).toEqual({ kind: "valid", panel, changesDraft: false })
  })

  it("ignores key order when comparing with the draft", () => {
    const reordered = JSON.stringify({
      queries: panel.queries,
      datasource: panel.datasource,
      gridPos: panel.gridPos,
      title: panel.title,
      type: panel.type,
      id: panel.id,
    })
    const result = evaluateJson(reordered, spec, panel)
    expect(result).toMatchObject({ kind: "valid", changesDraft: false })
  })

  it("reports an edit that differs from the draft", () => {
    const result = evaluateJson(
      JSON.stringify({ ...panel, title: "JSON title" }),
      spec,
      panel
    )
    expect(result).toMatchObject({
      kind: "valid",
      changesDraft: true,
      panel: { title: "JSON title" },
    })
  })

  it("accepts fields the editor has no controls for", () => {
    const overrides = [{ match: { query: "A" }, color: "critical" }]
    const result = evaluateJson(
      JSON.stringify({ ...panel, overrides }),
      spec,
      panel
    )
    expect(result).toMatchObject({
      kind: "valid",
      changesDraft: true,
      panel: { overrides },
    })
  })

  it("lists a syntax error as one panel issue", () => {
    const text = formatPanel(panel).replace(/\}$/, "")
    const result = evaluateJson(text, spec, panel)
    expect(result).toMatchObject({
      kind: "invalid",
      issues: [{ path: "", scope: "panel" }],
    })
  })

  it("names a schema problem by its path inside the panel", () => {
    const result = evaluateJson(
      JSON.stringify({ ...panel, unit: "bogus" }),
      spec,
      panel
    )
    expect(result.kind).toBe("invalid")
    if (result.kind !== "invalid") return
    expect(result.issues.some((issue) => issue.path.includes("/unit"))).toBe(
      true
    )
  })

  it("refuses to change the panel id", () => {
    const result = evaluateJson(
      JSON.stringify({ ...panel, id: "other" }),
      spec,
      panel
    )
    expect(result).toMatchObject({
      kind: "invalid",
      issues: [{ path: "/id" }],
    })
  })
})

describe("textForDraft", () => {
  it("keeps the text when the draft is the last applied one", () => {
    expect(textForDraft(panel, appliedKey(panel), true)).toBeNull()
  })

  it("keeps the text when the draft is the same in another key order", () => {
    const reordered = Object.fromEntries(
      Object.entries(panel).reverse()
    ) as DataPanel
    expect(textForDraft(reordered, appliedKey(panel), true)).toBeNull()
  })

  it("replaces valid text when the draft changed elsewhere", () => {
    const changed = { ...panel, title: "Suggested" }
    expect(textForDraft(changed, appliedKey(panel), true)).toBe(
      formatPanel(changed)
    )
  })

  it("keeps invalid text when the draft changed elsewhere", () => {
    const changed = { ...panel, title: "Suggested" }
    expect(textForDraft(changed, appliedKey(panel), false)).toBeNull()
  })
})

describe("issueRows", () => {
  it("writes a syntax error as a line", () => {
    const issues: JsonIssue[] = [
      { path: "", message: "Unexpected token", scope: "panel", line: 3 },
    ]
    expect(issueRows(issues)).toEqual({
      rows: [
        {
          location: { kind: "line", text: "Line 3" },
          message: "Unexpected token",
        },
      ],
      more: 0,
    })
  })

  it("writes a schema problem as its path", () => {
    const issues: JsonIssue[] = [
      { path: "/unit", message: "is not allowed", scope: "panel" },
    ]
    expect(issueRows(issues).rows).toEqual([
      {
        location: { kind: "path", text: "/unit" },
        message: "is not allowed",
      },
    ])
  })

  it("leaves out the location of a whole-panel problem", () => {
    const issues: JsonIssue[] = [
      { path: "", message: "The panel must be a JSON object.", scope: "panel" },
    ]
    expect(issueRows(issues).rows[0].location).toBeNull()
  })

  it("shows at most 20 and counts the rest", () => {
    const issues: JsonIssue[] = Array.from({ length: 27 }, (_, index) => ({
      path: `/field${index}`,
      message: "is wrong",
      scope: "panel" as const,
    }))
    const { rows, more } = issueRows(issues)
    expect(rows).toHaveLength(maxShownIssues)
    expect(more).toBe(7)
  })
})
