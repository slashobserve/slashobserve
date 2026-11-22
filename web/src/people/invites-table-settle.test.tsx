import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
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
      screen.getByRole("button", {
        name: "Withdraw the invite for ada@example.com",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("i1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Withdrew the invite for ada@example.com"
      )
    )
  })
})
