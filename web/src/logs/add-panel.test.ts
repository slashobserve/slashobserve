import { describe, expect, it } from "vitest"

import type { Dashboard, DataPanel, Panel } from "@/spec"

import { appendLogsPanel, freePanelId } from "./add-panel"

const input = {
  title: "Checkout logs",
  datasource: "loki",
  query: '{service_name="checkout"}',
}

function dashboard(panels: Panel[]): Dashboard {
  return {
    specVersion: "0",
    title: "Shop",
    time: { from: "now-6h", to: "now" },
    panels,
  }
}

function stat(id: string, y: number, h = 4): DataPanel {
  return {
    id,
    type: "stat",
    title: id,
    datasource: "prom",
    gridPos: { x: 0, y, w: 12, h },
    queries: [{ query: "up" }],
  }
}

describe("appendLogsPanel", () => {
  it("places the panel at the top of an empty dashboard", () => {
    const next = appendLogsPanel(dashboard([]), input)
    expect(next.panels).toEqual([
      {
        id: "logs",
        type: "logs",
        title: "Checkout logs",
        datasource: "loki",
        gridPos: { x: 0, y: 0, w: 24, h: 10 },
        queries: [{ query: '{service_name="checkout"}' }],
        display: { showTime: true },
      },
    ])
  })

  it("appends below the lowest panel of a grid without rows", () => {
    const next = appendLogsPanel(
      dashboard([stat("a", 0, 6), stat("b", 6, 4)]),
      input
    )
    expect(next.panels).toHaveLength(3)
    expect(next.panels[2].gridPos).toEqual({ x: 0, y: 10, w: 24, h: 10 })
  })

  it("appends below the children of an expanded last row", () => {
    const next = appendLogsPanel(
      dashboard([
        stat("a", 0),
        {
          id: "r",
          type: "row",
          title: "Row",
          gridPos: { x: 0, y: 4, w: 24, h: 1 },
          panels: [stat("c", 5, 8)],
        },
      ]),
      input
    )
    expect(next.panels).toHaveLength(3)
    expect(next.panels[2].gridPos.y).toBe(13)
  })

  it("puts the panel under the children of a collapsed last row", () => {
    const next = appendLogsPanel(
      dashboard([
        stat("a", 0),
        {
          id: "r",
          type: "row",
          title: "Row",
          collapsed: true,
          gridPos: { x: 0, y: 4, w: 24, h: 1 },
          panels: [stat("c", 5, 8)],
        },
      ]),
      input
    )
    expect(next.panels).toHaveLength(2)
    const row = next.panels[1]
    expect(
      row.type === "row" && row.panels?.map((p) => [p.id, p.gridPos.y])
    ).toEqual([
      ["c", 5],
      ["logs", 13],
    ])
  })

  it("puts the panel right under an empty collapsed last row", () => {
    const next = appendLogsPanel(
      dashboard([
        {
          id: "r",
          type: "row",
          title: "Row",
          collapsed: true,
          gridPos: { x: 0, y: 7, w: 24, h: 1 },
        },
      ]),
      input
    )
    const row = next.panels[0]
    expect(row.type === "row" && row.panels?.[0].gridPos.y).toBe(8)
  })

  it("never mutates the dashboard it is given", () => {
    const spec = dashboard([
      stat("a", 0),
      {
        id: "r",
        type: "row",
        title: "Row",
        collapsed: true,
        gridPos: { x: 0, y: 4, w: 24, h: 1 },
        panels: [stat("c", 5)],
      },
    ])
    const before = structuredClone(spec)
    appendLogsPanel(spec, input)
    expect(spec).toEqual(before)
  })
})

describe("freePanelId", () => {
  it("skips ids used anywhere, nested row panels included", () => {
    const spec = dashboard([
      stat("logs", 0),
      {
        id: "r",
        type: "row",
        title: "Row",
        gridPos: { x: 0, y: 4, w: 24, h: 1 },
        panels: [stat("logs-2", 5)],
      },
    ])
    expect(freePanelId(spec)).toBe("logs-3")
    expect(appendLogsPanel(spec, input).panels[2].id).toBe("logs-3")
  })

  it("uses the base when it is free", () => {
    expect(freePanelId(dashboard([stat("a", 0)]))).toBe("logs")
  })
})
