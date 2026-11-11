import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { renderWithProviders } from "@/test/render"

import type { SavedSearch } from "./queries"
import { SavedSearchRowMenu } from "./saved-search-row-menu"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast: toasts }))

const mineId = "11111111-1111-4111-8111-111111111111"
const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"
const searchId = "44444444-4444-4444-8444-444444444444"

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return {
    id,
    title,
    general: false,
    personal: false,
    permissions: { comment: true, edit: true, manage: true, create: true },
    ...extra,
  } as Folder
}

const folders = [
  folder(mineId, "Ana's folder", { personal: true }),
  folder(shopId, "Shop"),
  folder(opsId, "Ops", {
    permissions: { comment: true, edit: false, manage: false, create: false },
  }),
]

function saved(inFolder: Folder, edit = true): SavedSearch {
  return {
    id: searchId,
    kind: "logs",
    title: "Checkout errors",
    datasource_uid: "es",
    query: "level:error",
    filters: {},
    from: "now-1h",
    to: "now",
    folder: {
      id: inFolder.id,
      title: inFolder.title,
      general: inFolder.general,
      personal: inFolder.personal,
    },
    permissions: { comment: true, edit, manage: false, create: false },
  } as SavedSearch
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

type Sent = { method: string; body: unknown }

function stubServer(refuse = false) {
  const sent: Sent[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/folders") return json({ folders })
      if (url.pathname === `/api/v1/saved-searches/${searchId}`) {
        const text = await request.text()
        sent.push({
          method: request.method,
          body: text === "" ? null : JSON.parse(text),
        })
        if (refuse) {
          return json(
            { error: "you cannot edit that folder", code: "forbidden" },
            403
          )
        }
        return request.method === "DELETE"
          ? new Response(null, { status: 204 })
          : json({})
      }
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
  return sent
}

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockClear()
  toasts.error.mockClear()
})

describe("SavedSearchRowMenu", () => {
  it("is absent without Edit", () => {
    stubServer()
    renderWithProviders(<SavedSearchRowMenu saved={saved(folders[1], false)} />)
    expect(screen.queryByRole("button", { name: /^More for/ })).toBeNull()
  })

  it("renames through a dialog", async () => {
    const sent = stubServer()
    const { user } = renderWithProviders(
      <SavedSearchRowMenu saved={saved(folders[1])} />
    )
    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Rename..." }))
    const dialog = await screen.findByRole("dialog", {
      name: "Rename saved search",
    })
    const title = within(dialog).getByLabelText("Title")
    await user.clear(title)
    await user.type(title, "  Payment errors ")
    await user.click(within(dialog).getByRole("button", { name: "Rename" }))
    await waitFor(() =>
      expect(sent).toEqual([
        { method: "PATCH", body: { title: "Payment errors" } },
      ])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Renamed to Payment errors")
    )
  })

  it("moves to a shared folder the caller can edit, never into My Dashboards", async () => {
    const sent = stubServer()
    const { user } = renderWithProviders(
      <SavedSearchRowMenu saved={saved(folders[0])} />
    )
    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await screen.findByText("Publish to folder")
    await waitFor(() =>
      expect(screen.getAllByRole("menuitemradio")).toHaveLength(2)
    )
    expect(screen.queryByRole("menuitemradio", { name: "Ops" })).toBeNull()
    await user.click(screen.getByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() =>
      expect(sent).toEqual([{ method: "PATCH", body: { folder_id: shopId } }])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Published to Shop")
    )
  })

  it("shows the server's message when a move is refused", async () => {
    stubServer(true)
    const { user } = renderWithProviders(
      <SavedSearchRowMenu saved={saved(folders[0])} />
    )
    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await user.click(await screen.findByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith(
        "Could not move Checkout errors. You cannot edit that folder."
      )
    )
  })

  it("deletes only after a confirmation", async () => {
    const sent = stubServer()
    const { user } = renderWithProviders(
      <SavedSearchRowMenu saved={saved(folders[1])} />
    )
    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete..." }))
    const dialog = await screen.findByRole("alertdialog")
    expect(sent).toEqual([])
    await user.click(
      within(dialog).getByRole("button", { name: "Delete saved search" })
    )
    await waitFor(() =>
      expect(sent).toEqual([{ method: "DELETE", body: null }])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Deleted Checkout errors")
    )
  })
})
