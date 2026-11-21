import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import type { LayoutPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { panelDragCancel } from "./edit/panel-move-handle"
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

  it("selects the panel from its title button, and only when it can be selected", async () => {
    const onSelect = vi.fn()
    const { user, rerender } = renderWithProviders(
      <PanelChrome panel={panel} state="ready" onSelect={onSelect} />
    )
    expect(
      screen.getByRole("heading", { name: "Checkout latency" })
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole("button", { name: "Select panel: Checkout latency" })
    )
    expect(onSelect).toHaveBeenCalledTimes(1)

    rerender(<PanelChrome panel={panel} state="ready" />)
    expect(
      screen.queryByRole("button", { name: "Select panel: Checkout latency" })
    ).not.toBeInTheDocument()
  })

  it("shows the panel description when the info button is hovered or focused", async () => {
    const described = { ...panel, description: "Requests that took over 2s" }
    const { user } = renderWithProviders(
      <PanelChrome panel={described} state="ready" />
    )
    await user.hover(
      screen.getByRole("button", { name: "About Checkout latency" })
    )
    expect(await screen.findByText("Requests that took over 2s")).toBeVisible()
  })

  it("marks the selected panel with data-selected and aria-current", () => {
    const { rerender } = renderWithProviders(
      <PanelChrome panel={panel} state="ready" selected />
    )
    const article = screen.getByRole("article", { name: "Checkout latency" })
    expect(article).toHaveAttribute("data-selected")
    expect(article).toHaveAttribute("aria-current", "true")

    rerender(<PanelChrome panel={panel} state="ready" />)
    const unselected = screen.getByRole("article", {
      name: "Checkout latency",
    })
    expect(unselected).not.toHaveAttribute("data-selected")
    expect(unselected).not.toHaveAttribute("aria-current")
  })

  it("starts a drag from the header, title and grip but never from the header's other buttons", () => {
    const described = { ...panel, description: "Requests that took over 2s" }
    renderWithProviders(
      <PanelChrome
        panel={described}
        state="ready"
        onSelect={vi.fn()}
        dragHandle
        links={[
          { title: "Runbook", href: "https://example.com/rb", newTab: false },
        ]}
        menu={<button type="button">Panel menu</button>}
      />
    )
    const header = screen
      .getByRole("heading", { name: "Checkout latency" })
      .closest("header")
    expect(header).toHaveClass("panel-drag-handle")
    const startsDrag = (element: Element) =>
      element.closest(".panel-drag-handle") !== null &&
      element.closest(panelDragCancel) === null
    for (const name of [
      "Move or resize Checkout latency",
      "Select panel: Checkout latency",
    ]) {
      expect(startsDrag(screen.getByRole("button", { name }))).toBe(true)
    }
    for (const name of [
      "About Checkout latency",
      "Panel links: Checkout latency",
      "Panel menu",
    ]) {
      expect(startsDrag(screen.getByRole("button", { name }))).toBe(false)
    }
  })
})
