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

  it("moves focus to the next invite's Withdraw once the confirm closes", async () => {
    const queryClient = testQueryClient()
    queryClient.setQueryData(invitesQuery().queryKey, [
      invite,
      { ...invite, id: "i2", email: "bob@example.com" },
    ])
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
    await user.click(
      await screen.findByRole("button", { name: "Withdraw invite" })
    )

    await waitFor(() =>
      expect(
        screen.getByRole("button", {
          name: "Withdraw the invite for bob@example.com",
        })
      ).toHaveFocus()
    )
  })

  it.each([
    ["a middle invite", "b", "c"],
    ["the last invite", "c", "b"],
  ])(
    "moves focus to the neighbour by id once %s is gone from the table",
    async (_, withdrawn, focused) => {
      const queryClient = testQueryClient()
      const all = ["a", "b", "c"].map((id) => ({
        ...invite,
        id,
        email: `${id}@example.com`,
      }))
      queryClient.setQueryData(invitesQuery().queryKey, all)
      calls.send.mockImplementation(async (id: string) => {
        queryClient.setQueryData(
          invitesQuery().queryKey,
          all.filter((row) => row.id !== id)
        )
      })
      const { user } = renderWithProviders(<InvitesTable />, { queryClient })

      await user.click(
        await screen.findByRole("button", {
          name: `Withdraw the invite for ${withdrawn}@example.com`,
        })
      )
      await user.click(
        await screen.findByRole("button", { name: "Withdraw invite" })
      )
      await waitFor(() => expect(calls.send).toHaveBeenCalledWith(withdrawn))
      await releaseHeldMutation()

      await waitFor(() =>
        expect(
          screen.getByRole("button", {
            name: `Withdraw the invite for ${focused}@example.com`,
          })
        ).toHaveFocus()
      )
    }
  )
})
