import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { InboxItem } from "@/inbox/inbox-item"
import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import { renderWithProviders } from "@/test/render"

import { NeedsYouList } from "./activity-lists"

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"

function item(over: Partial<InboxItem>): InboxItem {
  return {
    seq: 1,
    type: "dashboard.version_created",
    target: { kind: "dashboard", id: dashboardId },
    dashboard_id: dashboardId,
    folder_id: null,
    actor: {
      kind: "user",
      id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    data: { title: "Checkout" } as unknown as InboxItem["data"],
    created_at: new Date().toISOString(),
    reason: "proposal_closed",
    unread: true,
    target_title: "Checkout",
    excerpt: null,
    question: null,
    ...over,
  }
}

const linked = item({ seq: 12 })
const unlinked = item({
  seq: 7,
  type: "annotation.created",
  target: { kind: "annotation", id: "8d1e3f2c-0b5a-4c7d-9e11-2a6b8c4d5e72" },
  dashboard_id: null,
  target_title: null,
})

function mockServer() {
  const states: { path: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname.startsWith("/api/v1/inbox/items/")) {
        states.push({ path: url.pathname, body: await request.clone().json() })
        return new Response(null, { status: 204 })
      }
      return new Response(
        JSON.stringify({
          items: [linked, unlinked],
          next_cursor: null,
          unread_count: 2,
        }),
        { headers: { "Content-Type": "application/json" } }
      )
    })
  )
  return states
}

function row(seq: number): Promise<HTMLElement> {
  return waitFor(() => {
    const element = document.querySelector<HTMLElement>(
      `[data-inbox-row="${seq}"]`
    )
    if (element === null) throw new Error(`no row ${seq}`)
    return element
  })
}

beforeEach(() =>
  useInboxPanelStore.setState({
    open: false,
    view: "mentions",
    filter: "done",
    item: null,
  })
)

afterEach(() => vi.unstubAllGlobals())

describe("NeedsYouList", () => {
  it("marks an unread item read when the person opens its page", async () => {
    const states = mockServer()
    const { user } = renderWithProviders(<NeedsYouList />, {
      route: { path: "/", url: "/" },
    })

    await user.click(await row(12))

    await waitFor(() =>
      expect(states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { unread: false } },
      ])
    )
  })

  it("opens the Inbox on an item with no page of its own, expanded there", async () => {
    const states = mockServer()
    const { user } = renderWithProviders(<NeedsYouList />, {
      route: { path: "/", url: "/" },
    })

    await user.click(await row(7))

    expect(useInboxPanelStore.getState()).toMatchObject({
      open: true,
      view: "for_you",
      filter: "open",
      item: 7,
    })
    await waitFor(() =>
      expect(states).toEqual([
        { path: "/api/v1/inbox/items/7", body: { unread: false } },
      ])
    )
  })

  it("opens the Inbox from Open Inbox", async () => {
    mockServer()
    const { user } = renderWithProviders(<NeedsYouList />, {
      route: { path: "/", url: "/" },
    })

    await user.click(await screen.findByRole("button", { name: "Open Inbox" }))

    expect(useInboxPanelStore.getState().open).toBe(true)
  })
})
