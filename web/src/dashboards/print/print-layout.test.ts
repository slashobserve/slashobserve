import { describe, expect, it } from "vitest"

import type { LayoutItem } from "../rows/expand-layout"
import { gridRowHeight, pixelHeight } from "../stack-heights"
import type { Dashboard } from "@/spec"

import { printEntries, printItems } from "./print-layout"

function panel(key: string, y: number, x: number, h: number): LayoutItem {
  return {
    kind: "panel",
    key,
    panel: { id: key, type: "stat", title: key } as never,
    title: key,
    selections: {},
    first: true,
    gridPos: { x, y, w: 12, h },
  }
}

describe("printItems", () => {
  it("lists the panels in reading order, one per line", () => {
    const items = printItems([
      panel("right", 0, 12, 4),
      panel("below", 4, 0, 4),
      panel("left", 0, 0, 4),
    ])
    expect(items.map((item) => item.entry.key)).toEqual([
      "left",
      "right",
      "below",
    ])
  })

  it("gives a panel the height it has when the page is stacked", () => {
    const [item] = printItems([panel("a", 0, 0, 8)])
    expect(item?.height).toBeGreaterThanOrEqual(pixelHeight(8))
  })

  it("gives a row header and a note one grid row", () => {
    const row: LayoutItem = {
      kind: "row",
      key: "r",
      row: { id: "r", type: "row", title: "Row" } as never,
      title: "Row",
      collapsed: false,
      childCount: 1,
      gridPos: { x: 0, y: 0, w: 24, h: 1 },
    }
    const note: LayoutItem = {
      kind: "note",
      key: "n",
      text: "note",
      gridPos: { x: 0, y: 1, w: 24, h: 1 },
    }
    expect(printItems([row, note]).map((item) => item.height)).toEqual([
      gridRowHeight,
      gridRowHeight,
    ])
  })
})

describe("printEntries", () => {
  const spec = {
    title: "t",
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "r1",
        type: "row",
        title: "Row",
        collapsed: true,
        gridPos: { x: 0, y: 0, w: 24, h: 1 },
        panels: [
          {
            id: "inside",
            title: "inside",
            type: "stat",
            gridPos: { x: 0, y: 1, w: 12, h: 4 },
            queries: [{ query: "up" }],
          },
        ],
      },
    ],
  } as unknown as Dashboard

  it("includes the panels of a row that is collapsed on the page", () => {
    const keys = printEntries(spec, {}, () => undefined).map(
      (item) => item.entry.key
    )
    expect(keys).toContain("inside")
  })
})
