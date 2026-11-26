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

  it("gives the datasource badge only the width the title leaves free", () => {
    renderWithProviders(
      <PanelChrome panel={panel} state="ready" datasourceLabel="Prometheus" />
    )
    const badge = screen.getByText("Prometheus")
    expect(badge).toHaveClass("flex-1", "basis-0")
    expect(badge).not.toHaveClass("shrink-[1000]")
    expect(
      screen.getByRole("heading", { name: "Checkout latency" })
    ).not.toHaveClass("basis-0")
  })

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

  it("keeps the info button beside the menu, clear of the hover actions that overlay the title", () => {
    const described = { ...panel, description: "Requests that took over 2s" }
    renderWithProviders(
      <PanelChrome
        panel={described}
        state="ready"
        datasourceLabel="prom"
        menu={<button type="button">Panel menu</button>}
      />
    )
    const info = screen.getByRole("button", { name: "About Checkout latency" })
    const menu = screen.getByRole("button", { name: "Panel menu" })
    expect(info.parentElement).toBe(menu.parentElement)
    expect(info.nextElementSibling).toBe(menu)
    expect(info.parentElement).toHaveClass("relative", "ml-auto")
  })

  it("shows no info button for a panel without a description", () => {
    renderWithProviders(<PanelChrome panel={panel} state="ready" />)
    expect(
      screen.queryByRole("button", { name: /^About / })
    ).not.toBeInTheDocument()
  })

  it("puts an arrangeable panel's move hint in the header padding, taking no width", () => {
    renderWithProviders(
      <PanelChrome panel={panel} state="ready" onSelect={vi.fn()} arrangeable />
    )
    const header = screen
      .getByRole("heading", { name: "Checkout latency" })
      .closest("header")
    expect(header).toHaveClass("relative", "pl-3")
    expect(header?.querySelector("[data-panel-move-hint]")).toHaveClass(
      "absolute",
      "left-0",
      "w-3"
    )
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
        grabbable
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

  it("hints that an arrangeable panel's header drags it, but not on its buttons", () => {
    const described = { ...panel, description: "Requests that took over 2s" }
    renderWithProviders(
      <PanelChrome
        panel={described}
        state="ready"
        onSelect={vi.fn()}
        arrangeable
        grabbable
        menu={<button type="button">Panel menu</button>}
      />
    )
    const header = screen
      .getByRole("heading", { name: "Checkout latency" })
      .closest("header")
    expect(header).toHaveClass("panel-drag-handle")
    expect(header).toHaveAttribute("title", "Drag to move")
    expect(
      header?.querySelector("[data-panel-move-hint][aria-hidden='true']")
    ).toHaveClass("group-hover/header:opacity-60")
    expect(
      screen.queryByRole("button", { name: /^Move or resize/ })
    ).not.toBeInTheDocument()
    for (const name of ["About Checkout latency", "Panel menu"]) {
      expect(
        screen.getByRole("button", { name }).closest("[title]")
      ).toHaveAttribute("title", "")
    }
  })

  it("keeps the grip's place but drops the hint while another canvas tool is on", () => {
    renderWithProviders(
      <PanelChrome panel={panel} state="ready" onSelect={vi.fn()} arrangeable />
    )
    const header = screen
      .getByRole("heading", { name: "Checkout latency" })
      .closest("header")
    expect(header).not.toHaveClass("panel-drag-handle")
    expect(header).not.toHaveAttribute("title")
    const hint = header?.querySelector("[data-panel-move-hint]")
    expect(hint).toBeInTheDocument()
    expect(hint).not.toHaveClass("group-hover/header:opacity-60")
  })

  it("offers no move affordance to a panel that cannot be arranged", () => {
    renderWithProviders(
      <PanelChrome panel={panel} state="ready" onSelect={vi.fn()} />
    )
    const header = screen
      .getByRole("heading", { name: "Checkout latency" })
      .closest("header")
    expect(header).not.toHaveClass("panel-drag-handle")
    expect(
      screen.queryByRole("button", { name: /^Move or resize/ })
    ).not.toBeInTheDocument()
  })
})
