import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { ValueAnchorLayout } from "../value-anchors"
import BargaugePanel from "./bargauge-panel"

function series(service: string, value: number): PreparedFrame {
  return {
    ref: service,
    labels: { service },
    fields: [
      { name: "Time", type: "time", values: [1] },
      { name: "Value", type: "number", values: [value] },
    ],
  } as PreparedFrame
}

const frames = [series("checkout", 62), series("search", 41)]

describe("bargauge anchors", () => {
  it("publishes a target for each value it drew", () => {
    const { store } = renderPublishing(
      BargaugePanel,
      panelOf("bargauge"),
      frames
    )
    const published = store.current()
    expect(published?.type).toBe("bargauge")
    const layout = published?.layout as ValueAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.targets.map(({ labels }) => labels)).toEqual([
      { service: "checkout" },
      { service: "search" },
    ])
    expect(layout.targets.every(({ text }) => text !== "")).toBe(true)
  })
})
