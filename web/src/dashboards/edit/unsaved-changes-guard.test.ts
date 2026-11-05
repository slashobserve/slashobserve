import { beforeEach, describe, expect, it } from "vitest"

import type { Dashboard } from "@/spec"

import { useLayoutStore } from "./layout-store"
import { shouldBlockNavigation } from "./unsaved-changes-guard"

const spec: Dashboard = {
  specVersion: "0",
  title: "guard",
  time: { from: "now-1h", to: "now" },
  panels: [
    {
      id: "a",
      type: "stat",
      title: "a",
      gridPos: { x: 0, y: 0, w: 12, h: 4 },
      datasource: "prom",
      queries: [{ query: "up" }],
    },
  ],
}

const page = "/_authed/d/$dashboardId/{-$slug}"
const live = { routeId: page, params: { dashboardId: "d1" }, search: {} }
const pinned = { ...live, search: { version: 3 } }
const elsewhere = { routeId: page, params: { dashboardId: "d2" }, search: {} }
const history = {
  routeId: "/_authed/d/$dashboardId/versions",
  params: { dashboardId: "d1" },
  search: {},
}

function makeDirty() {
  const { start, commitLayout } = useLayoutStore.getState()
  start("d1", 1, spec)
  const layout = useLayoutStore.getState().session!.layout
  commitLayout(
    layout.map((entry) => ({ ...entry, w: 6 })),
    "a"
  )
}

beforeEach(() => useLayoutStore.getState().end())

describe("shouldBlockNavigation", () => {
  it("lets any navigation through while the layout is clean", () => {
    useLayoutStore.getState().start("d1", 1, spec)
    expect(shouldBlockNavigation({ current: live, next: pinned })).toBe(false)
    expect(shouldBlockNavigation({ current: live, next: elsewhere })).toBe(
      false
    )
  })

  it("blocks leaving the dashboard or changing its pinned version while dirty", () => {
    makeDirty()
    expect(shouldBlockNavigation({ current: live, next: elsewhere })).toBe(true)
    expect(shouldBlockNavigation({ current: live, next: pinned })).toBe(true)
    expect(shouldBlockNavigation({ current: pinned, next: live })).toBe(true)
    expect(shouldBlockNavigation({ current: live, next: history })).toBe(true)
  })

  it("lets the slug change through while dirty", () => {
    makeDirty()
    const slugged = { ...live, params: { dashboardId: "d1", slug: "guard" } }
    expect(shouldBlockNavigation({ current: live, next: slugged })).toBe(false)
    expect(shouldBlockNavigation({ current: slugged, next: live })).toBe(false)
  })

  it("lets a change that keeps the pinned version through while dirty", () => {
    makeDirty()
    const withRange = { ...live, search: { from: "now-6h" } }
    expect(shouldBlockNavigation({ current: live, next: withRange })).toBe(
      false
    )
  })
})
