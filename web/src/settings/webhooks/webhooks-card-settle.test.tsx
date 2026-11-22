import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import type { Webhook } from "./queries"
import { WebhooksCard } from "./webhooks-card"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useDeleteWebhook: () =>
    useHeldMutation((id: string) => calls.send(id) as Promise<void>),
}))

const webhook = {
  id: "w1",
  name: "Dashboard changes",
  url: "https://hooks.example.com/in",
  format: "generic",
  event_types: ["dashboard.moved"],
  owner: { kind: "user", id: "u", label: "Ada" },
  enabled: true,
  state: "sending",
  disabled_reason: null,
  disabled_at: null,
  consecutive_failures: 0,
  next_attempt_at: null,
  waiting_events: { count: 0, more: false },
  created_at: "2026-11-11T09:00:00Z",
  updated_at: "2026-11-11T09:00:00Z",
} as unknown as Webhook

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("WebhooksCard", () => {
  it("reports the deletion of the last webhook after the card was replaced", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <WebhooksCard
          webhooks={[webhook]}
          editingId={null}
          meId="u"
          onEdit={() => {}}
          onSecret={() => {}}
        />
      </HeldMutationHost>
    )

    await user.click(
      screen.getByRole("button", { name: "More for Dashboard changes" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete webhook",
      })
    )
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("w1"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith(
        "Deleted Dashboard changes"
      )
    )
  })
})
