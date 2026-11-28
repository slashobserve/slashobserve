import type { ReactNode } from "react"
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import { inboxKeys, type InboxView } from "@/api/keys"
import { ActivityStreamProvider } from "@/live/activity-stream"
import { FakeEventSource } from "@/test/fake-event-source"
import { testQueryClient } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { useInboxPanelStore } from "./inbox-panel-store"
import {
  inboxPollInterval,
  keepInboxHead,
  useInboxCount,
  useInboxLive,
  useMarkInboxRead,
  usePanelInboxList,
  useSetInboxItemDone,
} from "./queries"
import { must } from "@/test/must"

function item(seq: number): InboxItem {
  return { seq } as InboxItem
}

function page(seqs: number[], next: string | null = null) {
  return { items: seqs.map(item), next_cursor: next, unread_count: 4 }
}

let requests: URL[] = []

function wrapperFor(client: QueryClient, streamed: boolean) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      {streamed ? (
        <ActivityStreamProvider>{children}</ActivityStreamProvider>
      ) : (
        children
      )}
    </QueryClientProvider>
  )
}

function seedTwoPages(client: QueryClient) {
  client.setQueryData(inboxKeys.list("for_you", "open"), {
    pages: [page([9, 8], "c2"), page([7, 6])],
    pageParams: [undefined, "c2"],
  })
}

beforeEach(() => {
  requests = []
  FakeEventSource.instances = []
  vi.stubGlobal("EventSource", FakeEventSource)
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requests.push(new URL(request.url))
      return new Response(JSON.stringify(page([10, 9])), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  useInboxPanelStore.setState({ open: false, view: "for_you", filter: "open" })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("inboxPollInterval", () => {
  test("polls every 15 s outside the shell while the page is visible", () => {
    expect(inboxPollInterval(false, true)).toBe(15_000)
  })

  test("never polls inside the shell, whose activity stream keeps it live", () => {
    expect(inboxPollInterval(true, true)).toBe(false)
    expect(inboxPollInterval(true, false)).toBe(false)
  })

  test("never polls while the page is hidden", () => {
    expect(inboxPollInterval(false, false)).toBe(false)
  })
})

describe("useInboxCount", () => {
  function intervalWhen(streamed: boolean) {
    const client = testQueryClient()
    renderHook(() => useInboxCount(), {
      wrapper: wrapperFor(client, streamed),
    })
    return client
      .getQueryCache()
      .find({ queryKey: inboxKeys.list("for_you", "open") })?.observers[0]
      ?.options.refetchInterval
  }

  test("reads the count from the first page of For you", async () => {
    const client = testQueryClient()
    const { result } = renderHook(() => useInboxCount(), {
      wrapper: wrapperFor(client, false),
    })
    await waitFor(() => expect(result.current).toBe(4))
    expect(must(requests[0]).searchParams.get("view")).toBe("for_you")
    expect(must(requests[0]).searchParams.get("filter")).toBe("open")
  })

  test("polls For you on a page outside the shell and not inside it", () => {
    expect(intervalWhen(false)).toBe(15_000)
    expect(intervalWhen(true)).toBe(false)
  })
})

describe("inbox mutations", () => {
  function unreadItem(seq: number): InboxItem {
    return { seq, unread: true, reason: "mention" } as InboxItem
  }

  function seedUnread(client: QueryClient) {
    client.setQueryData(inboxKeys.list("for_you", "open"), {
      pages: [
        {
          items: [unreadItem(9), unreadItem(8)],
          next_cursor: null,
          unread_count: 2,
        },
      ],
      pageParams: [undefined],
    })
  }

  function holdWrites() {
    const answer = { send: () => {} }
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = new URL(request.url)
        requests.push(url)
        if (request.method === "GET")
          return new Response(JSON.stringify(page([9, 8])), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        return new Promise<Response>((resolve) => {
          answer.send = () => resolve(new Response(null, { status: 204 }))
        })
      })
    )
    return answer
  }

  function renderWith(client: QueryClient) {
    return renderHook(
      () => ({
        count: useInboxCount(),
        markRead: useMarkInboxRead(),
        setDone: useSetInboxItemDone(),
      }),
      { wrapper: wrapperFor(client, true) }
    )
  }

  test("Mark all read drops the badge before the server answers, then refetches", async () => {
    const answer = holdWrites()
    const client = testQueryClient()
    seedUnread(client)
    const { result } = renderWith(client)
    expect(result.current.count).toBe(2)

    act(() => result.current.markRead.mutate(9))

    await waitFor(() => expect(result.current.count).toBe(0))
    expect(requests.some((url) => url.pathname === "/api/v1/inbox")).toBe(false)
    act(() => answer.send())
    await waitFor(() => expect(requests.at(-1)?.pathname).toBe("/api/v1/inbox"))
  })

  test("marking one item done drops the badge by one at once", async () => {
    holdWrites()
    const client = testQueryClient()
    seedUnread(client)
    const { result } = renderWith(client)

    act(() => result.current.setDone.mutate({ seq: 8, done: true }))

    await waitFor(() => expect(result.current.count).toBe(1))
  })

  test("puts the count back when the write fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 500 }))
    )
    const client = testQueryClient()
    seedUnread(client)
    const { result } = renderWith(client)

    act(() => result.current.markRead.mutate(9))

    await waitFor(() => expect(result.current.markRead.isError).toBe(true))
    expect(result.current.count).toBe(2)
  })
})

