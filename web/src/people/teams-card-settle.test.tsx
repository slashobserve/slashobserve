import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { usersQuery } from "./queries"
import { teamsQuery, type Team } from "./teams"
import { TeamsCard } from "./teams-card"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  deleteTeam: vi.fn(),
  member: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./teams", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./teams")>()),
  useDeleteTeam: () =>
    useHeldMutation((id: string) => calls.deleteTeam(id) as Promise<void>),
  useTeamMember: () =>
    useHeldMutation(
      (input: { team: string; user: string; add: boolean }) =>
        calls.member(input) as Promise<void>
    ),
}))

const team = {
  id: "t1",
  name: "Platform",
  members: [{ id: "u1", name: "Ada Lovelace", email: "ada@example.com" }],
} as Team

function renderCard() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(teamsQuery().queryKey, { teams: [team] })
  queryClient.setQueryData(usersQuery().queryKey, [])
  return renderWithProviders(
    <HeldMutationHost>
      <TeamsCard />
    </HeldMutationHost>,
    { queryClient }
  )
}

beforeEach(() => {
  resetHeldMutation()
  calls.deleteTeam.mockResolvedValue(undefined)
  calls.member.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("TeamsCard", () => {
  it("reports a team deletion after the table dropped its row", async () => {
    const { user } = renderCard()

    await user.click(
      await screen.findByRole("button", { name: "Delete team Platform" })
    )
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete team",
      })
    )
    await waitFor(() => expect(calls.deleteTeam).toHaveBeenCalledWith("t1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Deleted team Platform")
    )
  })

  it("reports a member removal after the table dropped the member", async () => {
    const { user } = renderCard()

    await user.click(
      await screen.findByRole("button", {
        name: "Remove Ada Lovelace from Platform",
      })
    )
    await waitFor(() => expect(calls.member).toHaveBeenCalledTimes(1))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Removed Ada Lovelace from Platform"
      )
    )
  })
})
