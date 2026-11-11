import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { renderWithProviders } from "@/test/render"

import { FindingMoveMenu } from "./finding-move-menu"
import type { FindingEnvelope } from "./queries"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast: toasts }))

const findingId = "6b1f8a52-0c1d-4e7a-9a43-2d5e8f0a1b3c"
const mineId = "11111111-1111-4111-8111-111111111111"
const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
    ...extra,
  } as Folder
}

const folders = [
  folder(mineId, "Ana's folder", { personal: true }),
  folder(shopId, "Shop"),
  folder(opsId, "Ops", {
    permissions: { comment: true, edit: false, manage: false, create: false },
    level: "comment",
  }),
]

function envelope(
  place: { dashboard: string | null; folder: Folder | null },
  edit = true
): FindingEnvelope {
  return {
    finding: {
      id: findingId,
      title: "Slow checkout",
      dashboard_id: place.dashboard,
      folder_id: place.folder?.id ?? null,
    },
    folder:
      place.folder === null
        ? null
        : {
            id: place.folder.id,
            title: place.folder.title,
            general: place.folder.general,
            personal: place.folder.personal,
          },
    permissions: { comment: true, edit, manage: false, create: false },
  } as unknown as FindingEnvelope
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(move: () => Response = () => json({})) {
  const moved: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/folders") return json({ folders })
      if (url.pathname === `/api/v1/findings/${findingId}/move`) {
        moved.push(JSON.parse(await request.text()))
        return move()
      }
      return json({ error: { code: "not_found", message: "not found" } }, 404)
    })
  )
  return moved
}

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockClear()
  toasts.error.mockClear()
})

describe("FindingMoveMenu", () => {
  it("is absent for a finding on a dashboard", () => {
    stubServer()
    renderWithProviders(
      <FindingMoveMenu
        envelope={envelope({ dashboard: "d1", folder: folders[1] })}
      />
    )
    expect(screen.queryByRole("button", { name: "Move" })).toBeNull()
  })

  it("is absent for a reader without Edit", () => {
    stubServer()
    renderWithProviders(
      <FindingMoveMenu
        envelope={envelope({ dashboard: null, folder: folders[1] }, false)}
      />
    )
    expect(screen.queryByRole("button", { name: "Move" })).toBeNull()
  })

  it("offers the shared folders the caller can edit and posts the chosen one", async () => {
    const moved = stubServer()
    const { user } = renderWithProviders(
      <FindingMoveMenu
        envelope={envelope({ dashboard: null, folder: folders[0] })}
      />
    )
    await user.click(screen.getByRole("button", { name: "Move" }))
    await screen.findByText("Publish to folder")
    await waitFor(() =>
      expect(screen.getAllByRole("menuitemradio")).toHaveLength(2)
    )
    expect(
      screen.getByRole("menuitemradio", { name: /Ana's folder/ })
    ).toHaveAttribute("aria-checked", "true")
    expect(screen.queryByRole("menuitemradio", { name: "Ops" })).toBeNull()

    await user.click(screen.getByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() => expect(moved).toEqual([{ folder_id: shopId }]))
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Published to Shop")
    )
  })

  it("says Moved when the finding was already shared or in no folder", async () => {
    const moved = stubServer()
    const { user } = renderWithProviders(
      <FindingMoveMenu envelope={envelope({ dashboard: null, folder: null })} />
    )
    await user.click(screen.getByRole("button", { name: "Move" }))
    await screen.findByText("Move to folder")
    await user.click(await screen.findByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() => expect(moved).toHaveLength(1))
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Moved to Shop")
    )
  })

  it("shows the server's message when the move is refused", async () => {
    stubServer(() =>
      json({ error: "you cannot edit that folder", code: "forbidden" }, 403)
    )
    const { user } = renderWithProviders(
      <FindingMoveMenu envelope={envelope({ dashboard: null, folder: null })} />
    )
    await user.click(screen.getByRole("button", { name: "Move" }))
    await user.click(await screen.findByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith(
        "Could not move Slow checkout. You cannot edit that folder."
      )
    )
    expect(toasts.success).not.toHaveBeenCalled()
  })
})