describe("keepInboxHead", () => {
  test("drops every page after the first", () => {
    const client = testQueryClient()
    seedTwoPages(client)

    keepInboxHead(client, "for_you", "open")

    expect(client.getQueryData(inboxKeys.list("for_you", "open"))).toEqual({
      pages: [page([9, 8], "c2")],
      pageParams: [undefined],
    })
  })
})

describe("usePanelInboxList", () => {
  function intervalWhen(streamed: boolean, view: InboxView = "mentions") {
    const client = testQueryClient()
    renderHook(() => usePanelInboxList(view, "open"), {
      wrapper: wrapperFor(client, streamed),
    })
    return client
      .getQueryCache()
      .find({ queryKey: inboxKeys.list(view, "open") })?.observers[0]?.options
      .refetchInterval
  }

  test("polls the open panel's list on a page outside the shell and not inside it", () => {
    expect(intervalWhen(false)).toBe(15_000)
    expect(intervalWhen(true)).toBe(false)
  })

  test("leaves For you to the poll it always has", () => {
    expect(intervalWhen(false, "for_you")).toBe(false)
  })
})

describe("useInboxLive", () => {
  const event = {
    seq: 1,
    type: "dashboard.created",
    target: { kind: "dashboard", id: "d1" },
    dashboard_id: "d1",
    folder_id: "f1",
    actor: { kind: "user", id: "u1", run_id: null },
    data: {},
    created_at: "2026-11-25T10:00:00Z",
  }

  function mount() {
    vi.useFakeTimers()
    const client = testQueryClient()
    for (const view of ["for_you", "mentions", "proposals"] as const) {
      client.setQueryData(inboxKeys.list(view, "open"), {
        pages: [page([1])],
        pageParams: [undefined],
      })
    }
    renderHook(() => useInboxLive(), { wrapper: wrapperFor(client, true) })
    const invalidated = () =>
      client
        .getQueryCache()
        .getAll()
        .filter((query) => query.state.isInvalidated)
        .map((query) => query.queryKey.join("/"))
        .sort()
    const reset = () =>
      client
        .getQueryCache()
        .getAll()
        .forEach((query) => query.reset())
    return { invalidated, reset }
  }

  function emit() {
    act(() => must(FakeEventSource.instances[0]).emit("activity", event))
  }

  test("refreshes only For you at once on the first event while the panel is closed", () => {
    const { invalidated } = mount()
    emit()
    expect(invalidated()).toEqual(["inbox/list/for_you/open"])
  })

  test("also refreshes the list the open panel shows, and nothing else", () => {
    useInboxPanelStore.setState({ open: true, view: "mentions" })
    const { invalidated } = mount()
    emit()
    expect(invalidated()).toEqual([
      "inbox/list/for_you/open",
      "inbox/list/mentions/open",
    ])
  })

  test("keeps refreshing once a second under events that never stop", async () => {
    const { invalidated, reset } = mount()
    let refreshes = 0
    for (let ms = 0; ms < 3000; ms += 100) {
      emit()
      if (invalidated().length > 0) {
        refreshes++
        reset()
      }
      await act(() => vi.advanceTimersByTimeAsync(100))
    }
    if (invalidated().length > 0) refreshes++
    expect(refreshes).toBe(4)
  })
})
