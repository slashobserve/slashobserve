import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { parseSearch, stringifySearch } from "@/lib/search"
import { testQueryClient } from "@/test/render"

import { AuditObservations } from "./audit-observations"

const auditId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"

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

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(
  folders: Folder[],
  observations: { id: string; title: string; state: "open" | "resolved" }[] = []
) {
  const requested: URL[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      requested.push(url)
      switch (url.pathname) {
        case "/api/v1/folders":
          return json({ folders })
        case "/api/v1/observations":
          return json({
            observations: observations.map((item) => ({
              ...item,
              updated_at: "2026-11-06T09:00:00Z",
            })),
            next_cursor: null,
          })
      }
      return json({}, 404)
    })
  )
  return requested
}

function renderPage() {
  const root = createRootRoute()
  const page = createRoute({
    getParentRoute: () => root,
    path: "/audit/observations",
    component: AuditObservations,
  })
  const router = createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: ["/audit/observations"] }),
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

describe("AuditObservations", () => {
  it("lists only the observations of the flagged folder, even renamed, under one heading", async () => {
    const requested = serve(
      [
        folder("9c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e5f", "General", {
          general: true,
        }),
        folder(auditId, "Records", { audit: true }),
      ],
      [
        { id: "o1", title: "Login failures", state: "open" },
        { id: "o2", title: "Odd exports", state: "resolved" },
      ]
    )
    renderPage()

    const table = await screen.findByRole("region", { name: "Observations" })
    expect(
      within(table).getByRole("link", { name: "Login failures" })
    ).toHaveAttribute("href", "/o/o1")
    expect(
      within(table).getByRole("link", { name: "Odd exports" })
    ).toBeVisible()
    expect(
      screen.getAllByRole("heading", { name: "Observations" })
    ).toHaveLength(1)
    expect(screen.getByText("In the Records folder.")).toBeInTheDocument()
    expect(
      requested
        .filter((url) => url.pathname === "/api/v1/observations")
        .map((url) => url.searchParams.get("folder_id"))
    ).toEqual([auditId])
    expect(requested.map((url) => url.pathname)).not.toContain(
      "/api/v1/dashboards"
    )
  })

  it("says the folder has no observations", async () => {
    serve([folder(auditId, "Audit", { audit: true })])
    renderPage()

    expect(
      await screen.findByText("No observations in this folder")
    ).toBeInTheDocument()
  })

  it("says the Audit folder was deleted when no folder is flagged", async () => {
    serve([folder("1a2b3c4d-0000-4000-8000-000000000001", "Audit", {})])
    renderPage()

    expect(
      await screen.findByText("The Audit folder was deleted.")
    ).toBeInTheDocument()
    expect(screen.queryByText(/Explore/)).toBeNull()
    expect(
      screen.getByRole("heading", { level: 1, name: "Observations" })
    ).toBeInTheDocument()
  })

  it("offers a retry when the folders do not load", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({}, 500))
    )
    renderPage()

    expect(
      await screen.findByText("Could not load the Audit folder")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument()
  })
})
