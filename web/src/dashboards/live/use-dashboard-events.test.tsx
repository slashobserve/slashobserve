import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import { dashboardKeys, threadKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { useDashboardEvents } from "./use-dashboard-events"

const router = vi.hoisted(() => ({
  state: { matches: [] },
  navigate: vi.fn(),
  invalidate: vi.fn(),
}))

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouter: () => router,
}))

type Listener = (event: Event) => void

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static readonly CONNECTING = 0
  static readonly CLOSED = 2
  closed = false
  readyState = 1
  onerror: (() => void) | null = null
  readonly url: string
  private listeners = new Map<string, Listener[]>()

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(name: string, listener: Listener) {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener])
  }

  close() {
    this.closed = true
    this.readyState = 2
  }

  send(name: string, data: unknown) {
    for (const listener of this.listeners.get(name) ?? []) {
      listener(new MessageEvent(name, { data: JSON.stringify(data) }))
    }
  }
}

function pageTransition(type: "pagehide" | "pageshow", persisted: boolean) {
  return Object.assign(new Event(type), { persisted })
}

function openSources() {
  return FakeEventSource.instances.filter((source) => !source.closed)
}

function wrapperWith(client = testQueryClient()) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

const wrapper = wrapperWith()

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal("EventSource", FakeEventSource)
})

afterEach(() => vi.unstubAllGlobals())

it("closes the dashboard stream while the page sits in the back/forward cache and reopens it when the page returns", () => {
  const { result } = renderHook(() => useDashboardEvents("d1", {}), {
    wrapper,
  })
  act(() =>
    openSources()[0].send("ready", { dashboard_id: "d1", latest_version: 1 })
  )
  expect(result.current.status).toBe("live")

  act(() => window.dispatchEvent(pageTransition("pagehide", true)))
  expect(openSources()).toHaveLength(0)

  act(() => window.dispatchEvent(pageTransition("pageshow", true)))
  expect(openSources()).toHaveLength(1)
  expect(openSources()[0].url).toBe("/api/v1/dashboards/d1/events")
})

it("refetches the dashboard and the list when the dashboard moves to another folder", () => {
  const client = testQueryClient()
  const invalidate = vi.spyOn(client, "invalidateQueries")
  renderHook(() => useDashboardEvents("d1", {}), {
    wrapper: wrapperWith(client),
  })
  act(() =>
    openSources()[0].send("dashboard.moved", {
      type: "dashboard.moved",
      dashboard_id: "d1",
    })
  )
  const refetched = invalidate.mock.calls.map(([filters]) => filters?.queryKey)
  expect(refetched).toEqual(
    expect.arrayContaining([dashboardKeys.latest("d1"), dashboardKeys.list()])
  )
})

it("closes the stream 15 seconds after the tab is hidden and catches up when it is shown again", () => {
  vi.useFakeTimers()
  let hidden = false
  const visibility = vi
    .spyOn(document, "hidden", "get")
    .mockImplementation(() => hidden)
  try {
    const client = testQueryClient()
    const invalidate = vi.spyOn(client, "invalidateQueries")
    renderHook(() => useDashboardEvents("d1", {}), {
      wrapper: wrapperWith(client),
    })
    act(() =>
      openSources()[0].send("ready", { dashboard_id: "d1", latest_version: 1 })
    )

    hidden = true
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"))
      vi.advanceTimersByTime(14_999)
    })
    expect(openSources()).toHaveLength(1)
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(openSources()).toHaveLength(0)

    hidden = false
    invalidate.mockClear()
    act(() => document.dispatchEvent(new Event("visibilitychange")))
    expect(openSources()).toHaveLength(1)
    act(() =>
      openSources()[0].send("ready", { dashboard_id: "d1", latest_version: 1 })
    )
    const refetched = invalidate.mock.calls.map(
      ([filters]) => filters?.queryKey
    )
    expect(refetched).toEqual(
      expect.arrayContaining([
        dashboardKeys.history("d1"),
        dashboardKeys.comments("d1"),
        threadKeys.list("dashboard:d1"),
        dashboardKeys.revisions("d1"),
      ])
    )
  } finally {
    visibility.mockRestore()
    vi.useRealTimers()
  }
})

it("refetches the dashboard and the list on every reconnect, so a move made while the stream was closed is caught up", () => {
  const client = testQueryClient()
  client.setQueryData(dashboardKeys.latest("d1"), { version: { version: 3 } })
  const invalidate = vi.spyOn(client, "invalidateQueries")
  renderHook(() => useDashboardEvents("d1", {}), {
    wrapper: wrapperWith(client),
  })
  const refetched = () =>
    invalidate.mock.calls.map(([filters]) => filters?.queryKey)

  act(() =>
    openSources()[0].send("ready", { dashboard_id: "d1", latest_version: 3 })
  )
  expect(refetched()).not.toContainEqual(dashboardKeys.latest("d1"))

  act(() => window.dispatchEvent(pageTransition("pagehide", true)))
  act(() => window.dispatchEvent(pageTransition("pageshow", true)))
  act(() =>
    openSources()[0].send("ready", { dashboard_id: "d1", latest_version: 3 })
  )
  expect(refetched()).toEqual(
    expect.arrayContaining([dashboardKeys.latest("d1"), dashboardKeys.list()])
  )
})

it.each(["comment.created", "comment.updated", "revision.updated"])(
  "refetches the dashboard's threads on %s",
  (event) => {
    const client = testQueryClient()
    const invalidate = vi.spyOn(client, "invalidateQueries")
    renderHook(() => useDashboardEvents("d1", {}), {
      wrapper: wrapperWith(client),
    })
    act(() => openSources()[0].send(event, { dashboard_id: "d1" }))
    expect(
      invalidate.mock.calls.map(([filters]) => filters?.queryKey)
    ).toContainEqual(threadKeys.list("dashboard:d1"))
  }
)

it("checks the dashboard as soon as its stream ends, so a dashboard the caller lost shows as gone without waiting for a reconnect", async () => {
  const requested: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requested.push(new URL(request.url).pathname)
      return new Response(
        JSON.stringify({ error: "dashboard not found", code: "not_found" }),
        { status: 404, headers: { "Content-Type": "application/json" } }
      )
    })
  )
  router.invalidate.mockClear()
  renderHook(() => useDashboardEvents("d1", {}), {
    wrapper: wrapperWith(testQueryClient()),
  })
  const source = openSources()[0]
  act(() => source.send("ready", { dashboard_id: "d1", latest_version: 1 }))

  source.readyState = FakeEventSource.CONNECTING
  act(() => source.onerror?.())
  await waitFor(() => expect(router.invalidate).toHaveBeenCalledTimes(1))
  expect(requested).toEqual(["/api/v1/dashboards/d1"])

  act(() => source.onerror?.())
  await Promise.resolve()
  expect(requested).toEqual(["/api/v1/dashboards/d1"])
})
