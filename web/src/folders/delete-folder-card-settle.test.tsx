import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { DeleteFolderCard } from "./delete-folder-card"
import type { Folder } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  navigate: vi.fn(),
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => calls.navigate,
}))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDeleteFolder: () => useHeldMutation(() => calls.send() as Promise<void>),
}))

const folder = { id: "f1", title: "Shop", dashboard_count: 0 } as Folder

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("DeleteFolderCard", () => {
  it("leaves the deleted folder after the page dropped the card", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <DeleteFolderCard folder={folder} />
      </HeldMutationHost>
    )

    await user.click(screen.getByRole("button", { name: "Delete folder" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete folder",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledTimes(1))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.navigate).toHaveBeenCalledWith({ to: "/dashboards" })
    )
    expect(calls.toast.success).toHaveBeenCalledWith("Deleted Shop")
  })
})
