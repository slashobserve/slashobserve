import { act, fireEvent, render, waitFor } from "@testing-library/react"
import { createRef, type ReactNode, type RefObject } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

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

function renderLayer(
  extra?: ReactNode,
  wholePanel = false,
  tool: "comment" | "ask" = "comment"
) {
  const store = createAnchorLayoutStore()
  store.publish("timeseries", layout)
  const containerRef = createRef<HTMLDivElement>()
  const surfaceRef = createRef<HTMLDivElement>()
  const view = render(
    <div ref={containerRef}>
      <div ref={surfaceRef} data-canvas-surface="">
        <span data-testid="plot" />
        {extra}
      </div>
      <CanvasLayer
        containerRef={containerRef as RefObject<HTMLElement | null>}
        surfaceRef={surfaceRef as RefObject<HTMLElement | null>}
        store={store}
        surfaceId="s1"
        panelId="p1"
        tool={tool}
        body={body}
        shownRange={{ from: "now-1h", to: "now" }}
        wholePanel={wholePanel}
      />
    </div>
  )
  return view.getByTestId("plot")
}

function outline() {
  return document.querySelector("[data-canvas-outline]")
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

  it("leaves a click on a pin to the pin, so its thread opens instead of a new draft", async () => {
    const opened = vi.fn()
    const plot = renderLayer(
      <button type="button" data-pin="t1" onClick={opened}>
        pin
      </button>
    )
    await loaded(plot)
    const pin = document.querySelector<HTMLElement>("[data-pin]")!
    act(() => {
      fireEvent.pointerDown(pin, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerUp(pin, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.click(pin, { detail: 1 })
    })
    expect(opened).toHaveBeenCalledTimes(1)
    expect(useCanvasTool.getState().draft).toBeNull()
  })

  it.each(["data-ask-pin", "data-ask-pin-stack"])(
    "leaves a click on a private ask marker (%s) to the marker",
    async (attribute) => {
      const opened = vi.fn()
      const plot = renderLayer(
        <button type="button" {...{ [attribute]: "r1" }} onClick={opened}>
          ask
        </button>
      )
      await loaded(plot)
      const marker = document.querySelector<HTMLElement>(`[${attribute}]`)!
      act(() => {
        fireEvent.pointerDown(marker, { button: 0, clientX: 100, clientY: 50 })
        fireEvent.pointerUp(marker, { button: 0, clientX: 100, clientY: 50 })
        fireEvent.click(marker, { detail: 1 })
      })
      expect(opened).toHaveBeenCalledTimes(1)
      expect(useCanvasTool.getState().draft).toBeNull()
    }
  )

  it("leaves a click on a stack of pins to the stack", async () => {
    const opened = vi.fn()
    const plot = renderLayer(
      <button type="button" data-pin-stack="2" onClick={opened}>
        2
      </button>
    )
    await loaded(plot)
    const stack = document.querySelector<HTMLElement>("[data-pin-stack]")!
    act(() => {
      fireEvent.pointerDown(stack, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerUp(stack, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.click(stack, { detail: 1 })
    })
    expect(opened).toHaveBeenCalledTimes(1)
    expect(useCanvasTool.getState().draft).toBeNull()
  })

  it("lets the keyboard press a control inside the panel body, and still swallows a pointer click on it", async () => {
    const pressed = vi.fn()
    const plot = renderLayer(
      <button type="button" onClick={pressed}>
        Sort
      </button>
    )
    await loaded(plot)
    const sort = document.querySelector<HTMLElement>("button")!
    fireEvent.click(sort, { detail: 0 })
    expect(pressed).toHaveBeenCalledTimes(1)
    fireEvent.click(sort, { detail: 1 })
    expect(pressed).toHaveBeenCalledTimes(1)
  })

  it("clears the outline when a drag is released outside the panel", async () => {
    const plot = renderLayer()
    await loaded(plot)
    const container = plot.parentElement!.parentElement!
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerMove(plot, { clientX: 300, clientY: 60 })
      fireEvent.pointerLeave(container)
    })
    expect(outline()).not.toBeNull()
    act(() => {
      fireEvent.pointerUp(document.body, {
        button: 0,
        clientX: 900,
        clientY: 60,
      })
    })
    expect(outline()).toBeNull()
  })

  it("comments on the whole panel from a click or a drag when asked to, as in a panel's table view", () => {
    const plot = renderLayer(undefined, true)
    act(() => {
      fireEvent.pointerMove(plot, { clientX: 100, clientY: 50 })
    })
    expect(outline()).toHaveAttribute("data-canvas-outline", "panel")
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerUp(plot, { button: 0, clientX: 101, clientY: 50 })
    })
    expect(useCanvasTool.getState().draft).toMatchObject({
      panelId: "p1",
      anchor: null,
      at: { x: 100, y: 50 },
    })
    useCanvasTool.getState().close()
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerMove(plot, { clientX: 300, clientY: 60 })
      fireEvent.pointerUp(plot, { button: 0, clientX: 300, clientY: 60 })
    })
    expect(useCanvasTool.getState().draft).toMatchObject({ anchor: null })
  })

  it("asks about the whole panel from a click in a panel's table view", () => {
    useCanvasTool.setState({ tool: "ask" })
    const plot = renderLayer(undefined, true, "ask")
    act(() => {
      fireEvent.pointerDown(plot, { button: 0, clientX: 100, clientY: 50 })
      fireEvent.pointerUp(plot, { button: 0, clientX: 101, clientY: 50 })
    })
    expect(useCanvasTool.getState().draft).toMatchObject({
      tool: "ask",
      panelId: "p1",
      anchor: null,
    })
  })
})
