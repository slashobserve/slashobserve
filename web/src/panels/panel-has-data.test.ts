import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import { createFieldResolver } from "./field-config"
import { panelHasData } from "./panel-has-data"

function frame(ref: string, values: unknown[]): WireFrame {
  return {
    ref,
    labels: {},
    fields: [
      { name: "Time", type: "time", values: [1, 2] },
      { name: "Value", type: "number", values },
    ],
  } as WireFrame
}

function panel(props: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p",
    title: "p",
    type: "timeseries",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    datasource: "prom",
    queries: [{ query: "a" }, { query: "b" }],
    ...props,
  } as DataPanel
}

function has(p: DataPanel, frames: WireFrame[]) {
  return panelHasData(p, frames, createFieldResolver(p))
}

describe("panelHasData", () => {
  const frames = [frame("A", [1, 2]), frame("B", [3, 4])]
  const hideB = [{ match: { query: "B" }, hidden: true }]
  const hideBoth = [
    { match: { query: "A" }, hidden: true },
    { match: { query: "B" }, hidden: true },
  ]

  it("is true when the prepared frames hold a value", () => {
    expect(has(panel(), frames)).toBe(true)
  })

  it("is false for frames without a value", () => {
    expect(has(panel(), [])).toBe(false)
    expect(has(panel(), [frame("A", [null, null])])).toBe(false)
  })

  it("is false when a transform leaves no value field, though the raw frames have data", () => {
    const filtered = panel({
      transform: [{ type: "filterFieldsByName", include: ["missing"] }],
    })
    expect(has(filtered, frames)).toBe(false)
  })

  it("stays true while a timeseries still has a series that is not hidden", () => {
    expect(has(panel({ overrides: hideB } as Partial<DataPanel>), frames)).toBe(
      true
    )
  })

  it("is false when every timeseries series is hidden by config", () => {
    expect(
      has(panel({ overrides: hideBoth } as Partial<DataPanel>), frames)
    ).toBe(false)
  })

  it("ignores hidden for panel types that do not hide series", () => {
    expect(
      has(
        panel({ type: "table", overrides: hideBoth } as Partial<DataPanel>),
        frames
      )
    ).toBe(true)
  })
})
