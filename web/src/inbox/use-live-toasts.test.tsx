import { act, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { QueryClient } from "@tanstack/react-query"

import { inboxKeys } from "@/api/keys"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { useInboxPanelStore } from "./inbox-panel-store"
import { liveToastDuration, liveToasterId } from "./live-toasts"
import { resetLiveToasts, useLiveInboxToasts } from "./use-live-toasts"
import { must } from "@/test/must"

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
    created_at: "2026-11-25T08:00:00Z",
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
  })
}

function Probe() {
  useLiveInboxToasts()
  return <p>mounted</p>
}

async function mounted(items: InboxItem[]) {
  server.items = items
  const result = renderWithProviders(<Probe />, {
    route: { path: "/", url: "/" },
  })
  await waitFor(() =>
    expect(
      result.queryClient.getQueryData(inboxKeys.list("for_you", "open"))
    ).toBeDefined()
  )
  await act(async () => {})
  return result
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
    const { queryClient } = await mounted([mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, [mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = must(toastMock.mock.calls[0])
    expect(title).toBe("Ada mentioned you on Checkout")
    expect(options.toasterId).toBe(liveToasterId)
    expect(options.duration).toBe(liveToastDuration)
    expect(options.action.label).toBe("Open")
  })

  it("makes no toast while the panel is open", async () => {
    const { queryClient } = await mounted([mention(5)])
    act(() => useInboxPanelStore.getState().show())
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("sums more than three into one toast that opens the panel", async () => {
    const { queryClient } = await mounted([mention(1)])
    const burst = [2, 3, 4, 5, 6].map(mention).reverse()
    await setItems(queryClient, [...burst, mention(1)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = must(toastMock.mock.calls[0])
    expect(title).toBe("5 new in Inbox")
    act(() => options.action.onClick())
    expect(useInboxPanelStore.getState().open).toBe(true)
  })

  it("toasts items that arrived while the tab was hidden once it returns", async () => {
    const { queryClient } = await mounted([mention(5)])
    setHidden(true)
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    setHidden(false)
    await act(async () => {})
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    expect(must(toastMock.mock.calls[0])[0]).toBe(
      "Ada mentioned you on Checkout"
    )
  })

  it("sums items missed while hidden into one summary on return", async () => {
    const { queryClient } = await mounted([mention(1)])
    setHidden(true)
    const burst = [2, 3, 4, 5].map(mention).reverse()
    await setItems(queryClient, [...burst, mention(1)])
    setHidden(false)
    await act(async () => {})
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    expect(must(toastMock.mock.calls[0])[0]).toBe("4 new in Inbox")
  })

  it("toasts nothing on return when nothing arrived while hidden", async () => {
    const { queryClient } = await mounted([mention(5)])
    setHidden(true)
    setHidden(false)
    await setItems(queryClient, [mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, [mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
  })

  it("starts from a first load after a reset", async () => {
    const { queryClient } = await mounted([mention(5)])
    resetLiveToasts()
    await setItems(queryClient, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("gives a second person in the same tab no toasts for earlier items", async () => {
    const { queryClient } = await mounted([mention(5)])
    signedIn.id = "u2"
    await setItems(queryClient, [mention(8), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("opens the panel on the item when it has no page of its own", async () => {
    const { queryClient } = await mounted([mention(5)])
    const pageless = {
      ...mention(6),
      type: "user.invited",
      reason: "proposal",
      unread: false,
      target: { kind: "user", id: "u9" },
    } as InboxItem
    await setItems(queryClient, [pageless, mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    act(() => must(toastMock.mock.calls[0])[1].action.onClick())
    const state = useInboxPanelStore.getState()
    expect(state.open).toBe(true)
    expect(state.item).toBe(6)
    expect(state.view).toBe("for_you")
    expect(state.filter).toBe("open")
  })

  it("toasts an item that arrives after the first load of an empty inbox", async () => {
    const { queryClient } = await mounted([])
    await setItems(queryClient, [mention(1)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
  })
})
