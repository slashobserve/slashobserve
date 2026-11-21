import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import { testQueryClient } from "@/test/render"

import { useDashboardEvents } from "./use-dashboard-events"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouter: () => ({
    state: { matches: [] },
    navigate: vi.fn(),
    invalidate: vi.fn(),
  }),
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

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={testQueryClient()}>
      {children}
    </QueryClientProvider>
  )
}

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
