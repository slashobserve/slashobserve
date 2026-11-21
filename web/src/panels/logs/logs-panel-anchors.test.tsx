import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { LogAnchorLayout } from "./anchors"
import LogsPanel from "./logs-panel"

const frames: PreparedFrame[] = [
  {
    ref: "A",
    fields: [
      { name: "Time", type: "time", values: [1_000, 2_000, 3_000] },
      { name: "Body", type: "string", values: ["first", "second", "third"] },
    ],
  },
]

describe("log anchors", () => {
  it("publishes a line for each entry it drew", () => {
    const { store } = renderPublishing(LogsPanel, panelOf("logs"), frames)
    const published = store.current()
    expect(published?.type).toBe("logs")
    const layout = published?.layout as LogAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.lines.map(({ entry }) => entry.message).sort()).toEqual([
      "first",
      "second",
      "third",
    ])
    expect(layout.elsewhere).toEqual([])
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(LogsPanel, panelOf("logs"), frames)
    view.unmount()
    expect(store.current()).toBeNull()
  })
})
