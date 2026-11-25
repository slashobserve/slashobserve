import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { ConnectionsCard } from "./connections-card"
import type { GrafanaConnection } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDeleteConnection: () =>
    useHeldMutation((id: string) => calls.send(id) as Promise<void>),
}))

const connection = {
  id: "c1",
  name: "Prod Grafana",
  url: "https://grafana.example.com",
  last_test: null,
  last_sync: null,
} as unknown as GrafanaConnection

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("ConnectionsCard", () => {
  it("reports the deletion of the last connection after the card was replaced", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <ConnectionsCard
          connections={[connection]}
          onImport={() => {}}
          onEdit={() => {}}
        />
      </HeldMutationHost>
    )

    await user.click(
      await screen.findByRole("button", { name: "More for Prod Grafana" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete connection",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("c1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Deleted Prod Grafana")
    )
  })
})
