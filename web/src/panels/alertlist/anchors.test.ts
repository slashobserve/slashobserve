import { describe, expect, it, vi } from "vitest"

import { must } from "@/panels/must"

import { edgeHeight } from "../list-anchors"

import { hitTest, locate, reveal, type AlertAnchorLayout } from "./anchors"

const layout: AlertAnchorLayout = {
  body: { width: 300, height: 100 },
  rows: [
    {
      rect: { x: 0, y: 0, width: 300, height: 30 },
      alert: {
        name: "HighErrorRate",
        labels: [
          ["service", "checkout"],
          ["severity", "page"],
        ],
      },
    },
    {
      rect: { x: 0, y: 30, width: 300, height: 30 },
      alert: { name: "SlowRequests", labels: [["service", "search"]] },
    },
  ],
}

describe("alert list hitTest", () => {
  it("names the alert row by its name and labels", () => {
    expect(hitTest(layout, { x: 30, y: 15 })).toEqual({
      kind: "row",
      pos: { x: 0.1, y: 0.15 },
      row: {
        alertname: "HighErrorRate",
        service: "checkout",
        severity: "page",
      },
    })
  })

  it("returns null below the last row", () => {
    expect(hitTest(layout, { x: 30, y: 80 })).toBeNull()
  })
})

describe("alert list locate", () => {
  it("finds the row a hit test named", () => {
    const point = { x: 30, y: 45 }
    const anchor = hitTest(layout, point)
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual(must(layout.rows[1]).rect)
  })

  it("finds a row by the alert name alone", () => {
    expect(
      locate(layout, {
        kind: "row",
        pos: { x: 0, y: 0 },
        row: { alertname: "HighErrorRate" },
      })
    ).toEqual(must(layout.rows[0]).rect)
  })

  it("points at the bottom edge for an alert scrolled out of view", () => {
    const scrolled: AlertAnchorLayout = {
      ...layout,
      elsewhere: [{ key: "disk", name: "DiskFull", labels: [] }],
    }
    expect(
      locate(scrolled, {
        kind: "row",
        pos: { x: 0, y: 0 },
        row: { alertname: "DiskFull" },
      })
    ).toEqual({ x: 0, y: 100 - edgeHeight, width: 300, height: edgeHeight })
  })

  it("returns null when the alert is gone or the anchor is not a row", () => {
    expect(
      locate(layout, {
        kind: "row",
        pos: { x: 0, y: 0 },
        row: { alertname: "DiskFull" },
      })
    ).toBeNull()
    expect(locate(layout, { kind: "position", pos: { x: 0, y: 0 } })).toBeNull()
  })
})

describe("alert list reveal", () => {
  const hidden: AlertAnchorLayout = {
    ...layout,
    elsewhere: [{ key: "disk", name: "DiskFull", labels: [] }],
  }
  const onDisk = {
    kind: "row" as const,
    pos: { x: 0, y: 0 },
    row: { alertname: "DiskFull" },
  }

  it("asks the panel to scroll an alert that is out of view into view", () => {
    const scroll = vi.fn()
    expect(reveal({ ...hidden, reveal: scroll }, onDisk)).toBe(true)
    expect(scroll).toHaveBeenCalledWith("disk")
  })

  it("does nothing for an alert already in view or gone", () => {
    const scroll = vi.fn()
    const drawn = { ...onDisk, row: { alertname: "HighErrorRate" } }
    expect(reveal({ ...hidden, reveal: scroll }, drawn)).toBe(false)
    const gone = { ...onDisk, row: { alertname: "Nope" } }
    expect(reveal({ ...hidden, reveal: scroll }, gone)).toBe(false)
    expect(reveal(hidden, onDisk)).toBe(false)
    expect(scroll).not.toHaveBeenCalled()
  })
})
