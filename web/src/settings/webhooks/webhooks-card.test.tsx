import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { Webhook } from "./queries"
import { WebhooksCard } from "./webhooks-card"

const fetchMock = vi.hoisted(() => vi.fn())

const stuck: Webhook = {
  id: "6c1f0f1e-6a53-4c8e-9c63-2f4d5f0b8a11",
  name: "Dashboard changes",
  url: "https://hooks.example.com/in",
  format: "generic",
  event_types: ["dashboard.moved"],
  owner: {
    kind: "user",
    id: "u",
    name: "Ada",
    origin: null,
    client: null,
    run_id: null,
    label: "Ada",
  },
  enabled: true,
  state: "retrying",
  disabled_reason: null,
  disabled_at: null,
  consecutive_failures: 21,
  next_attempt_at: "2999-01-01T00:00:00Z",
  waiting_events: { count: 3, more: false },
  created_at: "2026-11-11T09:00:00Z",
  updated_at: "2026-11-11T09:00:00Z",
}

function renderCard(webhook: Webhook, meId = "u") {
  return renderWithProviders(
    <WebhooksCard
      webhooks={[webhook]}
      editingId={null}
      meId={meId}
      onEdit={vi.fn()}
      onSecret={vi.fn()}
    />
  )
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal("fetch", fetchMock)
  return () => vi.unstubAllGlobals()
})

describe("WebhooksCard", () => {
  it("says how many events wait behind a retrying webhook and when it tries next", () => {
    renderCard(stuck)
    expect(screen.getByText("21 attempts in a row failed")).toBeInTheDocument()
    expect(screen.getByText(/3 events waiting/)).toHaveTextContent(
      /next attempt/
    )
  })

  it("offers Skip stuck event only while the webhook is failing, and posts the skip after a confirmation", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          webhook: { ...stuck, state: "sending", consecutive_failures: 0 },
          skipped: {
            id: "9",
            delivery_id: "d.9",
            event_type: "dashboard.moved",
            activity_seq: 9,
            attempted_at: "2026-11-11T09:30:00Z",
            duration_ms: 0,
            status_code: null,
            outcome: "skipped",
            error_code: "skipped_by_admin",
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    )
    const { user } = renderCard(stuck)
    await user.click(
      screen.getByRole("button", { name: "More for Dashboard changes" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Skip stuck event" })
    )
    expect(fetchMock).not.toHaveBeenCalled()
    await user.click(await screen.findByRole("button", { name: "Skip event" }))
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.method).toBe("POST")
    expect(new URL(request.url).pathname).toBe(
      `/api/v1/webhooks/${stuck.id}/skip`
    )
  })

  it("hides Skip stuck event on a healthy webhook", async () => {
    const { user } = renderCard({
      ...stuck,
      state: "sending",
      consecutive_failures: 0,
      waiting_events: null,
    })
    await user.click(
      screen.getByRole("button", { name: "More for Dashboard changes" })
    )
    expect(await screen.findByRole("menuitem", { name: "Edit" })).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: "Skip stuck event" })
    ).toBeNull()
  })

  it("offers Make me the owner only on another person's webhook, and patches the owner after a confirmation", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({ ...stuck, owner: { ...stuck.owner, id: "me" } }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }
      )
    )
    const { user } = renderCard(stuck, "me")
    await user.click(
      screen.getByRole("button", { name: "More for Dashboard changes" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Make me the owner" })
    )
    expect(fetchMock).not.toHaveBeenCalled()
    expect(
      await screen.findByText(/including events in your personal folder/)
    ).toBeVisible()
    await user.click(
      await screen.findByRole("button", { name: "Make me the owner" })
    )
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.method).toBe("PATCH")
    expect(await request.json()).toEqual({ owner: "me" })
  })

  it("hides Make me the owner on a webhook the caller already owns", async () => {
    const { user } = renderCard(stuck, "u")
    await user.click(
      screen.getByRole("button", { name: "More for Dashboard changes" })
    )
    expect(await screen.findByRole("menuitem", { name: "Edit" })).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: "Make me the owner" })
    ).toBeNull()
  })
})
