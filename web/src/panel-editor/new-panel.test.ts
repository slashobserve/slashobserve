import { describe, expect, it } from "vitest"

import type { Dashboard, DataPanel } from "@/spec"

import { newPanelDraft, withNewPanel } from "./new-panel"

const stat: DataPanel = {
  id: "panel",
  type: "stat",
  title: "Requests",
  datasource: "prom",
  gridPos: { x: 0, y: 0, w: 6, h: 4 },
  queries: [{ query: "up" }],
}

function dashboard(panels: Dashboard["panels"]): Dashboard {
  return {
    specVersion: "0",
    title: "Shop",
    time: { from: "now-1h", to: "now" },
    panels,
  }
}

describe("newPanelDraft", () => {
  it("takes the first data panel's datasource, a free id and the row below", () => {
    const draft = newPanelDraft(dashboard([stat]), [{ uid: "ch" }])
    expect(draft).toMatchObject({
      id: "panel-2",
      type: "timeseries",
      datasource: "prom",
      gridPos: { x: 0, y: 4, w: 12, h: 8 },
    })
  })

  it("falls back to the first configured datasource on a dashboard without data panels", () => {
    const text = {
      id: "note",
      type: "text" as const,
      gridPos: { x: 0, y: 0, w: 12, h: 6 },
      content: "hi",
    }
    expect(newPanelDraft(dashboard([text]), [{ uid: "ch" }])).toMatchObject({
      id: "panel",
      datasource: "ch",
      gridPos: { y: 6 },
    })
  })

  it("offers nothing when no datasource exists", () => {
    expect(newPanelDraft(dashboard([]), [])).toBeNull()
  })
})

describe("withNewPanel", () => {
  it("appends the panel below whatever the dashboard holds now", () => {
    const draft = {
      ...stat,
      id: "panel-2",
      gridPos: { x: 0, y: 0, w: 6, h: 4 },
    }
    const taller = dashboard([{ ...stat, gridPos: { x: 0, y: 0, w: 6, h: 9 } }])
    const result = withNewPanel(taller, draft)
    expect(result.panels.map((panel) => panel.id)).toEqual(["panel", "panel-2"])
    expect(result.panels[1]?.gridPos.y).toBe(9)
  })
})
