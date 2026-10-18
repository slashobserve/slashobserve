import { describe, expect, it } from "vitest"
import type { Dashboard } from "./observespec.gen"
import {
  allPanels,
  dataPanelTypes,
  findPanel,
  isDataPanel,
  isRowPanel,
  queryRef,
} from "./panels"

const spec: Dashboard = {
  specVersion: "0",
  title: "Panels",
  time: { from: "now-1h", to: "now" },
  panels: [
    {
      id: "a",
      type: "stat",
      title: "A",
      gridPos: { x: 0, y: 0, w: 6, h: 4 },
      datasource: "prom",
      queries: [{ query: "up" }],
    },
    {
      id: "r",
      type: "row",
      title: "Row",
      gridPos: { x: 0, y: 4, w: 24, h: 1 },
      panels: [
        {
          id: "b",
          type: "timeseries",
          title: "B",
          gridPos: { x: 0, y: 5, w: 12, h: 8 },
          datasource: "prom",
          queries: [{ query: "up" }, { ref: "errors", query: "up" }],
        },
        {
          id: "n",
          type: "text",
          gridPos: { x: 12, y: 5, w: 12, h: 8 },
          content: "",
        },
      ],
    },
    {
      id: "p",
      type: "placeholder",
      gridPos: { x: 0, y: 13, w: 6, h: 4 },
      reason: "unsupported",
    },
  ],
}

describe("allPanels", () => {
  it("lists top-level panels with their row children right after the row", () => {
    expect(
      allPanels(spec).map((e) => [e.pointer, e.panel.id, e.row?.id])
    ).toEqual([
      ["/panels/0", "a", undefined],
      ["/panels/1", "r", undefined],
      ["/panels/1/panels/0", "b", "r"],
      ["/panels/1/panels/1", "n", "r"],
      ["/panels/2", "p", undefined],
    ])
  })

  it("finds a row child by id and misses unknown ids", () => {
    expect(findPanel(spec, "b")?.pointer).toBe("/panels/1/panels/0")
    expect(findPanel(spec, "missing")).toBeUndefined()
  })
})

describe("panel kinds", () => {
  it("tells data panels, rows and the rest apart", () => {
    const kinds = allPanels(spec).map((e) => [
      e.panel.id,
      isDataPanel(e.panel),
      isRowPanel(e.panel),
    ])
    expect(kinds).toEqual([
      ["a", true, false],
      ["r", false, true],
      ["b", true, false],
      ["n", false, false],
      ["p", false, false],
    ])
    expect(dataPanelTypes).toHaveLength(10)
  })
})

describe("queryRef", () => {
  it("uses the written ref, else the letter for the position", () => {
    expect(queryRef({ query: "up" }, 0)).toBe("A")
    expect(queryRef({ query: "up" }, 2)).toBe("C")
    expect(queryRef({ ref: "errors", query: "up" }, 1)).toBe("errors")
  })
})
