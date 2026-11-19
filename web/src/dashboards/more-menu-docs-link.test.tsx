import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { docsPageUrl } from "@/lib/docs"
import { renderWithProviders } from "@/test/render"

import { MoreMenu } from "./more-menu"

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ available: false }), {
          headers: { "Content-Type": "application/json" },
        })
    )
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MoreMenu", () => {
  it("links the dashboards as code docs next to Export spec JSON", async () => {
    const { user } = renderWithProviders(
      <MoreMenu
        dashboardId="d1"
        mayEdit={false}
        detailsBlockedBy={null}
        onAddPanel={null}
        onEditJson={() => {}}
        duplicateBlockedBy={null}
        deleteBlockedBy={null}
        exportBlockedBy={null}
        onEditDetails={() => {}}
        onDuplicate={null}
        onDelete={() => {}}
        onExport={() => {}}
        onTvMode={() => {}}
        onTvModeFolder={null}
        cursorSync="off"
        onCursorSyncChange={() => {}}
      />,
      { route: { path: "/d/$dashboardId/{-$slug}", url: "/d/d1" } }
    )
    await user.click(await screen.findByRole("button", { name: /^More:/ }))
    const items = await screen.findAllByRole("menuitem")
    const names = items.map((item) => item.textContent)
    const exportAt = names.indexOf("Export spec JSON")
    expect(exportAt).toBeGreaterThanOrEqual(0)
    expect(names[exportAt + 1]).toBe("Dashboards as code")
    const link = screen.getByRole("menuitem", { name: "Dashboards as code" })
    expect(link).toHaveAttribute("href", docsPageUrl("use/dashboards-as-code"))
    expect(link).toHaveAttribute("target", "_blank")
  })
})
