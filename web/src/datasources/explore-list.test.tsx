import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { ExploreList } from "./explore-list"
import type { DatasourceStatus } from "./queries"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "viewer" } }),
}))

vi.mock("@/saved-searches/saved-search-table", () => ({
  SavedSearchTable: () => null,
}))

function source(uid: string, name: string, type = "loki"): DatasourceStatus {
  return {
    uid,
    type,
    name,
    source: "file",
    healthy: true,
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
  }
}

function servicesReply(uid: string, services: string[]) {
  return {
    datasource: uid,
    signal: "logs",
    field: "service_name",
    window_seconds: 3600,
    services,
    truncated: false,
  }
}

const replies: Record<string, { status: number; body: unknown }> = {
  fast: {
    status: 200,
    body: servicesReply("fast", ["checkout", "shop", "cart", "search", "pay"]),
  },
  few: { status: 200, body: servicesReply("few", ["billing"]) },
  slow: {
    status: 504,
    body: { error: "the query timed out", code: "query_timeout" },
  },
}

function stubServices() {
  const fetch = vi.fn(async (request: Request) => {
    const uid = new URL(request.url).pathname.split("/")[4] ?? ""
    const reply = replies[uid]
    return new Response(JSON.stringify(reply?.body ?? {}), {
      status: reply?.status ?? 404,
      headers: { "Content-Type": "application/json" },
    })
  })
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderList() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), {
    datasources: [
      source("fast", "Fast"),
      source("few", "Few"),
      source("slow", "Slow"),
    ],
  })
  return renderWithProviders(<ExploreList kind="logs" />, {
    queryClient,
    route: { path: "/", url: "/" },
  })
}

afterEach(() => vi.unstubAllGlobals())

describe("ExploreList services", () => {
  it("shows the first three services and expands the rest", async () => {
    stubServices()
    const { user } = renderList()
    const row = (await screen.findByRole("link", { name: "Fast" })).closest(
      "[role=row]"
    )
    expect(row).not.toBeNull()
    const cell = within(row as HTMLElement)
    expect(await cell.findByRole("link", { name: "checkout" })).toBeVisible()
    expect(cell.queryByRole("link", { name: "search" })).toBeNull()
    await user.click(cell.getByRole("button", { name: "+2 more" }))
    expect(cell.getByRole("link", { name: "pay" })).toBeVisible()
    await user.click(cell.getByRole("button", { name: "Show fewer" }))
    expect(cell.queryByRole("link", { name: "pay" })).toBeNull()
  })

  it("links a service to its source with the service filter", async () => {
    stubServices()
    renderList()
    const link = await screen.findByRole("link", { name: "billing" })
    expect(link.getAttribute("href")).toMatch(/^\/logs\/few\?f=/)
  })

  it("says a source's services are unavailable without hiding the others", async () => {
    stubServices()
    renderList()
    expect(await screen.findByText("Services unavailable")).toBeVisible()
    expect(await screen.findByRole("link", { name: "billing" })).toBeVisible()
  })

  it("filters sources by service name", async () => {
    stubServices()
    const { user } = renderList()
    await screen.findByRole("link", { name: "billing" })
    await user.type(
      screen.getByRole("searchbox", { name: "Filter sources and services" }),
      "BILL"
    )
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Fast" })).toBeNull()
    )
    expect(screen.getByRole("link", { name: "Few" })).toBeVisible()
  })

  it("filters sources by source name and says when nothing matches", async () => {
    stubServices()
    const { user } = renderList()
    await screen.findByRole("link", { name: "billing" })
    const box = screen.getByRole("searchbox", {
      name: "Filter sources and services",
    })
    await user.type(box, "slo")
    expect(screen.getByRole("link", { name: "Slow" })).toBeVisible()
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Few" })).toBeNull()
    )
    await user.clear(box)
    await user.type(box, "nothing")
    expect(
      screen.getByText("No source or service matches the filter.")
    ).toBeVisible()
  })

  it("shows the source list before any service has loaded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {}))
    )
    renderList()
    expect(await screen.findByRole("link", { name: "Fast" })).toBeVisible()
    expect(screen.getAllByText("Loading services")).toHaveLength(3)
  })
})

describe("ExploreList browsing", () => {
  it("leaves the audit datasource out of the log sources", async () => {
    stubServices()
    const queryClient = testQueryClient()
    queryClient.setQueryData(datasourceKeys.list(), {
      datasources: [
        source("few", "Few"),
        source("slashobserve-audit", "Audit trail", "opensearch"),
      ],
    })
    renderWithProviders(<ExploreList kind="logs" />, {
      queryClient,
      route: { path: "/", url: "/" },
    })

    expect(await screen.findByRole("link", { name: "Few" })).toBeVisible()
    expect(screen.queryByRole("link", { name: "Audit trail" })).toBeNull()
  })
})
