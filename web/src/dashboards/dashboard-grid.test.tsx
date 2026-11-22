import { act, cleanup, render } from "@testing-library/react"
import { useLayoutEffect, useState } from "react"
import type { Layout } from "react-grid-layout"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Dashboard } from "@/spec"

import { DashboardGrid } from "./dashboard-grid"
import { useLayoutStore } from "./edit/layout-store"

vi.mock("react-grid-layout", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-grid-layout")>()),
  useContainerWidth: () => ({
    width: 978,
    mounted: true,
    containerRef: { current: null },
  }),
}))

const spec = {
  specVersion: "0",
  title: "Checkout",
  time: { from: "now-1h", to: "now" },
  panels: [0, 6, 12, 18].map((x) => ({
    id: `p_stat_${x}`,
    type: "stat",
    title: `Stat ${x}`,
    gridPos: { x, y: 0, w: 4, h: 4 },
    datasource: "prom",
    queries: [{ query: "up" }],
  })),
} as unknown as Dashboard

const saved: Layout = spec.panels.map((panel) => ({
  i: panel.id,
  ...panel.gridPos,
}))

const reportsBeforeLoop = 20

function ArrangingPage({ reports }: { reports: Layout[] }) {
  const session = useLayoutStore((state) => state.session)
  const ensure = useLayoutStore((state) => state.ensure)
  const setLayout = useLayoutStore((state) => state.setLayout)
  const sessionOpen = session !== null
  useLayoutEffect(() => {
    ensure("d1", 2, spec)
  }, [sessionOpen, ensure])
  return (
    <DashboardGrid
      dashboardId="d1"
      spec={spec}
      selections={{}}
      values={{}}
      editable
      selectedId="p_stat_0"
      layout={session?.layout}
      onLayoutChange={(next) => {
        reports.push(next)
        if (reports.length <= reportsBeforeLoop) setLayout(next)
      }}
      renderPanel={(item) => <div>{item.panel.title}</div>}
    />
  )
}

function MovingPage({
  given,
  onReport,
}: {
  given: Layout
  onReport: (layout: Layout) => void
}) {
  const [session, setSession] = useState(given)
  const [seenGiven, setSeenGiven] = useState(given)
  if (seenGiven !== given) {
    setSeenGiven(given)
    setSession(given)
  }
  return (
    <DashboardGrid
      dashboardId="d1"
      spec={spec}
      selections={{}}
      values={{}}
      editable
      selectedId="p_stat_0"
      layout={session}
      onLayoutChange={(next) => {
        onReport(next)
        setSession(next)
      }}
      renderPanel={(item) => <div>{item.panel.title}</div>}
    />
  )
}

afterEach(() => {
  cleanup()
  useLayoutStore.getState().end()
})

describe("DashboardGrid", () => {
  it("does not write the fitted view back into a session that reopens after a save", async () => {
    const reports: Layout[] = []
    render(<ArrangingPage reports={reports} />)
    await act(async () => {
      useLayoutStore
        .getState()
        .setLayout(
          saved.map((item) => (item.x === 0 ? { ...item, w: 3 } : item))
        )
    })
    reports.length = 0
    await act(async () => {
      useLayoutStore.getState().end()
    })
    await act(async () => {})
    expect(reports.flat().filter((item) => item.w !== 4)).toEqual([])
    expect(
      useLayoutStore
        .getState()
        .session?.layout.map(({ i, x, y, w, h }) => ({ i, x, y, w, h }))
    ).toEqual(saved)
  })

  it("still reports the settled layout after a move it was given", async () => {
    const onReport = vi.fn()
    const view = render(<MovingPage given={saved} onReport={onReport} />)
    const lifted = saved.map((item, index) =>
      index === 0 ? { ...item, y: 8 } : item
    )
    await act(async () => {
      view.rerender(<MovingPage given={lifted} onReport={onReport} />)
    })
    await act(async () => {})
    expect(onReport).toHaveBeenLastCalledWith(
      expect.arrayContaining([expect.objectContaining({ i: "p_stat_0", y: 0 })])
    )
  })
})
