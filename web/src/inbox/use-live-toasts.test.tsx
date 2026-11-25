import { act, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { QueryClient } from "@tanstack/react-query"

import { inboxKeys } from "@/api/keys"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { useInboxPanelStore } from "./inbox-panel-store"
import { liveToastDuration, liveToasterId } from "./live-toasts"
import { resetLiveToasts, useLiveInboxToasts } from "./use-live-toasts"

const toastMock = vi.hoisted(() => vi.fn())
vi.mock("sonner", () => ({ toast: toastMock }))

const signedIn = vi.hoisted(() => ({ id: "u1" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useMatch: () => signedIn.id,
}))

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    value: hidden,
    configurable: true,
  })
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"))
  })
}

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"

function withData(data: Record<string, unknown>): InboxItem["data"] {
  return data as InboxItem["data"]
}

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
    data: withData({ object: { kind: "dashboard", id: dashboardId } }),
    created_at: new Date().toISOString(),
    reason: "mention",
    unread: true,
    target_title: "Checkout",
    excerpt: null,
    question: null,
  } as InboxItem
}

const server = { items: [] as InboxItem[] }

async function setItems(queryClient: QueryClient, items: InboxItem[]) {
  server.items = items
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: inboxKeys.all })
    await new Promise((resolve) => setTimeout(resolve, 5))
  })
}

function Probe() {
  useLiveInboxToasts()
  return <p>mounted</p>
}

function mountWith(items: InboxItem[]) {
  server.items = items
  return renderWithProviders(<Probe />, { route: { path: "/", url: "/" } })
}

async function settle() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 5))
  })
}

beforeEach(() => {
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
  toastMock.mockReset()
  resetLiveToasts()
  signedIn.id = "u1"
  setHidden(false)
  useInboxPanelStore.setState({ open: false })
})

afterEach(() => vi.unstubAllGlobals())

describe("useLiveInboxToasts", () => {
  it("toasts a new mention on the live toaster and not the first load", async () => {
    const { queryClient } = mountWith([mention(5)])
    await screen.findByText("mounted")
    await settle()
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, [mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = toastMock.mock.calls[0]
    expect(title).toBe("Ada mentioned you on Checkout")
    expect(options.toasterId).toBe(liveToasterId)
    expect(options.duration).toBe(liveToastDuration)
    expect(options.action.label).toBe("Open")
  })

  it("makes no toast while the panel is open", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    act(() => useInboxPanelStore.getState().show())
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("sums more than three into one toast that opens the panel", async () => {
    const { queryClient } = mountWith([mention(1)])
    await settle()
    const burst = [2, 3, 4, 5, 6].map(mention).reverse()
    await setItems(queryClient, [...burst, mention(1)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = toastMock.mock.calls[0]
    expect(title).toBe("5 new in Inbox")
    act(() => options.action.onClick())
    expect(useInboxPanelStore.getState().open).toBe(true)
  })

  it("does not toast items that arrived while the tab was hidden", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    setHidden(true)
    setHidden(false)
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, [mention(7), mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
  })

  it("does not toast items fetched while the tab is hidden", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    setHidden(true)
    await setItems(queryClient, [mention(6), mention(5)])
    setHidden(false)
    await setItems(queryClient, [mention(7), mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("starts from a first load after a reset", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    resetLiveToasts()
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("gives a second person in the same tab no toasts for earlier items", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    signedIn.id = "u2"
    await setItems(queryClient, [mention(8), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("opens the panel on the item when it has no page of its own", async () => {
    const { queryClient } = mountWith([mention(5)])
    await settle()
    const pageless = {
      ...mention(6),
      type: "user.invited",
      reason: "proposal",
      unread: false,
      target: { kind: "user", id: "u9" },
    } as InboxItem
    await setItems(queryClient, [pageless, mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    act(() => toastMock.mock.calls[0][1].action.onClick())
    const state = useInboxPanelStore.getState()
    expect(state.open).toBe(true)
    expect(state.item).toBe(6)
    expect(state.view).toBe("for_you")
    expect(state.filter).toBe("open")
  })

  it("toasts an item that arrives after the first load of an empty inbox", async () => {
    const { queryClient } = mountWith([])
    await settle()
    await setItems(queryClient, [mention(1)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
  })
})
