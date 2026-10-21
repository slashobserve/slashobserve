import { describe, expect, it } from "vitest"

import type { Dashboard, DataPanel, TextPanel } from "@/spec"

import {
  formatPanel,
  jsonErrorLine,
  missingPanelMessage,
  parsePanelJson,
  withPanel,
} from "./panel-json"

const gridPos = { x: 0, y: 0, w: 6, h: 4 }

const top: DataPanel = {
  id: "top",
  type: "stat",
  title: "Top",
  gridPos,
  datasource: "prom",
  queries: [{ query: "up" }],
}

const child: DataPanel = {
  id: "child",
  type: "timeseries",
  title: "Child",
  gridPos: { x: 0, y: 1, w: 6, h: 4 },
  datasource: "prom",
  queries: [{ query: "rate(up[5m])" }, { ref: "B", query: "up" }],
  thresholds: [{ color: "ok" }, { value: 5, color: "warning" }],
}

const notes: TextPanel = {
  id: "notes",
  type: "text",
  gridPos: { x: 6, y: 0, w: 6, h: 4 },
  content: "hello",
}

const spec: Dashboard = {
  specVersion: "0",
  title: "Editor",
  time: { from: "now-1h", to: "now" },
  panels: [
    top,
    notes,
    {
      id: "row",
      type: "row",
      title: "Row",
      gridPos: { x: 0, y: 5, w: 24, h: 1 },
      panels: [child],
    },
  ],
}

function failure(text: string, panelId: string) {
  const result = parsePanelJson(text, spec, panelId)
  if (result.ok) throw new Error("expected issues")
  return result.issues
}

describe("withPanel", () => {
  it("replaces a top-level panel without touching the input", () => {
    const next = { ...top, title: "New" }
    const placed = withPanel(spec, "top", next)
    expect(placed?.panels[0]).toEqual(next)
    expect(spec.panels[0]).toBe(top)
  })

  it("replaces a row child", () => {
    const next = { ...child, title: "New" }
    const placed = withPanel(spec, "child", next)
    const row = placed?.panels[2]
    expect(row?.type === "row" && row.panels?.[0]).toEqual(next)
  })

  it("returns null for an unknown panel", () => {
    expect(withPanel(spec, "missing", top)).toBeNull()
  })
})

describe("jsonErrorLine", () => {
  it("reads the line and column suffix", () => {
    expect(jsonErrorLine("bad (line 3 column 5)", "")).toBe(3)
    expect(
      jsonErrorLine("JSON.parse: bad at line 2 column 4 of the JSON data", "")
    ).toBe(2)
  })

  it("counts newlines before the position", () => {
    expect(jsonErrorLine("Unexpected token at position 6", "a\nb\nc\nd")).toBe(
      4
    )
  })

  it("is absent when the message has no location", () => {
    expect(jsonErrorLine("JSON Parse error: Expected '}'", "{")).toBeUndefined()
  })
})

describe("parsePanelJson", () => {
  it("reports a syntax error at path root", () => {
    const issues = failure('{\n  "id": "top",\n  "type": \n}', "top")
    expect(issues).toHaveLength(1)
    expect(issues[0].path).toBe("")
    expect(issues[0].message).not.toBe("")
    expect(issues[0].scope).toBe("panel")
  })

  it("reports a non-object", () => {
    const issue = {
      path: "",
      message: "The panel must be a JSON object.",
      scope: "panel",
    }
    expect(failure("[1]", "top")).toEqual([issue])
    expect(failure("null", "top")).toEqual([issue])
  })

  it("keeps the panel id", () => {
    expect(failure(JSON.stringify({ ...top, id: "other" }), "top")).toEqual([
      {
        path: "/id",
        message: 'The panel id must stay "top".',
        scope: "panel",
      },
    ])
  })

  it("refuses row and placeholder types", () => {
    for (const type of ["row", "placeholder"]) {
      expect(failure(JSON.stringify({ ...top, type }), "top")).toEqual([
        {
          path: "/type",
          message: "Choose a data panel type or text.",
          scope: "panel",
        },
      ])
    }
  })

  it("refuses to cross between text and data panels", () => {
    const message =
      "Use the panel type picker in the editor to change a data panel's type."
    expect(failure(JSON.stringify({ ...top, type: "text" }), "top")).toEqual([
      { path: "/type", message, scope: "panel" },
    ])
    expect(
      failure(JSON.stringify({ ...notes, type: "stat" }), "notes")
    ).toEqual([{ path: "/type", message, scope: "panel" }])
  })

  it("allows moving between data panel types", () => {
    const result = parsePanelJson(
      JSON.stringify({ ...top, type: "gauge" }),
      spec,
      "top"
    )
    expect(result.ok).toBe(true)
  })

  it("rewrites schema issue paths relative to a row child", () => {
    const issues = failure(
      JSON.stringify({ ...child, queries: [{ query: "" }, { query: 5 }] }),
      "child"
    )
    expect(issues.length).toBeGreaterThan(0)
    for (const issue of issues) {
      expect(issue.path.startsWith("/panels")).toBe(false)
    }
    expect(issues.some((issue) => issue.path.startsWith("/queries/"))).toBe(
      true
    )
  })

  it("rewrites an issue on the panel itself to the empty path", () => {
    const untitled: Partial<DataPanel> = { ...child }
    delete untitled.title
    const issues = failure(JSON.stringify(untitled), "child")
    expect(issues).toContainEqual({
      path: "",
      message: 'missing property "title"',
      scope: "panel",
    })
  })

  it("keeps a sibling panel's issue out of the draft when ids share a prefix", () => {
    const filler: DataPanel[] = Array.from({ length: 11 }, (_, i) => ({
      ...top,
      id: `p${i}`,
      gridPos: { x: 0, y: i, w: 6, h: 4 },
    }))
    const broken = { ...filler[10], queries: [{ query: "" }] }
    const crowded: Dashboard = {
      ...spec,
      panels: [...filler.slice(0, 10), broken as DataPanel],
    }
    const result = parsePanelJson(JSON.stringify(filler[1]), crowded, "p1")
    if (result.ok) throw new Error("expected issues")
    expect(result.issues.length).toBeGreaterThan(0)
    for (const issue of result.issues) {
      expect(issue.scope).toBe("dashboard")
      expect(issue.path.startsWith("/panels/10/")).toBe(true)
    }
  })

  it("scopes an issue on another panel to the dashboard", () => {
    const stale: Dashboard = {
      ...spec,
      panels: [top, { ...notes, content: 5 as unknown as string }],
    }
    const result = parsePanelJson(JSON.stringify(top), stale, "top")
    if (result.ok) throw new Error("expected issues")
    expect(result.issues).toContainEqual({
      path: "/panels/1/content",
      message: expect.any(String),
      scope: "dashboard",
    })
  })

  it("says the panel is missing when the version does not have it", () => {
    expect(failure(JSON.stringify({ ...top, id: "ghost" }), "ghost")).toEqual([
      { path: "", message: missingPanelMessage, scope: "dashboard" },
    ])
  })

  it("round trips a formatted panel", () => {
    for (const panel of [top, child, notes]) {
      const result = parsePanelJson(formatPanel(panel), spec, panel.id)
      expect(result).toEqual({ ok: true, panel })
    }
  })
})
