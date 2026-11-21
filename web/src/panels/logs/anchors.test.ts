import { describe, expect, it } from "vitest"

import { hitTest, type LogAnchorLayout } from "./anchors"

const layout: LogAnchorLayout = {
  body: { width: 400, height: 100 },
  lines: [
    {
      rect: { x: 0, y: 0, width: 400, height: 20 },
      entry: {
        key: "row-0",
        time: 1_000,
        message: "card declined",
        labels: { service: "payments" },
      },
    },
    {
      rect: { x: 0, y: 20, width: 400, height: 20 },
      entry: { key: "row-1", time: null, message: "", labels: {} },
    },
  ],
}

describe("logs hitTest", () => {
  it("names the log line under the pointer", () => {
    expect(hitTest(layout, { x: 40, y: 10 })).toEqual({
      kind: "log_line",
      pos: { x: 0.1, y: 0.1 },
      id: "row-0",
      from: new Date(1_000).toISOString(),
      quote: "card declined",
      series: { service: "payments" },
    })
  })

  it("leaves out what the line does not have", () => {
    expect(hitTest(layout, { x: 40, y: 30 })).toEqual({
      kind: "log_line",
      pos: { x: 0.1, y: 0.3 },
      id: "row-1",
    })
  })

  it("returns null below the last line", () => {
    expect(hitTest(layout, { x: 40, y: 90 })).toBeNull()
  })
})
