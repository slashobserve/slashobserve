import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { HistogramAnchorLayout } from "./anchors"
import HistogramPanel from "./histogram-panel"

const values = Array.from({ length: 101 }, (_, index) => index)

const frames: PreparedFrame[] = [
  {
    ref: "A",
    name: "latency",
    labels: {},
    fields: [
      { name: "Time", type: "time", values: values.map((_, index) => index) },
      { name: "Value", type: "number", values },
    ],
  },
]

describe("histogram anchors", () => {
  it("publishes the plot it drew with its bucket edges", () => {
    const { store } = renderPublishing(
      HistogramPanel,
      panelOf("histogram"),
      frames
    )
    const published = store.current()
    expect(published?.type).toBe("histogram")
    const layout = published?.layout as HistogramAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.edges.length).toBeGreaterThan(2)
    expect(layout.edges[0]).toBe(0)
    expect(layout.plot.width).toBeGreaterThan(0)
    expect(layout.format(layout.edges[1])).not.toBe("")
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(
      HistogramPanel,
      panelOf("histogram"),
      frames
    )
    view.unmount()
    expect(store.current()).toBeNull()
  })
})
