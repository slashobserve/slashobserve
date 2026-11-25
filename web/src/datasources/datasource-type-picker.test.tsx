import { screen } from "@testing-library/react"
import { createRef } from "react"
import { describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { DatasourcePanel } from "./datasource-panel"
import type { DatasourceType } from "./draft"

describe("DatasourcePanel type picker", () => {
  function renderCreate(onPickMcp: () => void, type?: DatasourceType) {
    const host = document.createElement("div")
    document.body.append(host)
    return renderWithProviders(
      <PanelHostContext.Provider value={host}>
        <DatasourcePanel
          target={{ mode: "create", type }}
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
    renderCreate(() => undefined, "loki")
    expect(
      await screen.findByRole("combobox", { name: "Type" })
    ).toHaveTextContent("Loki")
  })

  it("sets MCP server apart from the datasource types with a separator", async () => {
    const { user } = renderCreate(() => undefined)
    await user.click(await screen.findByRole("combobox", { name: "Type" }))
    await screen.findByRole("option", { name: "MCP server" })
    const options = screen
      .getAllByRole("option")
      .map((option) => option.textContent)
    expect(options.at(-1)).toBe("MCP server")
    expect(screen.getByRole("separator")).toBeInTheDocument()
  })

  it("titles the panel Add a source until a type is picked", async () => {
    const { user } = renderCreate(() => undefined)
    expect(await screen.findByText("Add a source")).toBeVisible()
    await user.click(screen.getByRole("combobox", { name: "Type" }))
    await user.click(await screen.findByRole("option", { name: "Loki" }))
    expect(await screen.findByText("Add a datasource")).toBeVisible()
    expect(screen.queryByText("Add a source")).toBeNull()
  })

  it("titles the panel Add a datasource when the page preselected a type", async () => {
    renderCreate(() => undefined, "loki")
    expect(await screen.findByText("Add a datasource")).toBeVisible()
  })
})
