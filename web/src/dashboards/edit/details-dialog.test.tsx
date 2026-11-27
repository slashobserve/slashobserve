import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Folder } from "@/folders/queries"
import type { Dashboard } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { DetailsDialog } from "./details-dialog"

const calls = vi.hoisted(() => ({
  save: vi.fn(),
  move: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@/spec", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/spec")>()),
  loadSpecValidator: async () => (spec: unknown) => ({ valid: true, spec }),
}))
vi.mock("../queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../queries")>()),
  useAddVersion: () => ({
    isPending: false,
    reset: () => {},
    mutate: (
      input: unknown,
      options: { onSuccess: (outcome: unknown) => void }
    ) => {
      calls.save(input)
      options.onSuccess({ kind: "saved", version: 4, mergedFrom: null })
    },
  }),
  useMoveDashboard: () => ({
    isPending: false,
    reset: () => {},
    mutate: (
      input: unknown,
      options: { onSuccess: () => void; onError: (error: unknown) => void }
    ) => {
      calls.move(input)
      options.onSuccess()
    },
  }),
}))

const ops = "11111111-1111-4111-8111-111111111111"
const shop = "22222222-2222-4222-8222-222222222222"
const mine = "33333333-3333-4333-8333-333333333333"

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

const spec = { title: "Orders", panels: [] } as unknown as Dashboard

function open(
  folders: Folder[],
  current: { id: string; personal: boolean } = { id: ops, personal: false }
) {
  return renderWithProviders(
    <DetailsDialog
      dashboardId="d1"
      spec={spec}
      version={3}
      folder={current}
      folders={folders}
      open
      onOpenChange={() => {}}
      finalFocus={() => true}
    />
  )
}

afterEach(() => {
  vi.clearAllMocks()
})

describe("DetailsDialog folder", () => {
  it("shows the folder the dashboard is in and the shared folders it can move to", async () => {
    open([
      folder(mine, "My Dashboards", {
        personal: true,
        kind: "personal",
        allowed_actions: ["create_in"],
      }),
      folder(ops, "Ops"),
      folder(shop, "Shop"),
    ])
    const trigger = screen.getByRole("combobox", { name: "Folder" })
    expect(trigger).toHaveTextContent("Ops")
    await userEvent.click(trigger)
    const options = (await screen.findAllByRole("option")).map(
      (option) => option.textContent
    )
    expect(options).toEqual(["Ops", "Shop"])
  })

  it("keeps Save off until something changes", async () => {
    open([folder(ops, "Ops"), folder(shop, "Shop")])
    const save = screen.getByRole("button", { name: "Save" })
    expect(save).toHaveAttribute("aria-disabled", "true")
    await userEvent.click(screen.getByRole("combobox", { name: "Folder" }))
    await userEvent.click(await screen.findByRole("option", { name: "Shop" }))
    expect(save).toHaveAttribute("aria-disabled", "false")
  })

  it("moves without saving a version when only the folder changed", async () => {
    open([folder(ops, "Ops"), folder(shop, "Shop")])
    await userEvent.click(screen.getByRole("combobox", { name: "Folder" }))
    await userEvent.click(await screen.findByRole("option", { name: "Shop" }))
    await userEvent.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(calls.move).toHaveBeenCalled())
    expect(calls.move).toHaveBeenCalledWith({ id: "d1", folderId: shop })
    expect(calls.save).not.toHaveBeenCalled()
    expect(calls.toast.success).toHaveBeenCalledWith("Moved to Shop")
  })

  it("saves the details first and then moves when both changed", async () => {
    open([folder(ops, "Ops"), folder(shop, "Shop")])
    const title = screen.getByRole("textbox", { name: "Title" })
    await userEvent.clear(title)
    await userEvent.type(title, "Orders overview")
    await userEvent.click(screen.getByRole("combobox", { name: "Folder" }))
    await userEvent.click(await screen.findByRole("option", { name: "Shop" }))
    await userEvent.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(calls.move).toHaveBeenCalled())
    expect(calls.save).toHaveBeenCalledOnce()
    expect(calls.save.mock.invocationCallOrder[0]).toBeLessThan(
      calls.move.mock.invocationCallOrder[0]!
    )
  })

  it("saves a version and never moves when the folder is left alone", async () => {
    open([folder(ops, "Ops"), folder(shop, "Shop")])
    const title = screen.getByRole("textbox", { name: "Title" })
    await userEvent.type(title, " 2")
    await userEvent.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(calls.save).toHaveBeenCalled())
    expect(calls.move).not.toHaveBeenCalled()
  })

  it("says Published when the dashboard leaves a personal folder", async () => {
    open(
      [
        folder(mine, "My Dashboards", {
          personal: true,
          kind: "personal",
          allowed_actions: ["create_in"],
        }),
        folder(shop, "Shop"),
      ],
      {
        id: mine,
        personal: true,
      }
    )
    await userEvent.click(screen.getByRole("combobox", { name: "Folder" }))
    await userEvent.click(await screen.findByRole("option", { name: "Shop" }))
    await userEvent.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(calls.move).toHaveBeenCalled())
    expect(calls.toast.success).toHaveBeenCalledWith("Published to Shop")
  })

  it("disables the folder and says why when there is nowhere else to go", () => {
    open([folder(ops, "Ops")])
    expect(screen.getByRole("combobox", { name: "Folder" })).toBeDisabled()
    expect(
      screen.getByText("There is no other folder you can move it to.")
    ).toBeInTheDocument()
  })
})
