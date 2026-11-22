import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { ProposalActions } from "./proposal-actions"
import type { Proposal } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  discard: vi.fn(),
  navigate: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => calls.navigate,
}))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDiscardProposal: () =>
    useHeldMutation((reason: string) => calls.discard(reason) as Promise<void>),
}))

const proposal = {
  id: "p1",
  state: "open",
  revision: 1,
  conflicts: [],
  allowed_actions: ["discard"],
} as unknown as Proposal

beforeEach(() => {
  resetHeldMutation()
  calls.discard.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("ProposalActions", () => {
  it("reports a discard after the proposal stopped being open", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <ProposalActions
          proposal={proposal}
          picks={{}}
          onPicksChange={() => {}}
        />
      </HeldMutationHost>
    )

    await user.click(screen.getByRole("button", { name: "Discard" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Discard",
      })
    )
    await waitFor(() => expect(calls.discard).toHaveBeenCalledWith(""))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Discarded")
    )
  })
})
