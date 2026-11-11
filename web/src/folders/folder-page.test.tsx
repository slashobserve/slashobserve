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

import type { DashboardSummary } from "@/dashboards/dashboard-columns"
import { parseSearch, stringifySearch } from "@/lib/search"
import { Route as FolderRoute } from "@/routes/_authed/_shell/f/$folderId"
import { PanelHostContext } from "@/shell/side-panel"
import { testQueryClient } from "@/test/render"

import { FolderPage } from "./folder-page"
import type { Folder } from "./queries"

const folderId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"
const otherFolderId = "9c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e5f"
const ana = {
  kind: "user",
  id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
  name: "Ana",
  email: "ana@example.com",
} as const
const principal = { ...ana, role: "admin" } as const

function folder(extra: Partial<Folder> = {}): Folder {
  return {
    id: folderId,
    title: "Shop",
    general: false,
    personal: false,
    restricted: false,
    dashboard_count: 1,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
    ...extra,
  }
}

const checkout = {
  id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
  title: "Checkout service health",
  description: null,
  tags: [],
  latest_version: 7,
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
  datasource_uids: ["prom"],
  open_comment_count: 0,
  folder: { id: folderId, title: "Shop", general: false, personal: false },
  permissions: { edit: true, comment: true, manage: true },
} as unknown as DashboardSummary

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

const slowCheckout = {
  id: "6b1f8a52-0c1d-4e7a-9a43-2d5e8f0a1b3c",
  title: "Slow checkout after the deploy",
  created_at: "2026-11-06T09:00:00Z",
  actor: { label: "Ana" },
}

const errorSearch = {
  id: "7c2a9b63-1d2e-4f8b-8b54-3e6f9a1b2c4d",
  kind: "logs",
  title: "Checkout errors",
  datasource_uid: "es",
  query: "level:error",
  filters: { columns: ["host"] },
  from: "now-6h",
  to: "now",
  folder: { id: folderId, title: "Shop", general: false, personal: false },
  permissions: { comment: true, edit: true, manage: true, create: true },
  updated_at: "2026-11-06T09:30:00Z",
}

type Extras = { findings?: unknown[]; searches?: unknown[] }

function mockServer(
  shown: Folder | null,
  dashboards: DashboardSummary[],
  extras: Extras = {}
) {
  const listed: URLSearchParams[] = []
  const requested: URL[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      requested.push(url)
      if (url.pathname === "/api/v1/findings") {
        return json({ findings: extras.findings ?? [], next_cursor: null })
      }
      if (url.pathname === "/api/v1/saved-searches") {
        return json({
          saved_searches: extras.searches ?? [],
          next_cursor: null,
        })
      }
      if (url.pathname === `/api/v1/folders/${folderId}`) {
        return shown === null
          ? json({ error: { code: "not_found", message: "not found" } }, 404)
          : json(shown)
      }
      if (url.pathname === "/api/v1/folders") {
        const other = folder({ id: otherFolderId, title: "Payments" })
        return json({ folders: shown === null ? [other] : [shown, other] })
      }
      if (url.pathname === "/api/v1/dashboards") {
        listed.push(url.searchParams)
        return json({
          dashboards,
          next_cursor: null,
          total_count: dashboards.length,
        })
      }
      return json({ error: { code: "not_found", message: "not found" } }, 404)
    })
  )
  return Object.assign(listed, { requested })
}

