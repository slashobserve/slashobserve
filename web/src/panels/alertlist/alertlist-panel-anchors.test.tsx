import { afterEach, describe, expect, it, vi } from "vitest"

import { panelOf, renderPublishing, stubRects } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import { locate } from "./anchors"
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

const many = Array.from({ length: 5 }, (_, index) =>
  alert(`Alert${index}`, "firing")
)

function stubList() {
  stubRects((element) => {
    if (element.tagName === "UL") {
      return { x: 0, y: 0, width: 400, height: 100 }
    }
    const list = element.parentElement
    if (element.tagName !== "LI" || list?.tagName !== "UL") return undefined
    return {
      x: 0,
      y: Array.from(list.children).indexOf(element) * 50 - list.scrollTop,
      width: 400,
      height: 50,
    }
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  Reflect.deleteProperty(Element.prototype, "scrollIntoView")
})

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

  it("culls rows scrolled out of the list into elsewhere", () => {
    stubList()
    const { store } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist"),
      many
    )
    const layout = store.current()?.layout as AlertAnchorLayout
    expect(layout.rows.map(({ alert }) => alert.name)).toEqual([
      "Alert0",
      "Alert1",
    ])
    expect(layout.rows[1].rect).toEqual({
      x: 0,
      y: 50,
      width: 400,
      height: 50,
    })
    expect(layout.elsewhere?.map(({ name }) => name)).toEqual([
      "Alert2",
      "Alert3",
      "Alert4",
    ])
    expect(
      locate(layout, {
        kind: "row",
        pos: { x: 0, y: 0 },
        row: { alertname: "Alert4" },
      })
    ).toEqual({ x: 0, y: 238, width: 400, height: 2 })
  })

  it("scrolls the list itself to a hidden row and never the page", () => {
    stubList()
    const page = vi.fn()
    Element.prototype.scrollIntoView = page
    const { store, view } = renderPublishing(
      AlertlistPanel,
      panelOf("alertlist"),
      many
    )
    const layout = store.current()?.layout as AlertAnchorLayout
    const hidden = alertModel(many, {}, now).alerts[4]
    layout.reveal?.(hidden.key)
    expect(view.container.querySelector("ul")?.scrollTop).toBe(150)
    expect(page).not.toHaveBeenCalled()
  })
})
