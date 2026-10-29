import { describe, expect, it } from "vitest"

import { parseSearch } from "@/lib/search"

import type { Panel } from "@/spec"

import { dashboardShareUrl, panelChoices, panelShareUrl } from "./share-links"

const origin = "https://observe.example.com"
const search = {
  from: "now-6h",
  to: "now",
  "var-pod": ["a", "b"],
  "var-route": "/checkout",
  "var-filter": ["job|=|api", "env|!=|dev"],
  version: 3,
  edit: "query",
  share: "1",
  cursor: "tooltip",
  refresh: "5s",
  panel: "rps",
}
const both = { time: true, variables: true }
const neither = { time: false, variables: false }

function queryOf(url: string) {
  return parseSearch(new URL(url).search)
}

describe("dashboardShareUrl", () => {
  it("carries the time range and every variable and nothing else", () => {
    const url = dashboardShareUrl(origin, "dash-1", search, both)
    expect(url.startsWith(`${origin}/dashboards/dash-1?`)).toBe(true)
    expect(queryOf(url)).toEqual({
      from: "now-6h",
      to: "now",
      "var-pod": ["a", "b"],
      "var-route": "/checkout",
      "var-filter": ["job|=|api", "env|!=|dev"],
    })
  })

  it("leaves the time range out when asked", () => {
    expect(
      queryOf(
        dashboardShareUrl(origin, "dash-1", search, {
          time: false,
          variables: true,
        })
      )
    ).not.toHaveProperty("from")
  })

  it("leaves the variables out when asked", () => {
    expect(
      queryOf(
        dashboardShareUrl(origin, "dash-1", search, {
          time: true,
          variables: false,
        })
      )
    ).toEqual({ from: "now-6h", to: "now" })
  })

  it("has no query string when nothing is included", () => {
    expect(dashboardShareUrl(origin, "dash-1", search, neither)).toBe(
      `${origin}/dashboards/dash-1`
    )
  })

  it("never carries version, edit, share, cursor, refresh or panel", () => {
    const keys = Object.keys(
      queryOf(dashboardShareUrl(origin, "dash-1", search, both))
    )
    for (const excluded of [
      "version",
      "edit",
      "share",
      "cursor",
      "refresh",
      "panel",
    ]) {
      expect(keys).not.toContain(excluded)
    }
  })

  it("encodes values so they read back unchanged", () => {
    const url = dashboardShareUrl(
      origin,
      "dash-1",
      { "var-q": ["a&b=c", "x y", "$__all"] },
      both
    )
    expect(queryOf(url)).toEqual({ "var-q": ["a&b=c", "x y", "$__all"] })
  })

  it("keeps an empty variable value", () => {
    expect(dashboardShareUrl(origin, "dash-1", { "var-svc": "" }, both)).toBe(
      `${origin}/dashboards/dash-1?var-svc=`
    )
  })
})

function panel(id: string, type: string, x: number, y: number, title?: string) {
  return { id, type, title, gridPos: { x, y, w: 6, h: 4 } } as unknown as Panel
}

describe("panelChoices", () => {
  it("lists data and text panels in layout order with row children after their row", () => {
    const row = {
      ...panel("row-a", "row", 0, 8),
      panels: [panel("c2", "stat", 6, 9), panel("c1", "table", 0, 9, "Slow")],
    } as unknown as Panel
    const choices = panelChoices({
      panels: [
        panel("p2", "timeseries", 12, 0, "Latency"),
        row,
        panel("note", "text", 0, 4),
        panel("p1", "timeseries", 0, 0),
        panel("skip", "placeholder", 0, 20),
        panel("links", "dashlist", 0, 21),
      ],
    })
    expect(choices).toEqual([
      { id: "p1", title: "p1" },
      { id: "p2", title: "Latency" },
      { id: "note", title: "note" },
      { id: "c1", title: "Slow" },
      { id: "c2", title: "c2" },
    ])
  })

  it("is empty for a dashboard of rows without children", () => {
    expect(panelChoices({ panels: [panel("row-a", "row", 0, 0)] })).toEqual([])
  })
})

describe("panelShareUrl", () => {
  it("adds the panel id to the dashboard link", () => {
    const url = panelShareUrl(origin, "dash-1", "p99", search, both)
    expect(queryOf(url)).toEqual({
      from: "now-6h",
      to: "now",
      "var-pod": ["a", "b"],
      "var-route": "/checkout",
      "var-filter": ["job|=|api", "env|!=|dev"],
      panel: "p99",
    })
  })

  it("uses the chosen panel, not the one in the current search", () => {
    expect(panelShareUrl(origin, "dash-1", "p99", search, neither)).toBe(
      `${origin}/dashboards/dash-1?panel=p99`
    )
  })
})
