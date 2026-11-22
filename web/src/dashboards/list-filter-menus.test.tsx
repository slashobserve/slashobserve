import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { Folder } from "@/folders/queries"

import { FolderMenu, MadeByMenu, TagsMenu } from "./list-filter-menus"

const session = vi.hoisted(() => ({ role: "admin" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))

function stubUsers() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify({ users: [], next_cursor: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MadeByMenu", () => {
  it("lists people for an Admin", async () => {
    session.role = "admin"
    const fetch = stubUsers()
    renderWithProviders(<MadeByMenu value={undefined} onChange={() => {}} />)
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce())
    expect(new URL(fetch.mock.calls[0]![0].url).pathname).toBe("/api/v1/users")
  })

  it.each(["editor", "commenter", "viewer"])(
    "does not ask a %s for the list of people",
    async (role) => {
      session.role = role
      const fetch = stubUsers()
      renderWithProviders(<MadeByMenu value={undefined} onChange={() => {}} />)
      await new Promise((resolve) => setTimeout(resolve, 50))
      expect(fetch).not.toHaveBeenCalled()
    }
  )
})

function folder(id: string, title: string, audit: boolean): Folder {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: false,
    admins_only: audit,
    audit,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
  }
}

describe("FolderMenu", () => {
  const folders = [folder("f1", "Shop", false), folder("f2", "Trail", true)]

  async function open(value: string | undefined) {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ folders }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
      )
    )
    renderWithProviders(<FolderMenu value={value} onChange={() => {}} />)
    await userEvent.click(
      await screen.findByRole("button", { name: /Folder:/ })
    )
  }

  it("does not offer the Audit folder to browse", async () => {
    await open(undefined)
    expect(
      await screen.findByRole("menuitemradio", { name: "Shop" })
    ).toBeDefined()
    expect(screen.queryByRole("menuitemradio", { name: "Trail" })).toBeNull()
  })

  it("keeps it when a link already chose it", async () => {
    await open("f2")
    expect(
      await screen.findByRole("menuitemradio", { name: "Trail" })
    ).toBeDefined()
  })
})

describe("TagsMenu", () => {
  it.each([
    [true, "exclude_audit=true"],
    [false, ""],
  ])(
    "asks for the tags with excludeAudit %s as %j",
    async (excludeAudit, query) => {
      const fetch = vi.fn<(request: Request) => Promise<Response>>(
        async () =>
          new Response(JSON.stringify({ tags: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
      )
      vi.stubGlobal("fetch", fetch)
      renderWithProviders(
        <TagsMenu
          value={undefined}
          onChange={() => {}}
          excludeAudit={excludeAudit}
        />
      )
      await waitFor(() => expect(fetch).toHaveBeenCalledOnce())
      const url = new URL(fetch.mock.calls[0]![0].url)
      expect(url.pathname).toBe("/api/v1/dashboards/tags")
      expect(url.search).toBe(query === "" ? "" : `?${query}`)
    }
  )
})
