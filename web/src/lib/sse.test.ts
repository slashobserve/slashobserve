import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import { readSse, SseParser } from "./sse"

describe("SseParser", () => {
  test("joins a frame split across chunks, even inside a CRLF", () => {
    const parser = new SseParser()
    expect(parser.push("id: x-1\r\nevent: au")).toEqual([])
    expect(parser.push('dit\r\ndata: {"id":')).toEqual([])
    expect(parser.push('"e1"}\r')).toEqual([])
    expect(parser.push("\n\r\n")).toEqual([
      { event: "audit", data: '{"id":"e1"}', id: "x-1" },
    ])
  })

  test("joins multi-line data with newlines", () => {
    const parser = new SseParser()
    expect(parser.push("data: one\ndata:two\ndata\n\n")).toEqual([
      { event: "message", data: "one\ntwo\n", id: "" },
    ])
  })

  test("skips comments and reads the retry delay", () => {
    const parser = new SseParser()
    expect(parser.push(": ping\n\nretry: 3000\nretry: soon\n\n")).toEqual([])
    expect(parser.retryMs).toBe(3000)
  })

  test("keeps the last id across frames and a frame without one", () => {
    const parser = new SseParser("x-0")
    const frames = parser.push(
      "event: ready\ndata: {}\n\nid: x-2\ndata: a\n\ndata: b\n\n"
    )
    expect(frames.map((frame) => frame.id)).toEqual(["x-0", "x-2", "x-2"])
    expect(parser.lastEventId).toBe("x-2")
  })

  test("applies an id only when its frame is dispatched", () => {
    const parser = new SseParser("x-1")
    expect(parser.push("id: x-2\nevent: audit\ndata: {}\n")).toEqual([])
    expect(parser.lastEventId).toBe("x-1")
    parser.push("\n")
    expect(parser.lastEventId).toBe("x-2")
  })

  test("applies the id of a frame with no data at its blank line", () => {
    const parser = new SseParser()
    expect(parser.push("id: x-3\n\n")).toEqual([])
    expect(parser.lastEventId).toBe("x-3")
  })

  test("drops a frame with no data", () => {
    expect(new SseParser().push("event: audit\n\n")).toEqual([])
  })
})

const encoder = new TextEncoder()

function streamed(chunks: string[], fail = false): Response {
  let index = 0
  return new Response(
    new ReadableStream<Uint8Array>({
      pull(controller) {
        if (index < chunks.length) {
          controller.enqueue(encoder.encode(chunks[index++]))
        } else if (fail) {
          controller.error(new TypeError("network error"))
        } else {
          controller.close()
        }
      },
    }),
    { headers: { "content-type": "text/event-stream" } }
  )
}

describe("readSse", () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal("fetch", fetchMock)
  })

  afterEach(() => vi.unstubAllGlobals())

  test("sends the last id and hands back the id it ended on", async () => {
    fetchMock.mockResolvedValue(
      streamed([
        "retry: 3000\nevent: ready\ndata: {}\n\nid: x-",
        "5\nevent: audit\ndata: {}\n\n",
      ])
    )
    const onFrame = vi.fn()
    const read = await readSse("/s", {
      lastEventId: "x-4",
      signal: new AbortController().signal,
      onFrame,
    })
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).toMatchObject({ "last-event-id": "x-4" })
    expect(onFrame.mock.calls.map(([frame]) => frame.event)).toEqual([
      "ready",
      "audit",
    ])
    expect(read).toEqual({
      end: { kind: "closed" },
      lastEventId: "x-5",
      retryMs: 3000,
    })
  })

  test("sends no id on a first connection", async () => {
    fetchMock.mockResolvedValue(streamed([]))
    await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame: vi.fn(),
    })
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).not.toHaveProperty("last-event-id")
  })

  test("waits at least a second whatever retry the server sends", async () => {
    fetchMock.mockResolvedValue(streamed(["retry: 5\n\n"]))
    const read = await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame: vi.fn(),
    })
    expect(read.retryMs).toBe(1000)
  })

  test("reports a 200 that is not an event stream without reading it", async () => {
    fetchMock.mockResolvedValue(
      new Response("data: {}\n\n", {
        headers: { "content-type": "text/html; charset=utf-8" },
      })
    )
    const onFrame = vi.fn()
    const read = await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame,
    })
    expect(read.end).toEqual({ kind: "not-a-stream", contentType: "text/html" })
    expect(onFrame).not.toHaveBeenCalled()
  })

  test("reports a refused status with its body", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ code: "too_many_streams" }), {
        status: 429,
      })
    )
    const read = await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame: vi.fn(),
    })
    expect(read.end).toMatchObject({
      kind: "refused",
      body: { code: "too_many_streams" },
    })
    expect(read.end.kind === "refused" && read.end.response.status).toBe(429)
  })

  test("reports a connection that drops, keeping the id it reached", async () => {
    fetchMock.mockResolvedValue(streamed(["id: x-7\ndata: {}\n\n"], true))
    const read = await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame: vi.fn(),
    })
    expect(read).toMatchObject({ end: { kind: "dropped" }, lastEventId: "x-7" })
  })

  test("resumes before a frame the connection cut in half", async () => {
    fetchMock.mockResolvedValue(
      streamed(["id: x-7\ndata: {}\n\nid: x-8\nevent: audit\ndata: {"], true)
    )
    const onFrame = vi.fn()
    const read = await readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame,
    })
    expect(onFrame).toHaveBeenCalledTimes(1)
    expect(read).toMatchObject({ end: { kind: "dropped" }, lastEventId: "x-7" })
  })

  test("stops handing frames over once aborted", async () => {
    fetchMock.mockResolvedValue(
      streamed(["data: a\n\ndata: b\n\n", "data: c\n\n"])
    )
    const controller = new AbortController()
    const onFrame = vi.fn(() => controller.abort())
    const read = await readSse("/s", {
      lastEventId: "",
      signal: controller.signal,
      onFrame,
    })
    expect(onFrame).toHaveBeenCalledTimes(1)
    expect(read.end).toEqual({ kind: "aborted" })
  })

  test("drops the connection when the page is hidden for the cache", async () => {
    fetchMock.mockImplementation((_url: string, init: RequestInit) => {
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(encoder.encode("event: ready\ndata: {}\n\n"))
          init.signal?.addEventListener("abort", () =>
            controller.error(new DOMException("aborted", "AbortError"))
          )
        },
      })
      return Promise.resolve(
        new Response(body, {
          headers: { "content-type": "text/event-stream" },
        })
      )
    })
    const onFrame = vi.fn()
    const reading = readSse("/s", {
      lastEventId: "",
      signal: new AbortController().signal,
      onFrame,
    })
    await vi.waitFor(() => expect(onFrame).toHaveBeenCalledTimes(1))
    window.dispatchEvent(new Event("pagehide"))
    expect((await reading).end).toEqual({ kind: "dropped" })
  })
})
