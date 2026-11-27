import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { folderKeys } from "@/api/keys"
import type { Folder } from "@/folders/queries"
import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { SavedSearch } from "./queries"
import { SavedSearchRowMenu } from "./saved-search-row-menu"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  remove: vi.fn(),
  move: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDeleteSavedSearch: () =>
    useHeldMutation(() => calls.remove() as Promise<void>),
  useMoveSavedSearch: () =>
    useHeldMutation((id: string) => calls.move(id) as Promise<void>),
}))

const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"

function folder(id: string, title: string) {
  return {
    id,
    title,
    general: false,
    personal: false,
    kind: "open",
    allowed_actions: [
      "create_in",
      "move_into",
      "rename",
      "set_permissions",
      "delete",
    ],
    permissions: { comment: true, edit: true, manage: true, create: true },
  } as Folder
}

const saved = {
  id: "44444444-4444-4444-8444-444444444444",
  kind: "logs",
  title: "Checkout errors",
  folder: { id: opsId, title: "Ops", general: false, personal: false },
  permissions: { comment: true, edit: true, manage: false, create: false },
} as SavedSearch

function renderRow() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(folderKeys.list(), [
    folder(shopId, "Shop"),
    folder(opsId, "Ops"),
  ])
  return renderWithProviders(
    <HeldMutationHost>
      <SavedSearchRowMenu saved={saved} />
    </HeldMutationHost>,
    { queryClient }
  )
}

beforeEach(() => {
  resetHeldMutation()
  calls.remove.mockResolvedValue(undefined)
  calls.move.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("SavedSearchRowMenu", () => {
  it("reports a deletion after the list dropped its row", async () => {
    const { user } = renderRow()

    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete..." }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete saved search",
      })
    )
    await waitFor(() => expect(calls.remove).toHaveBeenCalledTimes(1))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Deleted Checkout errors"
      )
    )
  })

  it("reports a move after the list dropped its row", async () => {
    const { user } = renderRow()

    await user.click(
      screen.getByRole("button", { name: "More for Checkout errors" })
    )
    await user.click(await screen.findByRole("menuitemradio", { name: "Shop" }))
    await waitFor(() => expect(calls.move).toHaveBeenCalledWith(shopId))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Moved to Shop")
    )
  })
})
