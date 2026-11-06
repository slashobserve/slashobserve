import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import { createFieldResolver } from "../field-config"
import {
  useSeriesLegend,
  type SeriesLegendEntry,
  type SeriesLegendOptions,
} from "./use-series-legend"

const panel = {
  id: "p1",
  title: "Requests",
  gridPos: { x: 0, y: 0, w: 12, h: 8 },
  type: "barchart",
  datasource: "prom",
  queries: [{ query: "up" }],
} as DataPanel

const frame: WireFrame = {
  ref: "A",
  fields: [
    { name: "Time", type: "time", values: [1, 2] },
    { name: "Value", type: "number", values: [3, 4] },
  ],
}

const config = createFieldResolver(panel).configFor(frame, frame.fields[1])

const entries: SeriesLegendEntry[] = ["checkout-0", "checkout-1"].map(
  (name, index) => ({
    key: name,
    label: name,
    color: `var(--series-${index + 1})`,
    values: [index, index + 1],
    config,
    hideInLegend: false,
  })
)

function SeriesLegend({
  stateKey,
  legend,
}: {
  stateKey: string
  legend: SeriesLegendOptions["legend"]
}) {
  const { node } = useSeriesLegend({
    stateKey,
    entries,
    options: { legend, legendValues: [], legendSortDesc: false },
    width: 600,
    height: 300,
  })
  return <>{node}</>
}

describe("series legend", () => {
  it("renders one row per series with its name", () => {
    render(<SeriesLegend stateKey="legend-bottom" legend="bottom" />)
    const rows = within(
      screen.getByRole("group", { name: "Series" })
    ).getAllByRole("button")
    expect(rows.map((row) => row.textContent)).toEqual([
      "checkout-0",
      "checkout-1",
    ])
    for (const row of rows) expect(row).toHaveAttribute("aria-pressed", "true")
  })

  it("renders nothing when the legend is hidden", () => {
    render(<SeriesLegend stateKey="legend-hidden" legend="hidden" />)
    expect(
      screen.queryByRole("group", { name: "Series" })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole("button")).not.toBeInTheDocument()
  })
})
