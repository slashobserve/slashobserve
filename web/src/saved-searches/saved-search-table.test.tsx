import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { renderWithProviders } from "@/test/render"

import type { SavedSearch } from "./queries"
import { SavedSearchTable } from "./saved-search-table"

const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"

function folder(id: string, title: string) {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: false,
    admins_only: false,
    audit: false,
    kind: "open",
    allowed_actions: [
      "create_in",
      "move_into",
      "rename",
      "set_permissions",
      "delete",
    ],
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    level: "admin",
    permissions: { comment: true, edit: true, manage: true, create: true },
  } as unknown as Folder
}

const folders = [folder(shopId, "Shop"), folder(opsId, "Ops")]

function saved(id: string, title: string, inFolder: Folder): SavedSearch {
  return {
    id,
    kind: "logs",
    title,
    datasource_uid: "es",
    query: "level:error",
    filters: {},
    from: "now-1h",
    to: "now",
    updated_at: "2026-11-06T09:00:00Z",
    actor: {
      kind: "user",
      id: "u1",
      name: "Ana",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ana",
    },
    folder: {
      id: inFolder.id,
      title: inFolder.title,
      general: false,
      personal: false,
    },
    permissions: { comment: true, edit: true, manage: false, create: false },
  } as unknown as SavedSearch
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(searches: SavedSearch[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/folders") return json({ folders })
      if (url.pathname === "/api/v1/saved-searches") {
        return json({ saved_searches: searches, next_cursor: null })
      }
      return json({})
    })
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("SavedSearchTable", () => {
  it("groups the saved searches under their folders and collapses a folder", async () => {
    serve([
      saved("s1", "Checkout errors", folders[0]),
      saved("s2", "Slow cache", folders[1]),
    ])
    const { user } = renderWithProviders(
      <SavedSearchTable kind="logs" sources={[]} />,
      { route: { path: "/", url: "/" } }
    )

    const grid = await screen.findByRole("grid", { name: "Saved searches" })
    await within(grid).findByRole("link", { name: "Checkout errors" })
    expect(within(grid).getByText("Shop")).toBeInTheDocument()
    expect(within(grid).getAllByText("1 saved search")).toHaveLength(2)
    expect(within(grid).getByRole("link", { name: "Slow cache" })).toBeVisible()

    await user.click(
      within(grid).getByRole("button", { name: "Collapse Shop" })
    )
    await waitFor(() =>
      expect(
        within(grid).queryByRole("link", { name: "Checkout errors" })
      ).toBeNull()
    )
    expect(within(grid).getByRole("link", { name: "Slow cache" })).toBeVisible()
    expect(
      within(grid).getByRole("button", { name: "Expand Shop" })
    ).toHaveAttribute("aria-expanded", "false")
  })

  it("names the source by its datasource name when it is known", async () => {
    serve([saved("s1", "Checkout errors", folders[0])])
    renderWithProviders(
      <SavedSearchTable
        kind="logs"
        sources={[{ uid: "es", name: "Elasticsearch" } as never]}
      />,
      { route: { path: "/", url: "/" } }
    )

    expect(await screen.findByText("Elasticsearch")).toBeInTheDocument()
  })

  it("says so when there are no saved searches", async () => {
    serve([])
    renderWithProviders(<SavedSearchTable kind="logs" sources={[]} />, {
      route: { path: "/", url: "/" },
    })

    expect(await screen.findByText("No saved searches yet")).toBeInTheDocument()
    expect(screen.queryByRole("grid")).toBeNull()
  })
})
