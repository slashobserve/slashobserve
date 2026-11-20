import { createRef } from "react"
import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import type { InspectTarget } from "./inspect-pane"
import { QueryTab } from "./query-tab"

const panel = {
  id: "p1",
  type: "timeseries",
  title: "Requests",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ ref: "A", query: "rate(up{job=~$job}[$__rate_interval])" }],
} as DataPanel

function targetWith(executed: string | undefined): InspectTarget {
  return {
    panel,
    title: "Requests",
    data: {
      frames: [],
      meta: {
        from: "2026-11-20T00:00:00Z",
        to: "2026-11-20T01:00:00Z",
        step_ms: 15000,
        queries: [{ ref: "A", kind: "range", step_ms: 15000, executed }],
      },
    } as unknown as InspectTarget["data"],
    frames: undefined,
    queryKey: ["panel"],
    selections: { job: ["api"] },
    body: createRef(),
    refetch: () => {},
  }
}

describe("QueryTab", () => {
  it("shows the query as run beside the saved text", () => {
    renderWithProviders(
      <QueryTab
        target={targetWith('rate(up{job=~"api"}[1m])')}
        variables={[]}
        zone="UTC"
      />
    )
    expect(screen.getByLabelText("Query A as run").textContent).toBe(
      'rate(up{job=~"api"}[1m])'
    )
  })

  it("leaves the as run block out when the server sent no expanded text", () => {
    renderWithProviders(
      <QueryTab target={targetWith(undefined)} variables={[]} zone="UTC" />
    )
    expect(screen.queryByLabelText("Query A as run")).toBeNull()
  })
})
