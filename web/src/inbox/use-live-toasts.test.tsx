import { act, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { inboxKeys } from "@/api/keys"
import type { QueryClient } from "@tanstack/react-query"

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

async function setItems(
  queryClient: QueryClient,
  key: readonly unknown[],
  items: InboxItem[]
) {
  await act(async () => {
    queryClient.setQueryData(key, page(items))
    await new Promise((resolve) => setTimeout(resolve, 5))
  })
}

function Probe() {
  useLiveInboxToasts()
  return <p>mounted</p>
}

function page(items: InboxItem[]) {
  return {
    pages: [{ items, next_cursor: null, unread_count: items.length }],
    pageParams: [undefined],
  }
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise(() => {}))
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
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, key, [mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = toastMock.mock.calls[0]
    expect(title).toBe("Ada mentioned you on Checkout")
    expect(options.toasterId).toBe(liveToasterId)
    expect(options.duration).toBe(liveToastDuration)
    expect(options.action.label).toBe("Open")
  })

  it("makes no toast while the panel is open", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    act(() => useInboxPanelStore.getState().show())
    await setItems(queryClient, key, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("sums more than three into one toast that opens the panel", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(1)])
    const burst = [2, 3, 4, 5, 6].map(mention).reverse()
    await setItems(queryClient, key, [...burst, mention(1)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
    const [title, options] = toastMock.mock.calls[0]
    expect(title).toBe("5 new in Inbox")
    act(() => options.action.onClick())
    expect(useInboxPanelStore.getState().open).toBe(true)
  })

  it("does not toast items that arrived while the tab was hidden", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    setHidden(true)
    setHidden(false)
    await setItems(queryClient, key, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()

    await setItems(queryClient, key, [mention(7), mention(6), mention(5)])
    await waitFor(() => expect(toastMock).toHaveBeenCalledTimes(1))
  })

  it("does not toast items fetched while the tab is hidden", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    setHidden(true)
    await setItems(queryClient, key, [mention(6), mention(5)])
    setHidden(false)
    await setItems(queryClient, key, [mention(7), mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("starts from a first load after a reset", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    resetLiveToasts()
    await setItems(queryClient, key, [mention(6), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })

  it("gives a second person in the same tab no toasts for earlier items", async () => {
    const { queryClient } = renderWithProviders(<Probe />, {
      route: { path: "/", url: "/" },
    })
    const key = inboxKeys.list("for_you", "open")
    await screen.findByText("mounted")
    await setItems(queryClient, key, [mention(5)])
    signedIn.id = "u2"
    await setItems(queryClient, key, [mention(8), mention(5)])
    expect(toastMock).not.toHaveBeenCalled()
  })
})
