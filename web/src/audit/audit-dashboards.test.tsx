import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { parseSearch, stringifySearch } from "@/lib/search"
import { PanelHostContext } from "@/shell/side-panel"
import { testQueryClient } from "@/test/render"

import { AuditDashboards } from "./audit-dashboards"

const auditId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"
const generalId = "9c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e5f"
const principal = {
  kind: "user",
  id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
  name: "Ana",
  email: "ana@example.com",
  role: "admin",
} as const

function folder(id: string, title: string, extra: Partial<Folder>): Folder {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: false,
    admins_only: false,
    audit: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
    ...extra,
  }
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

class IdleEventSource {
  static readonly CLOSED = 2
  readyState = 1
  onerror: (() => void) | null = null
  addEventListener() {}
  close() {}
}

function serve(folders: Folder[]) {
  const dashboardLists: URLSearchParams[] = []
  vi.stubGlobal("EventSource", IdleEventSource)
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      const detail = folders.find(
        (item) => url.pathname === `/api/v1/folders/${item.id}`
      )
      if (detail !== undefined) return json(detail)
      switch (url.pathname) {
        case "/api/v1/folders":
          return json({ folders })
        case "/api/v1/dashboards":
          dashboardLists.push(url.searchParams)
          return json({ dashboards: [], next_cursor: null, total_count: 0 })
        case "/api/v1/observations":
          return json({ observations: [], next_cursor: null })
        case "/api/v1/saved-searches":
          return json({ saved_searches: [], next_cursor: null })
      }
      return new Response("{}", { status: 404 })
    })
  )
  return dashboardLists
}

function renderPage() {
  const host = document.createElement("div")
  document.body.append(host)
  const root = createRootRoute()
  const authed = createRoute({
    getParentRoute: () => root,
    id: "_authed",
    beforeLoad: () => ({ principal }),
  })
  const page = createRoute({
    getParentRoute: () => authed,
    path: "/audit/dashboards",
    component: () => (
      <PanelHostContext value={host}>
        <AuditDashboards
          search={{}}
          onSearchChange={vi.fn()}
          settingsOpen={false}
          onSettings={vi.fn()}
        />
      </PanelHostContext>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([authed.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ["/audit/dashboards"] }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("AuditDashboards", () => {
  it("opens the folder the server flags and lists only its dashboards, even renamed", async () => {
    const lists = serve([
      folder(generalId, "General", { general: true }),
      folder(auditId, "Records", {
        audit: true,
        restricted: true,
        admins_only: true,
      }),
    ])
    renderPage()

    expect(
      await screen.findByRole("heading", { level: 1, name: "Records" })
    ).toBeInTheDocument()
    expect(lists.map((params) => params.get("folder"))).toEqual([auditId])
    expect(lists.map((params) => params.get("exclude_audit"))).toEqual([null])
  })

  it("says the Audit folder was deleted and is not recreated when no folder is flagged", async () => {
    const lists = serve([
      folder(generalId, "General", { general: true }),
      folder("1a2b3c4d-0000-4000-8000-000000000001", "Audit", {}),
    ])
    renderPage()

    expect(
      await screen.findByText("The Audit folder was deleted.")
    ).toBeInTheDocument()
    expect(screen.getByText(/does not create it again/)).toBeInTheDocument()
    expect(lists).toEqual([])
  })
})
