import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useAuditStream } from "./use-audit-stream"

type Listener = (event: Event) => void

class FakeEventSource {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static instances: FakeEventSource[] = []
  readyState = FakeEventSource.CONNECTING
  closed = false
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
    this.readyState = FakeEventSource.CLOSED
  }

  send(name: string, data: unknown, lastEventId = "") {
    this.readyState = FakeEventSource.OPEN
    this.dispatch(
      name,
      new MessageEvent(name, { data: JSON.stringify(data), lastEventId })
    )
  }

  drop(readyState: number) {
    this.readyState = readyState
    this.dispatch("error", new Event("error"))
  }

  private dispatch(name: string, event: Event) {
    for (const listener of this.listeners.get(name) ?? []) listener(event)
  }
}

const fetchMock = vi.fn()

function latest(): FakeEventSource {
  return FakeEventSource.instances[FakeEventSource.instances.length - 1]
}

function mount() {
  const handlers = { onEvent: vi.fn(), onGap: vi.fn(), onStop: vi.fn() }
  const hook = renderHook(() =>
    useAuditStream({ q: "outcome:denied", enabled: true, ...handlers })
  )
  return { ...handlers, hook }
}

function refused(status: number, code: string) {
  return new Response(JSON.stringify({ error: "refused", code }), {
    status,
    headers: { "content-type": "application/json" },
  })
}

describe("useAuditStream", () => {
  beforeEach(() => {
    FakeEventSource.instances = []
    fetchMock.mockReset()
    vi.stubGlobal("EventSource", FakeEventSource)
    vi.stubGlobal("fetch", fetchMock)
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it("follows the query and hands over each event", () => {
    const { onEvent, hook } = mount()
    expect(latest().url).toBe("/api/v1/audit/stream?q=outcome%3Adenied")
    act(() => latest().send("ready", {}))
    expect(hook.result.current.connected).toBe(true)
    act(() =>
      latest().send(
        "audit",
        { id: "e1", occurred_at: "2026-11-21T06:00:00Z" },
        "x-1-2"
      )
    )
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ id: "e1" }))
  })

  it("leaves a dropped connection to the browser, which resumes it", () => {
    const { onGap } = mount()
    act(() => latest().send("ready", {}))
    act(() => latest().drop(FakeEventSource.CONNECTING))
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(fetchMock).not.toHaveBeenCalled()
    act(() => latest().send("ready", {}))
    expect(onGap).not.toHaveBeenCalled()
  })

  it("turns itself off on a forbidden frame", () => {
    const { onStop } = mount()
    act(() => latest().send("forbidden", { code: "forbidden_role" }))
    expect(onStop).toHaveBeenCalledWith(
      "Live stopped: only Admins can follow the audit log."
    )
    expect(latest().closed).toBe(true)
  })

  it("turns itself off when the query is too expensive to follow", () => {
    const { onStop } = mount()
    act(() =>
      latest().send("error", { error: "slow", code: "query_too_expensive" })
    )
    expect(onStop).toHaveBeenCalledWith(
      expect.stringContaining("too slow to follow")
    )
  })

  it("explains a refused connection and turns itself off on 429", async () => {
    fetchMock.mockResolvedValue(refused(429, "too_many_streams"))
    const { onStop } = mount()
    act(() =>
      latest().send(
        "audit",
        { id: "e1", occurred_at: "2026-11-21T06:00:00Z" },
        "x-1-2"
      )
    )
    await act(async () => latest().drop(FakeEventSource.CLOSED))
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).toMatchObject({ "last-event-id": "x-1-2" })
    expect(onStop).toHaveBeenCalledWith(
      expect.stringContaining("already follow 5 audit streams")
    )
    expect(FakeEventSource.instances).toHaveLength(1)
  })

  it("starts over and reports a gap when the resume is too old", async () => {
    fetchMock.mockResolvedValue(refused(400, "invalid_request"))
    const { onGap, onStop } = mount()
    await act(async () => latest().drop(FakeEventSource.CLOSED))
    await act(async () => vi.runOnlyPendingTimersAsync())
    expect(FakeEventSource.instances).toHaveLength(2)
    act(() => latest().send("ready", {}))
    expect(onGap).toHaveBeenCalledTimes(1)
    expect(onStop).not.toHaveBeenCalled()
  })

  it("closes the stream when it unmounts", () => {
    const { hook } = mount()
    hook.unmount()
    expect(latest().closed).toBe(true)
  })
})
