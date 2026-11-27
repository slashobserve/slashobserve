import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import { renderWithProviders } from "@/test/render"

import { BulkMoveBar } from "./bulk-move-bar"
import type { DashboardSummary } from "./dashboard-columns"

const toasts = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))
vi.mock("sonner", () => ({ toast: toasts }))

const ops = "11111111-1111-4111-8111-111111111111"
const shop = "22222222-2222-4222-8222-222222222222"
const mine = "33333333-3333-4333-8333-333333333333"
const locked = "44444444-4444-4444-8444-444444444444"

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return {
    id,
    title,
    general: false,
    personal: false,
    kind: "open",
    allowed_actions: ["create_in", "move_into"],
    permissions: { edit: true },
    ...extra,
  } as Folder
}

const folders = [
  folder(mine, "My Dashboards", {
    personal: true,
    kind: "personal",
    allowed_actions: ["create_in"],
  }),
  folder(ops, "Ops"),
  folder(shop, "Shop"),
  folder(locked, "Locked", {
    allowed_actions: [],
    permissions: { edit: false } as never,
  }),
]

function dashboard(id: string, title: string, folderId: string) {
  return {
    id,
    title,
    folder: { id: folderId, title: "x", general: false, personal: false },
  } as DashboardSummary
}

type Call = { id: string; folder: string }

function stubMoves(failFor: string[] = []) {
  const calls: Call[] = []
  const fetch = vi.fn(async (request: Request) => {
    const id = new URL(request.url).pathname.split("/").pop() ?? ""
    const body = (await request.json()) as { folder_id: string }
    calls.push({ id, folder: body.folder_id })
    if (failFor.includes(id)) {
      return new Response(
        JSON.stringify({ error: { code: "forbidden", message: "no edit" } }),
        { status: 403, headers: { "Content-Type": "application/json" } }
      )
    }
    return new Response(JSON.stringify({ id }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
  })
  vi.stubGlobal("fetch", fetch)
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

async function chooseFolder(title: string) {
  await userEvent.click(
    screen.getByRole("combobox", {
      name: "Move the selected dashboards to this folder",
    })
  )
  await userEvent.click(await screen.findByRole("option", { name: title }))
}

describe("BulkMoveBar", () => {
  it("shows how many are selected and offers only folders the caller can move into", async () => {
    renderWithProviders(
      <BulkMoveBar
        selected={[dashboard("a", "Orders", ops), dashboard("b", "Fleet", ops)]}
        folders={folders}
        onMoved={() => {}}
        onClear={() => {}}
      />
    )
    expect(screen.getByRole("status")).toHaveTextContent("2 selected")
    await userEvent.click(
      screen.getByRole("combobox", {
        name: "Move the selected dashboards to this folder",
      })
    )
    const options = (await screen.findAllByRole("option")).map(
      (option) => option.textContent
    )
    expect(options).toEqual(["Ops", "Shop"])
  })

  it("keeps Move disabled until a folder is chosen", async () => {
    renderWithProviders(
      <BulkMoveBar
        selected={[dashboard("a", "Orders", ops)]}
        folders={folders}
        onMoved={() => {}}
        onClear={() => {}}
      />
    )
    expect(screen.getByRole("button", { name: "Move" })).toBeDisabled()
    await chooseFolder("Shop")
    expect(screen.getByRole("button", { name: "Move" })).toBeEnabled()
  })

  it("moves each selected dashboard once, skips the ones already there and reports it", async () => {
    const calls = stubMoves()
    const onMoved = vi.fn()
    renderWithProviders(
      <BulkMoveBar
        selected={[
          dashboard("a", "Orders", ops),
          dashboard("b", "Fleet", ops),
          dashboard("c", "Nodes", shop),
        ]}
        folders={folders}
        onMoved={onMoved}
        onClear={() => {}}
      />
    )
    await chooseFolder("Shop")
    await userEvent.click(screen.getByRole("button", { name: "Move" }))
    await waitFor(() => expect(onMoved).toHaveBeenCalled())
    expect(calls).toEqual([
      { id: "a", folder: shop },
      { id: "b", folder: shop },
    ])
    expect(onMoved).toHaveBeenCalledWith(["a", "b", "c"])
    expect(toasts.success).toHaveBeenCalledWith(
      "Moved 2 dashboards to Shop. 1 was already there."
    )
  })

  it("keeps the ones that failed selected and names them", async () => {
    stubMoves(["b"])
    const onMoved = vi.fn()
    renderWithProviders(
      <BulkMoveBar
        selected={[dashboard("a", "Orders", ops), dashboard("b", "Fleet", ops)]}
        folders={folders}
        onMoved={onMoved}
        onClear={() => {}}
      />
    )
    await chooseFolder("Shop")
    await userEvent.click(screen.getByRole("button", { name: "Move" }))
    await waitFor(() => expect(onMoved).toHaveBeenCalled())
    expect(onMoved).toHaveBeenCalledWith(["a"])
    expect(toasts.error).toHaveBeenCalledWith(
      expect.stringContaining(
        "Moved 1 of 2 dashboards to Shop. Could not move Fleet ("
      )
    )
  })

  it("clears the selection on request", async () => {
    const onClear = vi.fn()
    renderWithProviders(
      <BulkMoveBar
        selected={[dashboard("a", "Orders", ops)]}
        folders={folders}
        onMoved={() => {}}
        onClear={onClear}
      />
    )
    await userEvent.click(
      screen.getByRole("button", { name: "Clear selection" })
    )
    expect(onClear).toHaveBeenCalledOnce()
  })
})
