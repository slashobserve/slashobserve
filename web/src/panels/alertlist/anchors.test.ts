import { describe, expect, it } from "vitest"

import { hitTest, type AlertAnchorLayout } from "./anchors"

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
    expect(hitTest(layout, { x: 30, y: 60 })).toBeNull()
  })
})
