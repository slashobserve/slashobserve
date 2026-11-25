import { act, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { inboxKeys } from "@/api/keys"
import { panelWidth } from "@/components/dock/card-width"
import { DockPanel } from "@/components/dock/dock-panel"
import { LiveToaster } from "@/components/ui/sonner"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { InboxPanel } from "./inbox-panel"
import { useInboxPanelStore } from "./inbox-panel-store"
import { resetLiveToasts } from "./use-live-toasts"

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"
const server = { items: [] as InboxItem[] }

function mention(seq: number): InboxItem {
  return {
    seq,
    type: "thread.mentioned",
    target: { kind: "thread", id: `t${seq}` },
    dashboard_id: dashboardId,
    folder_id: null,
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    data: { object: { kind: "dashboard", id: dashboardId } },
    created_at: "2026-11-25T08:00:00Z",
    reason: "mention",
    unread: true,
    target_title: "Checkout",
    excerpt: null,
    question: null,
  } as unknown as InboxItem
}

function Page() {
  return (
    <>
      <DockPanel
        panelKey="discussion"
        labelledBy="discussion"
        width={420}
        bounds={panelWidth}
        onWidthChange={() => undefined}
        stacked="container"
      >
        <h2 id="discussion">Discussion</h2>
      </DockPanel>
      <InboxPanel />
      <LiveToaster />
    </>
  )
}

beforeEach(() => {
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    }
  )
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const limit = Number(new URL(request.url).searchParams.get("limit"))
      return new Response(
        JSON.stringify({
          items: server.items.slice(0, limit),
          next_cursor: null,
          unread_count: server.items.length,
        }),
        { headers: { "Content-Type": "application/json" } }
      )
    })
  )
  resetLiveToasts()
  useInboxPanelStore.setState(useInboxPanelStore.getInitialState())
})

afterEach(() => vi.unstubAllGlobals())

describe("the summary toast", () => {
  it("opens the Inbox panel on the first click and hides the page's panel", async () => {
    server.items = [mention(1)]
    const { queryClient, user } = renderWithProviders(<Page />, {
      route: { path: "/", url: "/" },
    })
    await waitFor(() =>
      expect(
        queryClient.getQueryData(inboxKeys.list("for_you", "open"))
      ).toBeDefined()
    )
    await act(async () => {})
    server.items = [2, 3, 4, 5, 6, 7, 8, 9, 10]
      .map(mention)
      .reverse()
      .concat(mention(1))
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: inboxKeys.all })
    })

    const open = await screen.findByRole("button", { name: "Open" })
    await user.click(open)

    await waitFor(() => expect(useInboxPanelStore.getState().open).toBe(true))
    expect(
      screen
        .getByRole("heading", { name: "Discussion", hidden: true })
        .closest("aside")
    ).toHaveProperty("hidden", true)
    expect(screen.getByRole("heading", { name: "Inbox" })).toBeTruthy()
  })
})
