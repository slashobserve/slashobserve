import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import { useActivityStream } from "./use-activity-stream"
import { must } from "@/test/must"

type Listener = (event: Event) => void

class FakeEventSource {
  static instances: FakeEventSource[] = []
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

  send(name: string, data = "{}") {
    for (const listener of this.listeners.get(name) ?? []) {
      listener(new MessageEvent(name, { data }))
    }
  }
}

function pageTransition(type: "pagehide" | "pageshow", persisted: boolean) {
  return Object.assign(new Event(type), { persisted })
}

function openSources() {
  return FakeEventSource.instances.filter((source) => !source.closed)
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal("EventSource", FakeEventSource)
})

afterEach(() => vi.unstubAllGlobals())

it("closes the stream while the page sits in the back/forward cache and catches up when it returns", () => {
  const onGap = vi.fn()
  renderHook(() =>
    useActivityStream({
      enabled: true,
      onEvent: vi.fn(),
      onGap,
    })
  )
  expect(openSources()).toHaveLength(1)

  act(() => window.dispatchEvent(pageTransition("pagehide", true)))
  expect(openSources()).toHaveLength(0)

  act(() => window.dispatchEvent(pageTransition("pageshow", true)))
  expect(openSources()).toHaveLength(1)
  act(() => must(openSources()[0]).send("ready"))
  expect(onGap).toHaveBeenCalledTimes(1)
})

it("leaves the stream alone on a first page show", () => {
  renderHook(() =>
    useActivityStream({
      enabled: true,
      onEvent: vi.fn(),
      onGap: vi.fn(),
    })
  )
  act(() => window.dispatchEvent(pageTransition("pageshow", false)))
  expect(FakeEventSource.instances).toHaveLength(1)
})

it("stops reconnecting when the server says the caller lost access, and catches up when the stream is enabled again", () => {
  vi.useFakeTimers()
  try {
    const onGap = vi.fn()
    const { rerender } = renderHook(
      ({ enabled }) => useActivityStream({ enabled, onEvent: vi.fn(), onGap }),
      { initialProps: { enabled: true } }
    )
    const source = must(openSources()[0])
    act(() => source.send("ready"))

    act(() => source.send("closed", '{"reason":"access_lost"}'))
    act(() => {
      vi.advanceTimersByTime(60_000)
    })
    expect(source.closed).toBe(true)
    expect(FakeEventSource.instances).toHaveLength(1)

    rerender({ enabled: false })
    rerender({ enabled: true })
    act(() => must(openSources()[0]).send("ready"))
    expect(onGap).toHaveBeenCalledTimes(1)
  } finally {
    vi.useRealTimers()
  }
})

it.each(["lifetime", "stopping", "check_failed", "read_failed"])(
  "leaves the reconnect to the browser, which resumes from the last event, when the server closed the stream for %s",
  (reason) => {
    renderHook(() =>
      useActivityStream({ enabled: true, onEvent: vi.fn(), onGap: vi.fn() })
    )
    const source = must(openSources()[0])
    act(() => source.send("ready"))

    act(() => source.send("closed", JSON.stringify({ reason })))

    expect(source.closed).toBe(false)
    expect(FakeEventSource.instances).toHaveLength(1)
  }
)
