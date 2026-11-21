import { describe, expect, it } from "vitest"

import { meta as baseMeta, panelOf, renderPublishing } from "../anchor-harness"
import StatusHistoryPanel from "../status-history/status-history-panel"
import type { PreparedFrame } from "../transforms"
import type { TimelineAnchorLayout } from "./anchors"
import StateTimelinePanel from "./state-timeline-panel"

const start = Date.parse(baseMeta.from)
const meta = {
  ...baseMeta,
  step_ms: 60_000,
  queries: [],
} as typeof baseMeta

function service(name: string, values: number[]): PreparedFrame {
  return {
    ref: name,
    name,
    labels: {},
    fields: [
      {
        name: "Time",
        type: "time",
        values: values.map((_, index) => start + index * 600_000),
      },
      { name: "Value", type: "number", values },
    ],
  } as PreparedFrame
}

const frames = [service("checkout", [1, 1, 0]), service("search", [1, 0, 0])]

describe("state timeline anchors", () => {
  it("publishes the rows it drew over the time range", () => {
    const { store } = renderPublishing(
      StateTimelinePanel,
      panelOf("state-timeline"),
      frames,
      { meta }
    )
    const published = store.current()
    expect(published?.type).toBe("state-timeline")
    const layout = published?.layout as TimelineAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.rows.map(({ name }) => name)).toEqual(["checkout", "search"])
    expect(layout.range).toEqual({
      from: start,
      to: Date.parse(baseMeta.to),
    })
    expect(layout.rowHeight).toBeGreaterThan(0)
    expect(layout.rows[0].segments.length).toBeGreaterThan(0)
  })

  it("publishes a status history under its own type", () => {
    const { store } = renderPublishing(
      StatusHistoryPanel,
      panelOf("status-history"),
      frames,
      { meta }
    )
    const published = store.current()
    expect(published?.type).toBe("status-history")
    const layout = published?.layout as TimelineAnchorLayout
    expect(layout.rows.map(({ name }) => name)).toEqual(["checkout", "search"])
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(
      StateTimelinePanel,
      panelOf("state-timeline"),
      frames,
      { meta }
    )
    view.unmount()
    expect(store.current()).toBeNull()
  })
})
