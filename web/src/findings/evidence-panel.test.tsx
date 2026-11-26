import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"
import type { Panel } from "@/spec"

import { EvidencePanel } from "./evidence-panel"

const executedText = 'sum(rate(errors{service="checkout"}[5m]))'

vi.mock("react-grid-layout", () => ({
  useContainerWidth: () => ({
    width: 480,
    mounted: true,
    containerRef: { current: null },
  }),
}))

vi.mock("@/datasources/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/datasources/queries")>()),
  useDatasources: () => ({
    data: [{ uid: "prom", type: "prometheus", name: "Prometheus" }],
  }),
}))

vi.mock("./queries", () => ({
  useFindingPanelData: () => ({
    data: {
      frames: [],
      meta: { queries: [{ ref: "A", executed: executedText }] },
    },
    isError: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

vi.mock("@/panels/panel-body", () => ({
  PanelBody: () => <div>chart</div>,
  StaticPanelBody: () => null,
}))

vi.mock("@/panels/panel-has-data", () => ({ panelHasData: () => true }))

function panel(query: string, type = "timeseries"): Panel {
  return {
    id: "evidence-1",
    type,
    title: "Error rate",
    datasource: "prom",
    queries: [{ query }],
  } as unknown as Panel
}

function renderPanel(input: Panel) {
  return renderWithProviders(
    <EvidencePanel
      findingId="f1"
      panel={input}
      from="2026-11-06T10:00:00Z"
      to="2026-11-06T11:00:00Z"
      height={220}
    />,
    { route: { path: "/", url: "/" } }
  )
}

describe("EvidencePanel", () => {
  it("offers Open in Explore with the panel's query, datasource and range", async () => {
    renderPanel(panel("sum(rate(errors[5m]))"))
    const link = await screen.findByRole("link", { name: "Open in Explore" })
    expect(link).toHaveAttribute("title", "Open in Metrics")
    const href = link.getAttribute("href") ?? ""
    expect(href).toContain("prom")
    expect(decodeURIComponent(href)).toContain("sum(rate(errors[5m]))")
    expect(href).toContain("2026-11-06T10")
    expect(href).toContain("2026-11-06T11")
  })

  it("opens a query that uses variables as the server ran it", async () => {
    renderPanel(panel("sum(rate(errors{service=$service}[5m]))"))
    const link = await screen.findByRole("link", { name: "Open in Explore" })
    expect(decodeURIComponent(link.getAttribute("href") ?? "")).toContain(
      executedText
    )
  })

  it("offers no link when the datasource is a variable it cannot resolve", async () => {
    renderPanel({
      ...panel("sum(rate(errors[5m]))"),
      datasource: "${datasource}",
    } as Panel)
    await screen.findByRole("article", { name: "Error rate" })
    expect(screen.queryByRole("link", { name: "Open in Explore" })).toBeNull()
  })
})