function renderFolderPage(settingsOpen = false) {
  const onSettings = vi.fn()
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
    path: "/f/$folderId",
    component: () => (
      <PanelHostContext value={host}>
        <FolderPage
          folderId={folderId}
          search={{}}
          onSearchChange={vi.fn()}
          settingsOpen={settingsOpen}
          onSettings={onSettings}
        />
      </PanelHostContext>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([authed.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [`/f/${folderId}`] }),
    parseSearch,
    stringifySearch,
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user, onSettings, host }
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("FolderPage", () => {
  it("lists only this folder's dashboards, with no Folder filter or folder row", async () => {
    const listed = mockServer(folder(), [checkout])
    renderFolderPage()

    expect(
      await screen.findByRole("link", { name: "Checkout service health" })
    ).toBeInTheDocument()
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Shop")
    expect(listed.map((params) => params.get("folder"))).toEqual([folderId])
    expect(screen.getByRole("status")).toHaveTextContent("1 dashboard")
    expect(
      screen.queryByRole("button", { name: /^Folder:/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Collapse Shop" })
    ).not.toBeInTheDocument()
  })

  it("says an empty folder has no dashboards yet rather than no matches", async () => {
    mockServer(folder({ dashboard_count: 0 }), [])
    renderFolderPage()

    expect(
      await screen.findByRole("heading", { name: "No dashboards yet" })
    ).toBeInTheDocument()
    expect(
      screen.queryByText("No dashboards match these filters.")
    ).not.toBeInTheDocument()
  })

  it("lists the folder's findings and saved searches below its dashboards", async () => {
    const listed = mockServer(folder(), [checkout], {
      findings: [slowCheckout],
      searches: [errorSearch],
    })
    renderFolderPage()

    const finding = await screen.findByRole("link", {
      name: "Slow checkout after the deploy",
    })
    expect(finding).toHaveAttribute("href", `/findings/${slowCheckout.id}`)
    const findings = screen.getByRole("region", { name: "Findings" })
    expect(within(findings).getByText("Ana")).toBeInTheDocument()

    const search = await screen.findByRole("link", { name: "Checkout errors" })
    expect(search).toHaveAttribute(
      "href",
      expect.stringMatching(/^\/logs\?.*ds=es.*q=level%3Aerror/)
    )
    const searches = screen.getByRole("region", { name: "Saved searches" })
    expect(within(searches).getByText("Logs")).toBeInTheDocument()
    expect(within(searches).getByText("es")).toBeInTheDocument()
    expect(
      within(searches).getByRole("button", {
        name: "More for Checkout errors",
      })
    ).toBeInTheDocument()

    const asked = (path: string) =>
      listed.requested.find((url) => url.pathname === path)
    expect(asked("/api/v1/findings")?.searchParams.get("folder_id")).toBe(
      folderId
    )
    expect(asked("/api/v1/saved-searches")?.searchParams.get("folder_id")).toBe(
      folderId
    )
  })

  it("hides both sections while the folder has neither", async () => {
    const listed = mockServer(folder(), [checkout])
    renderFolderPage()

    await screen.findByRole("link", { name: "Checkout service health" })
    await waitFor(() =>
      expect(listed.requested.map((url) => url.pathname)).toEqual(
        expect.arrayContaining(["/api/v1/findings", "/api/v1/saved-searches"])
      )
    )
    expect(screen.queryByRole("region", { name: "Findings" })).toBeNull()
    expect(screen.queryByRole("region", { name: "Saved searches" })).toBeNull()
  })

  it("gives a reader no menu on a saved search", async () => {
    mockServer(folder(), [checkout], {
      searches: [
        {
          ...errorSearch,
          permissions: {
            comment: true,
            edit: false,
            manage: false,
            create: false,
          },
        },
      ],
    })
    renderFolderPage()

    await screen.findByRole("link", { name: "Checkout errors" })
    expect(
      screen.queryByRole("button", { name: "More for Checkout errors" })
    ).toBeNull()
  })

  it("asks to open settings from the header button", async () => {
    mockServer(folder(), [checkout])
    const { user, onSettings } = renderFolderPage()

    await user.click(
      await screen.findByRole("button", { name: "Folder settings" })
    )
    expect(onSettings).toHaveBeenCalledWith(true)
  })

  it("shows the settings cards in the right panel and asks to close it", async () => {
    mockServer(folder(), [checkout])
    const { user, onSettings, host } = renderFolderPage(true)

    const panel = await within(host).findByRole("complementary", {
      name: "Folder settings",
    })
    expect(within(panel).getByLabelText("Folder name")).toHaveValue("Shop")
    expect(
      within(panel).getByRole("heading", { name: "Delete folder" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Folder settings" })
    ).toHaveAttribute("aria-expanded", "true")

    await user.click(within(panel).getByRole("button", { name: "Close" }))
    expect(onSettings).toHaveBeenCalledWith(false)
  })

  it("offers no settings button to someone who cannot manage the folder", async () => {
    mockServer(
      folder({
        level: "view",
        permissions: {
          comment: false,
          edit: false,
          manage: false,
          create: false,
        },
      }),
      [checkout]
    )
    renderFolderPage()

    expect(
      await screen.findByRole("link", { name: "Checkout service health" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Folder settings" })
    ).not.toBeInTheDocument()
  })

  it("says the folder is not found when the server has no such folder", async () => {
    mockServer(null, [])
    renderFolderPage()

    expect(
      await screen.findByRole("heading", { name: "Folder not found" })
    ).toBeInTheDocument()
  })
})

describe("folder route search", () => {
  const validate = FolderRoute.options.validateSearch as (
    raw: Record<string, unknown>
  ) => Record<string, unknown>

  it("keeps settings only as 1 and never lets the address pick another folder", () => {
    const cases: [Record<string, unknown>, Record<string, unknown>][] = [
      [{ settings: "1" }, { settings: "1", folder: undefined }],
      [{ settings: "true" }, { settings: undefined }],
      [{ settings: 1 }, { settings: undefined }],
      [{ folder: otherFolderId }, { folder: undefined, settings: undefined }],
      [
        { q: "pay", settings: "1", folder: otherFolderId },
        { q: "pay", settings: "1", folder: undefined },
      ],
    ]
    for (const [raw, expected] of cases) {
      const search = validate(raw)
      expect(search).toMatchObject(expected)
      expect("folder" in search && "settings" in search).toBe(true)
    }
  })
})
