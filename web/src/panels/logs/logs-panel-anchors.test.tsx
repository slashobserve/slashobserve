import { describe, expect, it, vi } from "vitest"

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

  it("publishes a reveal that scrolls the line's row into view", () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    const { store, view } = renderPublishing(LogsPanel, panelOf("logs"), frames)
    const layout = store.current()?.layout as LogAnchorLayout
    const key = layout.lines[1].entry.key
    layout.reveal?.(key)
    expect(scroll).toHaveBeenCalledTimes(1)
    const scrolled = scroll.mock.contexts[0] as HTMLElement
    expect(scrolled.dataset.key).toBe(key)
    expect(view.container.contains(scrolled)).toBe(true)
  })
})
