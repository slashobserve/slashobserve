import { createRef } from "react"
import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { AccountsTable } from "./accounts-table"
import type { ServiceAccount } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

const account = {
  id: "a1",
  name: "ci-bot",
  description: "",
  role: "viewer",
  provisioned: false,
} as ServiceAccount

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./use-all-keys", () => ({
  useAllKeys: () => ({ status: "success", accounts: [account], rows: [] }),
}))
vi.mock("./delete-account", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./delete-account")>()),
  useDeleteServiceAccount: () =>
    useHeldMutation((id: string) => calls.send(id) as Promise<void>),
}))

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("AccountsTable", () => {
  it("reports the deletion after the table dropped the account", async () => {
    const focus = document.createElement("button")
    document.body.append(focus)
    const deleteFocus = createRef<HTMLElement>()
    deleteFocus.current = focus
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <AccountsTable deleteFocus={deleteFocus} />
      </HeldMutationHost>
    )

    await user.click(
      screen.getByRole("button", { name: "Delete service account ci-bot" })
    )
    await user.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Delete account",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("a1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Deleted service account ci-bot"
      )
    )
    expect(document.activeElement).toBe(focus)
    focus.remove()
  })
})
