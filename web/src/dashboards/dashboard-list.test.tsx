import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"
import type { Folder } from "@/folders/queries"
import { testQueryClient } from "@/test/render"

import type { DashboardSummary } from "./dashboard-columns"
import { DashboardList } from "./dashboard-list"
import type { ListSearch } from "./list-filters"

const shopId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"
const opsId = "9c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e5f"
const ana = {
  kind: "user",
  id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
  name: "Ana",
  email: "ana@example.com",
} as const
const principal = { ...ana, role: "admin" } as const

function folder(id: string, title: string, count: number): Folder {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: false,
    admins_only: false,
    audit: false,
    dashboard_count: count,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
  }
}

function dashboard(id: string, title: string, inFolder: Folder) {
  return {
    id,
    title,
    description: null,
    tags: [],
    latest_version: 2,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    updated_by: ana,
    creator: {
      author: ana,
      origin: "web",
      origin_client: null,
      via: "direct",
      actor: {
        kind: "user",
        id: ana.id,
        name: ana.name,
        origin: "web",
        client: null,
        run_id: null,
        label: ana.name,
      },
    },
    datasource_uids: [],
    open_comment_count: 0,
    view_count: 0,
    folder: {
      id: inFolder.id,
      title: inFolder.title,
      general: false,
      personal: false,
    },
    permissions: { edit: true, comment: true, manage: true },
  } as unknown as DashboardSummary
}

const shop = folder(shopId, "Shop", 2)
const ops = folder(opsId, "Ops", 0)

const checkout = dashboard("d1", "Checkout health", shop)
const cart = dashboard("d2", "Cart errors", shop)

class IdleEventSource {
  static readonly CLOSED = 2
  readyState = 1
  onerror: (() => void) | null = null
  addEventListener() {}
  close() {}
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(dashboards: DashboardSummary[]) {
  vi.stubGlobal("EventSource", IdleEventSource)
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/dashboards") {
        return json({
          dashboards,
          next_cursor: null,
          total_count: dashboards.length,
        })
      }
      if (url.pathname === "/api/v1/folders") {
        return json({ folders: [shop, ops] })
      }
      return json({ error: { code: "not_found", message: "no" } })
    })
  )
}

function renderList(search: ListSearch = {}) {
  const root = createRootRoute()
  const authed = createRoute({
    getParentRoute: () => root,
    id: "_authed",
    beforeLoad: () => ({ principal }),
  })
  const page = createRoute({
    getParentRoute: () => authed,
    path: "/dashboards",
    component: () => (
      <>
        <h1 id="heading">Dashboards</h1>
        <DashboardList
          labelledBy="heading"
          search={search}
          onSearchChange={vi.fn()}
        />
      </>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([authed.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ["/dashboards"] }),
    parseSearch,
    stringifySearch,
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user }
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("DashboardList", () => {
  it("groups the dashboards under folder rows, collapses a folder and says an empty one has none", async () => {
    serve([checkout, cart])
    const { user } = renderList()

    const grid = await screen.findByRole("grid", { name: "Dashboards" })
    await within(grid).findByRole("link", { name: "Checkout health" })
    expect(
      within(grid).getByRole("link", { name: "Cart errors" })
    ).toBeVisible()
    expect(within(grid).getByText("Shop")).toBeVisible()
    expect(within(grid).getByText("Ops")).toBeVisible()
    expect(within(grid).getByText("No dashboards yet")).toBeVisible()

    await user.click(
      within(grid).getByRole("button", { name: "Collapse Shop" })
    )
    await waitFor(() =>
      expect(
        within(grid).queryByRole("link", { name: "Checkout health" })
      ).toBeNull()
    )
    expect(
      within(grid).getByRole("button", { name: "Expand Shop" })
    ).toHaveAttribute("aria-expanded", "false")
    expect(within(grid).getByText("No dashboards yet")).toBeVisible()
  })

  it("lists one flat table with a Folder column when a sort is chosen", async () => {
    serve([checkout, cart])
    renderList({ sort: "updated" })

    const grid = await screen.findByRole("grid", { name: "Dashboards" })
    await within(grid).findByRole("link", { name: "Checkout health" })
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toContain("Folder")
    expect(within(grid).queryByRole("button", { name: /^Collapse/ })).toBeNull()
    expect(within(grid).getAllByText("Shop")).toHaveLength(2)
  })

  it("selects dashboards from the rows and the header, and shows the move bar", async () => {
    serve([checkout, cart])
    const { user } = renderList()

    await user.click(
      await screen.findByRole("checkbox", { name: "Select Checkout health" })
    )
    expect(
      await screen.findByRole("checkbox", {
        name: "Select every dashboard on this page",
      })
    ).toHaveAttribute("aria-checked", "mixed")

    await user.click(
      screen.getByRole("checkbox", {
        name: "Select every dashboard on this page",
      })
    )
    expect(
      screen.getByRole("checkbox", { name: "Select Cart errors" })
    ).toBeChecked()
  })
})
