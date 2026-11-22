import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { DisconnectButton } from "./disconnect-button"
import type { ConnectedAgent } from "./rows"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDisconnect: () =>
    useHeldMutation((id: string) => calls.send(id) as Promise<void>),
}))

const grant = {
  id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
  client: { id: "c1", name: "Claude Code", redirect_host: "x", local: true },
  person: { id: "p1", name: "Ada Lovelace", email: "ada@example.com" },
  created_at: "2026-11-11T10:00:00Z",
  last_used_at: null,
  expires_at: "2027-02-09T10:00:00Z",
  state: "active",
  mode: "direct",
  presence: "offline",
  last_seen_at: null,
} as ConnectedAgent

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("DisconnectButton", () => {
  it("reports the disconnect after the list dropped its row", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <DisconnectButton grant={grant} scope="mine" />
      </HeldMutationHost>
    )

    await user.click(
      screen.getByRole("button", { name: "Disconnect Claude Code" })
    )
    await user.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Disconnect",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith(grant.id))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Disconnected Claude Code"
      )
    )
  })
})
