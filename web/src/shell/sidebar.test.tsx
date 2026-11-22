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
    source: "file",
    healthy: true,
    checked_at: "2026-11-14T00:00:00Z",
    dashboard_count: 0,
  }
}

let datasources: DatasourceStatus[] = []

beforeEach(() => {
  localStorage.clear()
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

function navLink(name: string): HTMLElement {
  const link = screen
    .getAllByRole("link", { name })
    .find((candidate) => candidate.closest("ul") === null)
  if (link === undefined) throw new Error(`no ${name} item`)
  return link
}

function precedes(first: Element, second: Element): boolean {
  return Boolean(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING
  )
}

describe("SidebarContent", () => {
  it("keeps the Workspace group outside the scrolling list so it never leaves the screen", async () => {
    renderWithProviders(<SidebarContent />, { route })
    const group = await screen.findByRole("group", { name: "Workspace" })
    expect(group.parentElement?.closest(".overflow-y-auto")).toBeNull()
    expect(navLink("Dashboards").closest(".overflow-y-auto")).not.toBeNull()
  })

  it("lists API keys in the Workspace group for an Admin", async () => {
    renderWithProviders(<SidebarContent />, { route })
    const group = await screen.findByRole("group", { name: "Workspace" })
    expect(
      within(group)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")])
    ).toEqual([
      ["Datasources", "/datasources"],
      ["API keys", "/keys"],
      ["People", "/people"],
      ["Audit", "/audit"],
      ["Settings", "/settings"],
    ])
  })

  it("lists pins right after the Dashboards item, before Logs", async () => {
    renderWithProviders(<SidebarContent />, { route })

    const pinned = await screen.findByRole(
      "link",
      { name: "Checkout latency" },
      { timeout: 5000 }
    )
    const dashboards = navLink("Dashboards")
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
    const dashboards = navLink("Dashboards")
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
    expect(navLink("Dashboards").querySelector(".font-semibold")).toBeNull()
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
    const dashboards = navLink("Dashboards")
    expect(home).toHaveAttribute("href", "/")
    expect(precedes(home, inbox)).toBe(true)
    expect(screen.queryByRole("button", { name: "New" })).toBeNull()
    expect(
      screen.getByRole("list", { name: "Open observations" })
    ).toContainElement(observation)
    expect(precedes(observations, observation)).toBe(true)
    expect(precedes(observation, dashboards)).toBe(true)
  })

  it("opens the Logs list even with one source and lists nothing under it", async () => {
    datasources = [source("loki", "loki", "Loki")]
    renderWithProviders(<SidebarContent />, { route })

    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Logs" })).toHaveAttribute(
        "href",
        "/logs"
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

  it("lists Log, Folder and Explore under Audit with the log current on /audit", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/audit", url: "/audit" },
    })

    const list = await screen.findByRole("list", { name: "Audit" })
    const audit = navLink("Audit")
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")])
    ).toEqual([
      ["Log", "/audit"],
      ["Folder", "/audit/dashboards"],
      ["Explore", "/logs/slashobserve-audit"],
    ])
    expect(within(list).getByRole("link", { name: "Log" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(audit).not.toHaveAttribute("aria-current")
    expect(audit.querySelector(".font-semibold")).not.toBeNull()
    expect(document.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  })

  it("lists the Audit pages only while one of them is open, so the pinned Workspace group stays short", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/dashboards", url: "/dashboards" },
    })

    expect(await screen.findByRole("link", { name: "Audit" })).toBeVisible()
    expect(screen.queryByRole("list", { name: "Audit" })).toBeNull()
  })

  it("gives each Audit page a name no other sidebar link contains, so a name finds one link", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/audit", url: "/audit" },
    })

    const list = await screen.findByRole("list", { name: "Audit" })
    const own = new Set(within(list).getAllByRole("link"))
    const others = screen
      .getAllByRole("link")
      .filter((link) => !own.has(link))
      .map((link) => (link.textContent ?? "").toLowerCase())
    for (const link of own) {
      const name = (link.textContent ?? "").toLowerCase()
      expect(others.filter((other) => name.includes(other))).toEqual([])
    }
  })

  it("on the audit dashboards highlights only that row, not Log", async () => {
    renderWithProviders(<SidebarContent />, {
      route: { path: "/audit/dashboards", url: "/audit/dashboards" },
    })

    const list = await screen.findByRole("list", { name: "Audit" })
    expect(within(list).getByRole("link", { name: "Folder" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(within(list).getByRole("link", { name: "Log" })).not.toHaveAttribute(
      "aria-current"
    )
    expect(navLink("Audit")).not.toHaveAttribute("aria-current")
    expect(navLink("Audit").querySelector(".font-semibold")).not.toBeNull()
    expect(document.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  })

  it("keeps Audit as the parent of the audit explorer and Logs out of it", async () => {
    datasources = [
      source("loki", "loki", "Loki"),
      source("slashobserve-audit", "opensearch", "Audit trail"),
    ]
    renderWithProviders(<SidebarContent />, {
      route: {
        path: "/logs/$datasourceUid",
        url: "/logs/slashobserve-audit",
      },
    })

    const list = await screen.findByRole("list", { name: "Audit" })
    expect(within(list).getByRole("link", { name: "Explore" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(navLink("Audit").querySelector(".font-semibold")).not.toBeNull()
    expect(navLink("Logs")).not.toHaveAttribute("aria-current")
    expect(navLink("Logs").querySelector(".font-semibold")).toBeNull()
    expect(document.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  })

  it("leaves the audit datasource out of the Log sources list", async () => {
    datasources = [
      source("loki", "loki", "Loki"),
      source("ch", "clickhouse", "ClickHouse"),
      source("slashobserve-audit", "opensearch", "Audit trail"),
    ]
    renderWithProviders(<SidebarContent />, { route })

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.textContent)
    ).toEqual(["ClickHouseSQL", "LokiLogQL"])
  })

  it("lists no Log sources when one source remains besides the audit datasource", async () => {
    datasources = [
      source("loki", "loki", "Loki"),
      source("slashobserve-audit", "opensearch", "Audit trail"),
    ]
    renderWithProviders(<SidebarContent />, { route })

    await screen.findByRole("link", { name: "Audit" })
    expect(screen.queryByRole("list", { name: "Log sources" })).toBeNull()
  })
})
