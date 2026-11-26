import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  releaseHeldMutation,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { invitesQuery, type Invite } from "./invites"
import { InvitesTable } from "./invites-table"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./invites", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./invites")>()),
  useWithdrawInvite: () =>
    useHeldMutation((id: string) => calls.send(id) as Promise<void>),
}))

const invite = {
  id: "i1",
  email: "ada@example.com",
  role: "viewer",
  invited_by: "Grace",
  created_at: "2026-11-11T10:00:00Z",
  sign_in_url: "https://example.com/in",
} as Invite

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("InvitesTable", () => {
  it("reports a withdrawal after the table dropped the invite", async () => {
    const queryClient = testQueryClient()
    queryClient.setQueryData(invitesQuery().queryKey, [invite])
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <InvitesTable />
      </HeldMutationHost>,
      { queryClient }
    )

    await user.click(
      await screen.findByRole("button", {
        name: "Withdraw the invite for ada@example.com",
      })
    )
    expect(calls.send).not.toHaveBeenCalled()
    await user.click(
      await screen.findByRole("button", { name: "Withdraw invite" })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("i1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Withdrew the invite for ada@example.com"
      )
    )
  })

  function listInvites(emails: readonly string[]) {
    const queryClient = testQueryClient()
    const all = emails.map((name) => ({
      ...invite,
      id: `id-${name}`,
      email: `${name}@example.com`,
    }))
    queryClient.setQueryData(invitesQuery().queryKey, all)
    return { queryClient, all }
  }

  function withdrawButton(name: string) {
    return screen.getByRole("button", {
      name: `Withdraw the invite for ${name}@example.com`,
    })
  }

  async function openWithdraw(
    user: ReturnType<typeof renderWithProviders>["user"],
    name: string
  ) {
    await user.click(
      await screen.findByRole("button", {
        name: `Withdraw the invite for ${name}@example.com`,
      })
    )
  }

  it("returns focus to the invite's own Withdraw when the confirm is cancelled", async () => {
    const { queryClient } = listInvites(["ada", "bob"])
    const { user } = renderWithProviders(<InvitesTable />, { queryClient })

    await openWithdraw(user, "ada")
    await user.click(await screen.findByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(withdrawButton("ada")).toHaveFocus())
    expect(calls.send).not.toHaveBeenCalled()
  })

  it("keeps focus on the invite's own Withdraw while it is withdrawn and after it fails", async () => {
    const { queryClient } = listInvites(["ada", "bob"])
    calls.send.mockRejectedValue(new Error("offline"))
    const { user } = renderWithProviders(<InvitesTable />, { queryClient })

    await openWithdraw(user, "ada")
    await user.click(
      await screen.findByRole("button", { name: "Withdraw invite" })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("id-ada"))
    await waitFor(() => expect(withdrawButton("ada")).toHaveFocus())
    await releaseHeldMutation()

    await waitFor(() => expect(calls.toast.error).toHaveBeenCalled())
    expect(withdrawButton("ada")).toHaveFocus()
    expect(withdrawButton("ada")).toHaveAttribute("aria-disabled", "false")
  })

  it.each([
    ["a middle invite", "ada", "cy"],
    ["the first invite", "zoe", "ada"],
    ["the last invite", "bob", "cy"],
  ])(
    "moves focus to the next invite in display order once %s is gone, or the previous one after the last",
    async (_, withdrawn, focused) => {
      const { queryClient, all } = listInvites(["zoe", "ada", "cy", "bob"])
      calls.send.mockImplementation(async (id: string) => {
        queryClient.setQueryData(
          invitesQuery().queryKey,
          all.filter((row) => row.id !== id)
        )
      })
      const { user } = renderWithProviders(<InvitesTable />, { queryClient })

      await openWithdraw(user, withdrawn)
      await user.click(
        await screen.findByRole("button", { name: "Withdraw invite" })
      )
      await waitFor(() =>
        expect(screen.queryByText(`${withdrawn}@example.com`)).toBeNull()
      )

      await waitFor(() => expect(withdrawButton(focused)).toHaveFocus())
      await releaseHeldMutation()
      expect(withdrawButton(focused)).toHaveFocus()
    }
  )
})
