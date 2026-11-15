import { screen, waitFor, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"
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

function source(uid: string, type: string, name = uid): DatasourceStatus {
  return {
    uid,
    type,
    name,
    healthy: true,
    checked_at: "2026-11-14T00:00:00Z",
    dashboard_count: 0,
  }
}

let datasources: DatasourceStatus[] = []

beforeEach(() => {
  datasources = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      const body = pathname.endsWith("/stars")
        ? { items: pins }
        : pathname.endsWith("/datasources")
          ? { datasources }
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

  it("on a pinned dashboard's page highlights only that row and makes Dashboards semibold", async () => {
    renderWithProviders(<SidebarContent />, { route })

    const pinned = await screen.findByRole(
      "link",
      { name: "Checkout latency" },
      { timeout: 5000 }
    )
    const dashboards = screen.getByRole("link", { name: "Dashboards" })
    expect(pinned).toHaveAttribute("aria-current", "page")
    expect(dashboards).not.toHaveAttribute("aria-current")
    expect(dashboards.querySelector(".font-semibold")).not.toBeNull()
    expect(document.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  })

  it("on an open observation's page highlights only that row and makes Observations semibold", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/o/$observationId", url: "/o/o1" },
    })

    const observation = await screen.findByRole(
      "link",
      { name: "Slow pods" },
      { timeout: 5000 }
    )
    const observations = screen.getByRole("link", { name: "Observations" })
    expect(observation).toHaveAttribute("aria-current", "page")
    expect(observations).not.toHaveAttribute("aria-current")
    expect(observations.querySelector(".font-semibold")).not.toBeNull()
    expect(
      screen
        .getByRole("link", { name: "Dashboards" })
        .querySelector(".font-semibold")
    ).toBeNull()
  })

  it("on the Observations list highlights Observations itself and leaves it regular weight", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/observations", url: "/observations" },
    })

    await screen.findByRole("link", { name: "Slow pods" }, { timeout: 5000 })
    const observations = screen.getByRole("link", { name: "Observations" })
    expect(observations).toHaveAttribute("aria-current", "page")
    expect(observations.querySelector(".font-semibold")).toBeNull()
    expect(screen.getByRole("link", { name: "Slow pods" })).not.toHaveAttribute(
      "aria-current"
    )
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

  it("opens the only log source straight from Logs and lists nothing under it", async () => {
    datasources = [source("loki", "loki", "Loki")]
    renderWithProviders(<SidebarContent />, { route })

    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Logs" })).toHaveAttribute(
        "href",
        "/logs/loki"
      )
    )
    expect(screen.queryByRole("list", { name: "Log sources" })).toBeNull()
    expect(screen.getByRole("link", { name: "Traces" })).toHaveAttribute(
      "href",
      "/traces"
    )
  })

  it("on a listed source's page highlights only that row and makes Logs semibold", async () => {
    datasources = [
      source("loki", "loki", "Loki"),
      source("ch", "clickhouse", "ClickHouse"),
    ]
    renderWithProviders(<SidebarContent />, {
      route: { path: "/logs/$datasourceUid", url: "/logs/loki" },
    })

    const list = await screen.findByRole("list", { name: "Log sources" })
    const logs = screen.getByRole("link", { name: "Logs" })
    expect(within(list).getByRole("link", { name: /Loki/ })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(logs).not.toHaveAttribute("aria-current")
    expect(logs.querySelector(".font-semibold")).not.toBeNull()
    expect(document.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  })

  it("on the only source's page highlights Logs itself", async () => {
    datasources = [source("loki", "loki", "Loki")]
    renderWithProviders(<SidebarContent />, {
      route: { path: "/logs/$datasourceUid", url: "/logs/loki" },
    })

    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Logs" })).toHaveAttribute(
        "aria-current",
        "page"
      )
    )
    expect(
      screen.getByRole("link", { name: "Logs" }).querySelector(".font-semibold")
    ).toBeNull()
  })

  it("lists several log sources between Logs and Traces", async () => {
    datasources = [
      source("loki", "loki", "Loki"),
      source("ch", "clickhouse", "ClickHouse"),
      source("es", "elasticsearch", "Elasticsearch"),
    ]
    renderWithProviders(<SidebarContent />, { route })

    const list = await screen.findByRole("list", { name: "Log sources" })
    const logs = screen.getByRole("link", { name: "Logs" })
    const traces = screen.getByRole("link", { name: "Traces" })
    expect(logs).toHaveAttribute("href", "/logs")
    expect(precedes(logs, list)).toBe(true)
    expect(precedes(list, traces)).toBe(true)
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.textContent)
    ).toEqual(["ClickHouseSQL", "ElasticsearchKQL", "LokiLogQL"])
  })
})
