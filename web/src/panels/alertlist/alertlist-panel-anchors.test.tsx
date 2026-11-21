import { describe, expect, it, vi } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { AlertAnchorLayout } from "./anchors"
import { alertModel } from "./alert-model"
import AlertlistPanel from "./alertlist-panel"

const now = Date.parse("2026-11-21T11:00:00Z")

function alert(
  alertname: string,
  alertstate: string,
  extra: Record<string, string> = {}
): PreparedFrame {
  return {
    ref: alertname,
    name: "ALERTS",
    labels: { alertname, alertstate, ...extra },
    fields: [
      { name: "Time", type: "time", values: [now] },
      { name: "Value", type: "number", values: [1] },
    ],
  } as PreparedFrame
}

const frames = [
  alert("HighErrorRate", "firing", { service: "checkout" }),
  alert("SlowRequests", "pending", { service: "search" }),
]

describe("alert list anchors", () => {
  it("publishes a row for each alert it drew", () => {
    const { store } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist"),
      frames
    )
    const published = store.current()
    expect(published?.type).toBe("alertlist")
    const layout = published?.layout as AlertAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.rows.map(({ alert }) => alert.name)).toEqual([
      "HighErrorRate",
      "SlowRequests",
    ])
    expect(layout.elsewhere).toEqual([])
  })

  it("publishes nothing for the stat view", () => {
    const { store } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist", { viewMode: "stat" }),
      frames
    )
    expect(store.current()).toBeNull()
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist"),
      frames
    )
    view.unmount()
    expect(store.current()).toBeNull()
  })

  it("publishes a reveal that scrolls the alert's row into view", () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    const { store, view } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist"),
      frames
    )
    const second = alertModel(frames, {}, now).alerts[1]
    const layout = store.current()?.layout as AlertAnchorLayout
    layout.reveal?.(second.key)
    expect(scroll).toHaveBeenCalledTimes(1)
    expect(scroll.mock.contexts[0]).toBe(
      view.container.querySelectorAll("li")[1]
    )
  })
})
