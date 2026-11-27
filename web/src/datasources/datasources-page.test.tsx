import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { mcpSourceKeys, datasourceKeys } from "@/api/keys"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { DatasourcesPage } from "./datasources-page"

function seeded(settled: boolean) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  if (settled) queryClient.setQueryData(mcpSourceKeys.list(), [])
  return queryClient
}

function renderPage(focusMcpSources: boolean, settled = true) {
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <DatasourcesPage focusMcpSources={focusMcpSources} />
    </PanelHostContext.Provider>,
    { queryClient: seeded(settled), route: { path: "/", url: "/" } }
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe("DatasourcesPage", () => {
  it("has one Add source button and no other add button", async () => {
    renderPage(false)
    expect(
      await screen.findByRole("heading", { name: "MCP sources" })
    ).toBeVisible()
    expect(screen.getAllByRole("button", { name: /^Add/ })).toHaveLength(1)
    expect(screen.getByRole("button", { name: "Add source" })).toBeVisible()
  })

  it("scrolls the MCP sources to the top and focuses them on the deep link", async () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    renderPage(true)
    const heading = await screen.findByRole("heading", { name: "MCP sources" })
    await waitFor(() => expect(heading).toHaveFocus())
    expect(scroll).toHaveBeenCalledWith({ block: "start" })
    expect(scroll.mock.contexts[0]).toBe(heading)
  })

  it("waits for both tables to load before scrolling, so they cannot push the heading down", async () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => undefined))
    )
    renderPage(true, false)
    await screen.findByRole("heading", { name: "MCP sources" })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(scroll).not.toHaveBeenCalled()
  })

  it("does not scroll without the deep link", async () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    renderPage(false)
    await screen.findByRole("heading", { name: "MCP sources" })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(scroll).not.toHaveBeenCalled()
  })
})
