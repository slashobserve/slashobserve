import { createRef } from "react"
import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { LayoutPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { PanelHoverActions } from "./panel-hover-actions"

const panel: LayoutPanel = {
  id: "p1",
  type: "timeseries",
  title: "Checkout latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

const text = {
  id: "t1",
  type: "text",
  title: "Notes",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  content: "hello",
} as LayoutPanel

function serveAgent(available: boolean) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ available }), {
          headers: { "Content-Type": "application/json" },
        })
    )
  )
}

function renderActions(over: {
  canComment?: boolean
  idle?: boolean
  panel?: LayoutPanel
  viewing?: boolean
}) {
  return renderWithProviders(
    <PanelHoverActions
      panel={over.panel ?? panel}
      canComment={over.canComment ?? true}
      idle={over.idle ?? true}
      viewing={over.viewing}
      composerRef={createRef<HTMLTextAreaElement>()}
    />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("PanelHoverActions", () => {
  it("offers Ask and Comment, named after the panel, when the agent is connected", async () => {
    serveAgent(true)
    renderActions({})
    expect(
      await screen.findByRole("button", { name: "Ask about Checkout latency" })
    ).toBeEnabled()
    expect(
      screen.getByRole("button", { name: "Comment on Checkout latency" })
    ).toBeEnabled()
  })

  it("leaves out Ask while no model is connected", async () => {
    serveAgent(false)
    renderActions({})
    await screen.findByRole("button", { name: "Comment on Checkout latency" })
    expect(
      screen.queryByRole("button", { name: /^Ask about/ })
    ).not.toBeInTheDocument()
  })

  it("leaves out Ask on a panel that has no data", async () => {
    serveAgent(true)
    renderActions({ panel: text })
    await screen.findByRole("button", { name: "Comment on Notes" })
    expect(
      screen.queryByRole("button", { name: /^Ask about/ })
    ).not.toBeInTheDocument()
  })

  it("shows nothing without Comment permission", async () => {
    serveAgent(true)
    renderActions({ canComment: false })
    await Promise.resolve()
    expect(screen.queryByRole("button")).not.toBeInTheDocument()
  })

  it("disables both while the layout has unsaved moves", async () => {
    serveAgent(true)
    renderActions({ idle: false })
    await screen.findByRole("button", { name: "Ask about Checkout latency" })
    for (const button of screen.getAllByRole("button")) {
      expect(button).toBeDisabled()
    }
  })

  it("fades with opacity only, on fine hover pointers, under the panel's hover", async () => {
    serveAgent(true)
    const { container } = renderActions({})
    await screen.findByRole("button", { name: "Comment on Checkout latency" })
    const wrapper = container.querySelector("[data-panel-hover-actions]")
    const classes = wrapper?.className ?? ""
    expect(classes).toContain(
      "[@media(hover:hover)_and_(pointer:fine)]:opacity-0"
    )
    expect(classes).toContain(
      "[@media(hover:hover)_and_(pointer:fine)]:group-hover/panel:opacity-100"
    )
    expect(classes).toContain(
      "[@media(hover:hover)_and_(pointer:fine)]:group-focus-within/panel:opacity-100"
    )
    expect(classes).toContain("motion-safe:transition-opacity")
    expect(classes).toContain("motion-safe:duration-100")
    expect(classes).not.toMatch(/translate|scale|transition-all/)
  })

  it.each([
    { viewing: false, hidden: true },
    { viewing: true, hidden: false },
  ])(
    "hides under a selected panel's toolbar only outside the full-size view (viewing $viewing)",
    async ({ viewing, hidden }) => {
      serveAgent(true)
      const { container } = renderActions({ viewing })
      await screen.findByRole("button", { name: "Comment on Checkout latency" })
      const classes =
        container.querySelector("[data-panel-hover-actions]")?.className ?? ""
      expect(classes.includes("group-data-[selected]/panel:hidden")).toBe(
        hidden
      )
    }
  )
})
