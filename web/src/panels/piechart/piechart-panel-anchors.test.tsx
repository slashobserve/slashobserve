import { act, fireEvent } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import { hides, type PieAnchorLayout } from "./anchors"
import PiechartPanel from "./piechart-panel"

function status(name: string, value: number): PreparedFrame {
  return {
    ref: name,
    name,
    labels: {},
    fields: [
      { name: "Time", type: "time", values: [1] },
      { name: "Value", type: "number", values: [value] },
    ],
  } as PreparedFrame
}

const frames = [status("200", 75), status("500", 25)]

describe("pie chart anchors", () => {
  it("publishes the slices it drew with the ring they sit on", () => {
    const { store } = renderPublishing(
      PiechartPanel,
      panelOf("piechart", { pieType: "donut" }),
      frames
    )
    const published = store.current()
    expect(published?.type).toBe("piechart")
    const layout = published?.layout as PieAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.slices.map(({ name }) => name)).toEqual(["200", "500"])
    expect(layout.slices[0].startAngle).toBe(0)
    expect(layout.slices[1].endAngle).toBeCloseTo(Math.PI * 2)
  })

  it("has no hole for a pie", () => {
    const { store } = renderPublishing(
      PiechartPanel,
      panelOf("piechart", { pieType: "pie" }),
      frames
    )
    expect((store.current()?.layout as PieAnchorLayout).inner).toBe(0)
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(
      PiechartPanel,
      panelOf("piechart"),
      frames
    )
    view.unmount()
    expect(store.current()).toBeNull()
  })

  it("names a slice the legend switched off as hidden, not drawn", () => {
    const { store, view } = renderPublishing(
      PiechartPanel,
      panelOf("piechart"),
      frames
    )
    act(() => {
      fireEvent.click(view.getByRole("button", { name: /500/ }))
    })
    const layout = store.current()?.layout as PieAnchorLayout
    expect(layout.slices.map(({ name }) => name)).toEqual(["200"])
    expect(layout.hidden).toEqual(["500"])
    expect(
      hides(layout, { kind: "slice", pos: { x: 0, y: 0 }, category: "500" })
    ).toBe(true)
  })
})
