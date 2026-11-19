import { waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DurationScatter } from "./duration-scatter"
import type { TraceRow } from "./trace-rows"

const rows: TraceRow[] = Array.from({ length: 50 }, (_, index) => ({
  traceId: `${index.toString(16).padStart(32, "0")}`,
  start: 1_000 + index * 10,
  service: "checkout",
  span: "GET /cart",
  durationMs: 10 + index,
  spans: 3,
  errors: 0,
}))

describe("DurationScatter", () => {
  it("leaves the dots out of the tab order because the table has the link of each trace", async () => {
    const { container } = renderWithProviders(
      <DurationScatter
        rows={rows}
        range={{ from: "now-1h", to: "now" }}
        window={{ start: 1_000, end: 2_000 }}
        datasourceUid="tempo"
        highlighted={null}
        onHighlight={() => undefined}
        p95Ms={null}
        p95Label="p95"
        selection={null}
        onSelectionChange={() => undefined}
        selectedCount={0}
      />,
      { route: { path: "/", url: "/" } }
    )
    await waitFor(() =>
      expect(container.querySelectorAll("svg a").length).toBe(50)
    )
    const stops = [...container.querySelectorAll("svg a")].filter(
      (link) => link.getAttribute("tabindex") !== "-1"
    )
    expect(stops).toEqual([])
  })
})
