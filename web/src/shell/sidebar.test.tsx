import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SidebarContent } from "./sidebar"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: { name: "Ana Lima", email: "ana@example.com", role: "admin" },
  }),
}))

const pins = [
  {
    id: "d1",
    title: "Checkout latency",
    folder_id: "f1",
    folder_title: "General",
    starred_at: "2026-11-06T09:00:00.000Z",
  },
]

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      const body = pathname.endsWith("/stars")
        ? { items: pins }
        : pathname.endsWith("/observations")
          ? { observations: [], next_cursor: null }
          : { items: [], next_cursor: null, unread_count: 0 }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      })
    })
  )
  return () => vi.unstubAllGlobals()
})

const route = { path: "/d/$dashboardId/{-$slug}", url: "/d/d1" }

describe("SidebarContent pinned dashboards", () => {
  it("lists pins right after the Dashboards item, before Logs", async () => {
    renderWithProviders(<SidebarContent collapsed={false} />, { route })

    const pinned = await screen.findByRole(
      "link",
      { name: "Checkout latency" },
      { timeout: 5000 }
    )
    const dashboards = screen.getByRole("link", { name: "Dashboards" })
    const logs = screen.getByRole("link", { name: "Logs" })
    expect(
      dashboards.compareDocumentPosition(pinned) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      pinned.compareDocumentPosition(logs) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(pinned).toHaveAttribute("aria-current", "page")
  })

  it("leaves the pins out of the collapsed rail", async () => {
    renderWithProviders(<SidebarContent collapsed />, { route })

    await screen.findByRole("link", { name: "Dashboards" })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(screen.queryByRole("link", { name: "Checkout latency" })).toBeNull()
  })
})
