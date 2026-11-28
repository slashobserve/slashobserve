import { fireEvent, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AuditHistogram } from "./audit-histogram"
import { must } from "@/test/must"

const fetchMock = vi.hoisted(() => vi.fn())

const window = {
  q: "",
  since: "2026-11-21T05:00:00.000Z",
  until: "2026-11-21T07:00:00.000Z",
}

const buckets = [
  { time: "2026-11-21T05:00:00Z", count: 40 },
  { time: "2026-11-21T05:30:00Z", count: 1 },
  { time: "2026-11-21T06:00:00Z", count: 0 },
  { time: "2026-11-21T06:30:00Z", count: 7 },
]

function render() {
  fetchMock.mockImplementation(() =>
    Promise.resolve(
      new Response(
        JSON.stringify({ step_ms: 1_800_000, total: 48, buckets, form: "kql" }),
        { headers: { "content-type": "application/json" } }
      )
    )
  )
  const onRange = vi.fn()
  const view = renderWithProviders(
    <AuditHistogram window={window} liveCount={0} onRange={onRange} />
  )
  return { ...view, onRange }
}

function stubGeometry() {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left: 0,
    top: 0,
    right: 400,
    bottom: 48,
    width: 400,
    height: 48,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  })
  HTMLElement.prototype.setPointerCapture = vi.fn()
  HTMLElement.prototype.releasePointerCapture = vi.fn()
  HTMLElement.prototype.hasPointerCapture = vi.fn(() => true)
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal("fetch", fetchMock)
  return () => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  }
})

describe("AuditHistogram", () => {
  it("shows the bucket start and count while a bar has focus", async () => {
    render()
    const bar = must((await screen.findAllByRole("button"))[1])
    bar.focus()
    const tip = await screen.findByRole("tooltip")
    expect(tip).toHaveTextContent("1 event")
    expect(tip).toHaveTextContent("2026")
  })

  it("sets the range to a bar with Enter", async () => {
    const { onRange } = render()
    const bar = must((await screen.findAllByRole("button"))[3])
    bar.focus()
    fireEvent.keyDown(bar, { key: "Enter" })
    expect(onRange).toHaveBeenCalledWith({
      from: "2026-11-21T06:30:00.000Z",
      to: "2026-11-21T07:00:00.000Z",
    })
  })

  it("sets the range to a span with Shift and the arrow keys", async () => {
    const { onRange } = render()
    const bars = await screen.findAllByRole("button")
    must(bars[0]).focus()
    fireEvent.keyDown(must(bars[0]), { key: "ArrowRight", shiftKey: true })
    fireEvent.keyDown(must(bars[1]), { key: "ArrowRight", shiftKey: true })
    fireEvent.keyDown(must(bars[2]), { key: "Enter" })
    expect(onRange).toHaveBeenCalledWith({
      from: "2026-11-21T05:00:00.000Z",
      to: "2026-11-21T06:30:00.000Z",
    })
  })

  it("sets the range to the bar that was clicked", async () => {
    stubGeometry()
    const { onRange } = render()
    const group = must((await screen.findAllByRole("button"))[0]).parentElement!
    fireEvent.pointerDown(group, { clientX: 150, pointerId: 1, button: 0 })
    fireEvent.pointerUp(group, { clientX: 150, pointerId: 1, button: 0 })
    expect(onRange).toHaveBeenCalledWith({
      from: "2026-11-21T05:30:00.000Z",
      to: "2026-11-21T06:00:00.000Z",
    })
  })

  it("sets the range to the bars that were dragged across", async () => {
    stubGeometry()
    const { onRange } = render()
    const group = must((await screen.findAllByRole("button"))[0]).parentElement!
    fireEvent.pointerDown(group, { clientX: 50, pointerId: 1, button: 0 })
    fireEvent.pointerMove(group, { clientX: 250, pointerId: 1 })
    fireEvent.pointerUp(group, { clientX: 250, pointerId: 1, button: 0 })
    expect(onRange).toHaveBeenCalledWith({
      from: "2026-11-21T05:00:00.000Z",
      to: "2026-11-21T06:30:00.000Z",
    })
  })
})
