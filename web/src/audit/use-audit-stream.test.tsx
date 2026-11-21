import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { FakeStream, lastEventIdOf, refusedStream } from "./stream-fake"
import { useAuditStream } from "./use-audit-stream"

vi.mock("@/dashboards/live/reconnect", () => ({ backoffDelay: () => 0 }))

const fetchMock = vi.fn()

function call(index: number): { url: string; init: RequestInit } {
  const [url, init] = fetchMock.mock.calls[index] as [string, RequestInit]
  return { url, init }
}

function mount() {
  const handlers = { onEvent: vi.fn(), onRestart: vi.fn(), onStop: vi.fn() }
  const hook = renderHook(() =>
    useAuditStream({ q: "outcome:denied", enabled: true, ...handlers })
  )
  return { ...handlers, hook }
}

const event = { id: "e1", occurred_at: "2026-11-21T06:00:00Z" }

describe("useAuditStream", () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal("fetch", fetchMock)
  })

  afterEach(() => vi.unstubAllGlobals())

  it("follows the query and hands over each event", async () => {
    const stream = new FakeStream()
    fetchMock.mockResolvedValueOnce(stream.response)
    const { onEvent, hook } = mount()
    expect(call(0).url).toBe("/api/v1/audit/stream?q=outcome%3Adenied")
    expect(lastEventIdOf(call(0).init)).toBe(null)
    act(() => stream.frame("ready", {}))
    await waitFor(() => expect(hook.result.current.connected).toBe(true))
    act(() => stream.frame("audit", event, "x-1"))
    await waitFor(() =>
      expect(onEvent).toHaveBeenCalledWith(
        expect.objectContaining({ id: "e1" })
      )
    )
  })

  it("resumes a dropped connection from the last id it handled", async () => {
    const first = new FakeStream()
    const second = new FakeStream()
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response)
    const { onEvent, onRestart } = mount()
    act(() => {
      first.frame("ready", {})
      first.frame("audit", event, "x-2")
    })
    await waitFor(() => expect(onEvent).toHaveBeenCalled())
    act(() => first.drop())
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(lastEventIdOf(call(1).init)).toBe("x-2")
    act(() => second.frame("ready", {}))
    expect(onRestart).not.toHaveBeenCalled()
  })

  it("reconnects after the server ends the stream, waiting at least a second", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] })
    try {
      const first = new FakeStream()
      fetchMock
        .mockResolvedValueOnce(first.response)
        .mockResolvedValueOnce(new FakeStream().response)
      mount()
      act(() => {
        first.write("retry: 5\n\n")
        first.frame("audit", event, "x-3")
        first.end()
      })
      await act(() => vi.advanceTimersByTimeAsync(999))
      expect(fetchMock).toHaveBeenCalledTimes(1)
      await act(() => vi.advanceTimersByTimeAsync(1))
      expect(fetchMock).toHaveBeenCalledTimes(2)
      expect(lastEventIdOf(call(1).init)).toBe("x-3")
    } finally {
      vi.useRealTimers()
    }
  })

  it("turns itself off when the answer is not an event stream", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response("<html></html>", {
        headers: { "content-type": "text/html" },
      })
    )
    const { onStop } = mount()
    await waitFor(() =>
      expect(onStop).toHaveBeenCalledWith(
        "Live stopped: the server answered with text/html instead of an event stream."
      )
    )
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("turns itself off on a forbidden frame", async () => {
    const stream = new FakeStream()
    fetchMock.mockResolvedValueOnce(stream.response)
    const { onStop } = mount()
    act(() => stream.frame("forbidden", { code: "forbidden_role" }))
    await waitFor(() =>
      expect(onStop).toHaveBeenCalledWith(
        "Live stopped: only Admins can follow the audit log."
      )
    )
    expect(call(0).init.signal?.aborted).toBe(true)
  })

  it("turns itself off when the query is too expensive to follow", async () => {
    const stream = new FakeStream()
    fetchMock.mockResolvedValueOnce(stream.response)
    const { onStop } = mount()
    act(() =>
      stream.frame("error", { error: "slow", code: "query_too_expensive" })
    )
    await waitFor(() =>
      expect(onStop).toHaveBeenCalledWith(
        expect.stringContaining("too slow to follow")
      )
    )
  })

  it.each([
    [429, "too_many_streams", /already follow 5 audit streams/],
    [403, "forbidden_role", /only Admins can follow/],
    [422, "query_invalid", /Live stopped: refused query_invalid/],
  ])(
    "turns itself off on %i with its own message",
    async (status, code, text) => {
      fetchMock.mockResolvedValueOnce(refusedStream(status, code))
      const { onStop } = mount()
      await waitFor(() =>
        expect(onStop).toHaveBeenCalledWith(expect.stringMatching(text))
      )
      expect(fetchMock).toHaveBeenCalledTimes(1)
    }
  )

  it("starts over without the id when the resume is too old, then reports it", async () => {
    const first = new FakeStream()
    const fresh = new FakeStream()
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(refusedStream(400, "invalid_request"))
      .mockResolvedValueOnce(fresh.response)
    const { onEvent, onRestart, onStop } = mount()
    act(() => first.frame("audit", event, "x-4"))
    await waitFor(() => expect(onEvent).toHaveBeenCalled())
    act(() => first.drop())
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3))
    expect(lastEventIdOf(call(1).init)).toBe("x-4")
    expect(lastEventIdOf(call(2).init)).toBe(null)
    expect(onRestart).not.toHaveBeenCalled()
    act(() => fresh.frame("ready", {}))
    await waitFor(() => expect(onRestart).toHaveBeenCalledTimes(1))
    expect(onStop).not.toHaveBeenCalled()
  })

  it("retries a server that is shutting down with the same id", async () => {
    const first = new FakeStream()
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(refusedStream(503, "shutting_down"))
      .mockResolvedValueOnce(new FakeStream().response)
    const { onEvent, onStop } = mount()
    act(() => first.frame("audit", event, "x-5"))
    await waitFor(() => expect(onEvent).toHaveBeenCalled())
    act(() => first.drop())
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3))
    expect(lastEventIdOf(call(2).init)).toBe("x-5")
    expect(onStop).not.toHaveBeenCalled()
  })

  it("aborts the request when it unmounts", () => {
    fetchMock.mockResolvedValueOnce(new FakeStream().response)
    const { hook } = mount()
    hook.unmount()
    expect(call(0).init.signal?.aborted).toBe(true)
  })
})
