import { act, fireEvent, render, waitFor } from "@testing-library/react"
import { createRef, type RefObject } from "react"
import { beforeEach, describe, expect, it } from "vitest"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import { createAnchorLayoutStore } from "@/panels/anchor-layout"
import type { TimeseriesAnchorLayout } from "@/panels/timeseries/anchors"

import { CanvasLayer } from "./canvas-composer"

const body = { width: 400, height: 200 }

const layout: TimeseriesAnchorLayout = {
  body,
  plot: { x: 0, y: 0, width: 400, height: 200 },
  xMin: 1000,
  xMax: 1400,
  xs: [1000, 1100, 1200, 1300, 1400],
  series: [],
}

function renderLayer() {
  const store = createAnchorLayoutStore()
  store.publish("timeseries", layout)
  const containerRef = createRef<HTMLDivElement>()
  const surfaceRef = createRef<HTMLDivElement>()
  const view = render(
    <div ref={containerRef}>
      <div ref={surfaceRef} data-canvas-surface="">
        <span data-testid="plot" />
      </div>
      <CanvasLayer
        containerRef={containerRef as RefObject<HTMLElement | null>}
        surfaceRef={surfaceRef as RefObject<HTMLElement | null>}
        store={store}
        surfaceId="s1"
        panelId="p1"
        tool="comment"
        body={body}
        shownRange={{ from: "now-1h", to: "now" }}
      />
    </div>
  )
  return view.getByTestId("plot")
}

beforeEach(() => {
  useCanvasTool.setState({ tool: "comment", draft: null })
})

async function loaded(plot: HTMLElement) {
  await waitFor(() => {
    fireEvent.pointerMove(plot, { clientX: 100, clientY: 50 })
    expect(document.querySelector("[data-canvas-outline]")).toHaveAttribute(
      "data-canvas-outline",
      "target"
    )
  })
}

describe("CanvasLayer", () => {
  it("opens a draft at the clicked spot", async () => {
    const plot = renderLayer()
    await loaded(plot)
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerUp(plot, { button: 0, clientX: 101, clientY: 50 })
    })
    expect(useCanvasTool.getState().draft).toMatchObject({
      panelId: "p1",
      at: { x: 100, y: 50 },
      surface: "s1",
      anchor: { kind: "time", window: { from: "now-1h", to: "now" } },
    })
  })

  it("turns a drag across the time axis into a range", async () => {
    const plot = renderLayer()
    await loaded(plot)
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerMove(plot, { clientX: 300, clientY: 60 })
      fireEvent.pointerUp(plot, { button: 0, clientX: 300, clientY: 60 })
    })
    expect(useCanvasTool.getState().draft).toMatchObject({
      at: { x: 300, y: 60 },
      anchor: {
        kind: "range",
        from: new Date(1100 * 1000).toISOString(),
        to: new Date(1300 * 1000).toISOString(),
        window: { from: "now-1h", to: "now" },
      },
    })
  })
})
