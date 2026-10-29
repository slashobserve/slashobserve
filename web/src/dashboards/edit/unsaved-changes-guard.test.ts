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

const live = { pathname: "/dashboards/d1/", search: {} }
const pinned = { pathname: "/dashboards/d1/", search: { version: 3 } }
const elsewhere = { pathname: "/dashboards/d2/", search: {} }

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
  })

  it("lets a change that keeps the pinned version through while dirty", () => {
    makeDirty()
    const withRange = { ...live, search: { from: "now-6h" } }
    expect(shouldBlockNavigation({ current: live, next: withRange })).toBe(
      false
    )
  })
})
