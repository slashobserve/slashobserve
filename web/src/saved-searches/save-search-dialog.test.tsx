import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { addPill, encodePills } from "@/logs/pills"
import { renderWithProviders } from "@/test/render"

import { logsDraft, tracesDraft, type SearchDraft } from "./draft"
import { SaveSearchDialog } from "./save-search-dialog"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast: toasts }))

const mineId = "11111111-1111-4111-8111-111111111111"
const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"

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
  folder(shopId, "Shop"),
  folder(mineId, "Ana's folder", { personal: true }),
  folder(opsId, "Ops", {
    permissions: { comment: true, edit: false, manage: false, create: false },
  }),
]

const range = { from: "now-6h", to: "now" }

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(refuse = false) {
  const saved: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/folders") return json({ folders })
      if (url.pathname === "/api/v1/saved-searches") {
        const body = JSON.parse(await request.text())
        saved.push(body)
        if (refuse) {
          return json(
            { error: "unknown datasource", code: "unknown_datasource" },
            422
          )
        }
        return json({ ...body }, 201)
      }
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
  return saved
}

function open(draft: SearchDraft) {
  return renderWithProviders(
    <SaveSearchDialog
      draft={draft}
      open
      onOpenChange={vi.fn()}
      finalFocus={() => true}
    />
  )
}

function logsDraftOf(search: Parameters<typeof logsDraft>[0]["search"]) {
  const result = logsDraft({
    uid: "es",
    type: "elasticsearch",
    search,
    range,
  })
  if (!("draft" in result)) throw new Error("expected a draft")
  return result.draft
}

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockClear()
})

describe("SaveSearchDialog", () => {
  it("saves the query with its pills folded in, the columns and the range", async () => {
    const saved = stubServer()
    const pills = encodePills(
      addPill([], {
        kind: "is",
        field: "service",
        value: "checkout",
        negate: false,
        pinned: false,
        paused: false,
      })
    )
    const { user } = open(
      logsDraftOf({ q: "level:error", f: pills, cols: "host,message" })
    )
    expect(await screen.findByLabelText("Title")).toHaveValue(
      'level:error and service:"checkout"'
    )
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: "Folder" })
      ).toHaveTextContent("Ana's folder")
    )
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() =>
      expect(saved).toEqual([
        {
          kind: "logs",
          datasource_uid: "es",
          query: 'level:error and service:"checkout"',
          filters: { columns: ["host", "message"] },
          from: "now-6h",
          to: "now",
          title: 'level:error and service:"checkout"',
          folder_id: mineId,
        },
      ])
    )
    expect(toasts.success).toHaveBeenCalledWith(
      `Saved "level:error and service:"checkout"" in Ana's folder`
    )
  })

  it("offers only folders where the caller can create and saves into the chosen one", async () => {
    const saved = stubServer()
    const { user } = open(logsDraftOf({ q: "error" }))
    const trigger = await screen.findByRole("combobox", { name: "Folder" })
    await waitFor(() => expect(trigger).toHaveTextContent("Ana's folder"))
    await user.click(trigger)
    expect(screen.getByRole("option", { name: "Shop" })).toBeInTheDocument()
    expect(screen.queryByRole("option", { name: "Ops" })).toBeNull()
    await user.click(screen.getByRole("option", { name: "Shop" }))
    await user.clear(screen.getByLabelText("Title"))
    await user.type(screen.getByLabelText("Title"), "  Errors  ")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0]).toMatchObject({ title: "Errors", folder_id: shopId })
  })

  it("sends no query for a ClickHouse trace search", async () => {
    const saved = stubServer()
    const { user } = open(
      tracesDraft({
        uid: "ch",
        type: "clickhouse",
        search: { service: "checkout", q: "{ ignored }" },
        range,
      })
    )
    expect(await screen.findByLabelText("Title")).toHaveValue("checkout")
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: "Folder" })
      ).toHaveTextContent("Ana's folder")
    )
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0]).toEqual({
      kind: "traces",
      datasource_uid: "ch",
      filters: { service: "checkout" },
      from: "now-6h",
      to: "now",
      title: "checkout",
      folder_id: mineId,
    })
  })

  it("keeps Save off until there is a title", async () => {
    stubServer()
    const { user } = open(logsDraftOf({ q: "error" }))
    await user.clear(await screen.findByLabelText("Title"))
    expect(screen.getByRole("button", { name: "Save" })).toHaveAttribute(
      "aria-disabled",
      "true"
    )
  })

  it("shows the server's message when the save is refused", async () => {
    stubServer(true)
    const { user } = open(logsDraftOf({ q: "error" }))
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: "Folder" })
      ).toHaveTextContent("Ana's folder")
    )
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(await screen.findByText("Unknown datasource.")).toBeInTheDocument()
    expect(toasts.success).not.toHaveBeenCalled()
  })
})
