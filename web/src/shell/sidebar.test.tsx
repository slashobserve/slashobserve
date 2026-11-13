import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SidebarContent } from "./sidebar"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: {
      id: "u1",
      name: "Ana Lima",
      email: "ana@example.com",
      role: "admin",
    },
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
          ? {
              observations: [{ id: "o1", title: "Slow pods", state: "open" }],
              next_cursor: null,
            }
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

function precedes(first: Element, second: Element): boolean {
  return Boolean(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING
  )
}

describe("SidebarContent", () => {
  it("lists pins right after the Dashboards item, before Logs", async () => {
    renderWithProviders(<SidebarContent />, { route })

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

  it("starts at Home, has no New menu and lists open observations under Observations", async () => {
    renderWithProviders(<SidebarContent />, { route })

    const observation = await screen.findByRole(
      "link",
      { name: "Slow pods" },
      { timeout: 5000 }
    )
    const home = screen.getByRole("link", { name: "Home" })
    const inbox = screen.getByRole("link", { name: "Inbox" })
    const observations = screen.getByRole("link", { name: "Observations" })
    const dashboards = screen.getByRole("link", { name: "Dashboards" })
    expect(home).toHaveAttribute("href", "/")
    expect(precedes(home, inbox)).toBe(true)
    expect(screen.queryByRole("button", { name: "New" })).toBeNull()
    expect(
      screen.getByRole("list", { name: "Open observations" })
    ).toContainElement(observation)
    expect(precedes(observations, observation)).toBe(true)
    expect(precedes(observation, dashboards)).toBe(true)
  })
})
