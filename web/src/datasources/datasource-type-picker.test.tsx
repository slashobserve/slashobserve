import { screen } from "@testing-library/react"
import { createRef } from "react"
import { describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { DatasourcePanel } from "./datasource-panel"

describe("DatasourcePanel type picker", () => {
  function renderCreate(onPickMcp: () => void) {
    const host = document.createElement("div")
    document.body.append(host)
    return renderWithProviders(
      <PanelHostContext.Provider value={host}>
        <DatasourcePanel
          target={{ mode: "create" }}
          onClose={() => undefined}
          trigger={createRef()}
          onPickMcp={onPickMcp}
        />
      </PanelHostContext.Provider>,
      { route: { path: "/", url: "/" } }
    )
  }

  it("lists the datasource types and MCP server side by side", async () => {
    const { user } = renderCreate(() => undefined)
    await user.click(await screen.findByRole("combobox", { name: "Type" }))
    expect(await screen.findByRole("option", { name: "Loki" })).toBeVisible()
    expect(screen.getByRole("option", { name: "MCP server" })).toBeVisible()
  })

  it("hands over to the MCP flow when MCP server is picked", async () => {
    const onPickMcp = vi.fn()
    const { user } = renderCreate(onPickMcp)
    await user.click(await screen.findByRole("combobox", { name: "Type" }))
    await user.click(await screen.findByRole("option", { name: "MCP server" }))
    expect(onPickMcp).toHaveBeenCalledOnce()
  })

  it("starts on the type the page preselected", async () => {
    const host = document.createElement("div")
    document.body.append(host)
    renderWithProviders(
      <PanelHostContext.Provider value={host}>
        <DatasourcePanel
          target={{ mode: "create", type: "loki" }}
          onClose={() => undefined}
          trigger={createRef()}
          onPickMcp={() => undefined}
        />
      </PanelHostContext.Provider>,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("combobox", { name: "Type" })
    ).toHaveTextContent("Loki")
  })
})
