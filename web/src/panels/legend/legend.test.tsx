import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import { createFieldResolver } from "../field-config"
import { Legend } from "./legend"
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

describe("Legend overflow", () => {
  it("says how many entries are not listed", () => {
    render(
      <Legend
        items={[
          {
            key: "up",
            label: "up",
            color: "var(--series-1)",
            line: "solid",
            style: "bars",
          },
        ]}
        hidden={new Set()}
        placement="bottom"
        available={100}
        width={0}
        onToggle={() => undefined}
        more={19}
      />
    )
    expect(screen.getByText("+19 more")).toBeTruthy()
  })
})

describe("Legend names", () => {
  const items = [
    "STATUS_CODE_ERROR",
    "STATUS_CODE_OK",
    "STATUS_CODE_UNSET",
  ].map((label, index) => ({
    key: label,
    label,
    color: `var(--series-${index + 1})`,
    line: "solid" as const,
    style: "bars" as const,
  }))
  const legend = (width: number) => (
    <Legend
      items={items}
      hidden={new Set()}
      placement="right"
      available={100}
      width={width}
      onToggle={() => undefined}
    />
  )
  const shown = () =>
    within(screen.getByRole("group", { name: "Series" }))
      .getAllByRole("button")
      .map((row) => row.textContent)

  it("drops the shared prefix from every entry when they do not fit", () => {
    render(legend(120))
    expect(shown()).toEqual(["ERROR", "OK", "UNSET"])
    expect(screen.getByTitle("STATUS_CODE_OK")).toBeTruthy()
    expect(screen.getByRole("button", { name: "STATUS_CODE_OK" })).toBeTruthy()
  })

  it("keeps whole names when they fit", () => {
    render(legend(400))
    expect(shown()).toEqual(items.map((item) => item.label))
  })

  it("keeps the measured spacing of inline values and never wraps them", () => {
    render(
      <Legend
        items={items}
        reducers={["lastNotNull"]}
        headers={[""]}
        cells={[["119"], ["1,107"], ["62,103"]]}
        hidden={new Set()}
        placement="right"
        available={100}
        width={160}
        onToggle={() => undefined}
      />
    )
    const value = screen.getByText("62,103")
    expect(value).toHaveClass("whitespace-pre")
    expect(value).not.toHaveClass("tabular-nums")
  })
})
