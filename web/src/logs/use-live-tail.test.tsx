import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import { rowCount } from "./frames"
import { useLiveTail } from "./use-live-tail"

type Listener = (event: Event) => void

class FakeEventSource {
  static instances: FakeEventSource[] = []
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
  }

  send(name: string, data: unknown) {
    this.dispatch(name, new MessageEvent(name, { data: JSON.stringify(data) }))
  }

  connectionLost() {
    this.dispatch("error", new Event("error"))
  }

  private dispatch(name: string, event: Event) {
    for (const listener of this.listeners.get(name) ?? []) listener(event)
  }
}

function lines(ns: string[], newest: string) {
  const frame: WireFrame = {
    ref: "A",
    labels: { service_name: "frontend" },
    fields: [
      { name: "Time", type: "time", values: ns.map(() => Date.now()) },
      { name: "TimeNs", type: "string", values: ns },
      { name: "Body", type: "string", values: ns.map((n) => `line ${n}`) },
    ],
  }
  return { frames: [frame], newest, truncated: false }
}

function mount(running = true, seed: WireFrame[] = []) {
  return renderHook(
    (props: { running: boolean }) =>
      useLiveTail({
        uid: "loki",
        query: '{service_name="frontend"} |= "error"',
        seed,
        running: props.running,
        session: 1,
      }),
    { initialProps: { running } }
  )
}

function latestSource(): FakeEventSource {
  return FakeEventSource.instances[FakeEventSource.instances.length - 1]
}

describe("useLiveTail", () => {
  beforeEach(() => {
    FakeEventSource.instances = []
    vi.stubGlobal("EventSource", FakeEventSource)
    vi.useFakeTimers({ now: new Date("2026-11-07T12:00:30Z") })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it("opens one stream on the query and starts 10 seconds back with no seed", () => {
    mount()
    expect(FakeEventSource.instances).toHaveLength(1)
    const url = new URL(latestSource().url, "http://localhost")
    expect(url.pathname).toBe("/api/v1/datasources/loki/tail")
    expect(url.searchParams.get("query")).toBe(
      '{service_name="frontend"} |= "error"'
    )
    expect(url.searchParams.get("from")).toBe("2026-11-07T12:00:20.000Z")
  })

  it("does not open a stream while the tail is not running", () => {
    mount(false)
    expect(FakeEventSource.instances).toHaveLength(0)
  })

  it("waits until ready, then merges every lines event and counts what it received", () => {
    const { result } = mount()
    expect(result.current.isFetching).toBe(true)
    act(() => latestSource().send("ready", { from: "now-10s" }))
    expect(result.current.isFetching).toBe(false)
    act(() => {
      latestSource().send(
        "lines",
        lines(["1000000001", "1000000002"], "2026-11-07T12:00:29.000000002Z")
      )
      latestSource().send(
        "lines",
        lines(["1000000002", "1000000003"], "2026-11-07T12:00:30.000000003Z")
      )
    })
    expect(rowCount(result.current.buffer.frames)).toBe(3)
    expect(result.current.buffer.received).toBe(3)
    expect(FakeEventSource.instances).toHaveLength(1)
  })

  it("stops with the server's reason when the stream sends an error event", () => {
    const { result } = mount()
    act(() => latestSource().send("ready", { from: "now-10s" }))
    act(() =>
      latestSource().send("error", {
        code: "query_invalid",
        error: "The query is not a log query.",
      })
    )
    expect(result.current.error?.message).toBe("The query is not a log query.")
    expect(result.current.isFetching).toBe(false)
    expect(latestSource().closed).toBe(true)
  })

  it("reopens after a lost connection from the newest line it received", () => {
    const { result } = mount()
    const first = latestSource()
    act(() => first.send("ready", { from: "now-10s" }))
    act(() =>
      first.send(
        "lines",
        lines(["1000000001"], "2026-11-07T12:00:29.000000001Z")
      )
    )
    act(() => first.connectionLost())
    expect(first.closed).toBe(true)
    expect(result.current.isFetching).toBe(true)
    expect(FakeEventSource.instances).toHaveLength(1)
    act(() => {
      vi.advanceTimersByTime(1_000)
    })
    expect(FakeEventSource.instances).toHaveLength(2)
    expect(
      new URL(latestSource().url, "http://localhost").searchParams.get("from")
    ).toBe("2026-11-07T12:00:29.000000001Z")
    expect(rowCount(result.current.buffer.frames)).toBe(1)
  })

  it("reads the server's JSON reason when the stream cannot open", async () => {
    vi.useRealTimers()
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "too_many_streams",
          error: "You already hold 5 tails.",
        }),
        { status: 429, headers: { "content-type": "application/json" } }
      )
    )
    vi.stubGlobal("fetch", fetchMock)
    const { result } = mount()
    act(() => latestSource().connectionLost())
    await vi.waitFor(() => expect(result.current.error).not.toBeNull())
    expect(result.current.error?.message).toBe(
      "You have 5 live tails open. Stop one to start another."
    )
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(FakeEventSource.instances).toHaveLength(1)
  })

  it("closes the stream when the tail stops", () => {
    const { result, rerender } = mount()
    const source = latestSource()
    act(() => source.send("ready", { from: "now-10s" }))
    rerender({ running: false })
    expect(source.closed).toBe(true)
    expect(result.current.isFetching).toBe(false)
  })
})
