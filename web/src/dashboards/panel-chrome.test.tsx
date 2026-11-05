import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { LayoutPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { PanelChrome } from "./panel-chrome"

const panel: LayoutPanel = {
  id: "p1",
  type: "timeseries",
  title: "Checkout latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

describe("PanelChrome", () => {
  it("labels the panel with its title", () => {
    renderWithProviders(<PanelChrome panel={panel} state="ready" />)
    expect(
      screen.getByRole("article", { name: "Checkout latency" })
    ).toHaveAttribute("data-state", "ready")
    expect(
      screen.getByRole("heading", { name: "Checkout latency" })
    ).toBeVisible()
  })

  it.each([
    { datasourceLabel: "Prometheus", shown: true },
    { datasourceLabel: undefined, shown: false },
  ])(
    "shows the datasource badge only when a label is given ($datasourceLabel)",
    ({ datasourceLabel, shown }) => {
      renderWithProviders(
        <PanelChrome
          panel={panel}
          state="ready"
          datasourceLabel={datasourceLabel}
        />
      )
      if (shown) expect(screen.getByText("Prometheus")).toBeVisible()
      else expect(screen.queryByText("Prometheus")).not.toBeInTheDocument()
    }
  )

  it("opens the links menu on click", async () => {
    const { user } = renderWithProviders(
      <PanelChrome
        panel={panel}
        state="ready"
        links={[
          { title: "Runbook", href: "https://example.com/rb", newTab: false },
        ]}
      />
    )
    expect(
      screen.queryByRole("menuitem", { name: "Runbook" })
    ).not.toBeInTheDocument()
    await user.click(
      screen.getByRole("button", { name: "Panel links: Checkout latency" })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Runbook" })
    ).toHaveAttribute("href", "https://example.com/rb")
  })
})
