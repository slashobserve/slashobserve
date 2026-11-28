import { closeWhileCached } from "./page-cache"

export type SseFrame = { event: string; data: string; id: string }

const lineBreak = /\r\n|\r|\n/

export class SseParser {
  lastEventId: string
  retryMs: number | null = null
  private idBuffer: string
  private buffer = ""
  private pendingCr = false
  private data: string[] = []
  private event = ""

  constructor(lastEventId = "") {
    this.lastEventId = lastEventId
    this.idBuffer = lastEventId
  }

  push(text: string): SseFrame[] {
    let input = text
    if (this.pendingCr) {
      this.pendingCr = false
      if (input.startsWith("\n")) input = input.slice(1)
    }
    this.buffer += input
    const frames: SseFrame[] = []
    for (;;) {
      const match = lineBreak.exec(this.buffer)
      if (match === null) break
      const line = this.buffer.slice(0, match.index)
      const rest = this.buffer.slice(match.index + match[0].length)
      if (match[0] === "\r" && rest === "") this.pendingCr = true
      this.buffer = rest
      this.line(line, frames)
    }
    return frames
  }

  private line(line: string, frames: SseFrame[]) {
    if (line === "") {
      this.lastEventId = this.idBuffer
      if (this.data.length > 0) {
        frames.push({
          event: this.event === "" ? "message" : this.event,
          data: this.data.join("\n"),
          id: this.lastEventId,
        })
      }
      this.data = []
      this.event = ""
      return
    }
    if (line.startsWith(":")) return
    const colon = line.indexOf(":")
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? "" : line.slice(colon + 1)
    if (value.startsWith(" ")) value = value.slice(1)
    switch (field) {
      case "data":
        this.data.push(value)
        break
      case "event":
        this.event = value
        break
      case "id":
        if (!value.includes("\0")) this.idBuffer = value
        break
      case "retry":
        if (/^\d+$/.test(value)) this.retryMs = Number(value)
        break
    }
  }
}

type SseEnd =
  | { kind: "closed" }
  | { kind: "dropped" }
  | { kind: "aborted" }
  | { kind: "not-a-stream"; contentType: string }
  | { kind: "refused"; response: Response; body: unknown }

export type SseRead = {
  end: SseEnd
  lastEventId: string
  retryMs: number | null
}

const minRetryMs = 1_000

function mediaType(response: Response): string {
  const [type = ""] = (response.headers.get("content-type") ?? "").split(";")
  return type.trim().toLowerCase()
}

type ReadOptions = {
  lastEventId: string
  signal: AbortSignal
  onFrame: (frame: SseFrame) => void
}

export async function readSse(
  url: string,
  options: ReadOptions
): Promise<SseRead> {
  const connection = new AbortController()
  const drop = () => connection.abort()
  if (options.signal.aborted) drop()
  options.signal.addEventListener("abort", drop)
  const stopWatchingCache = closeWhileCached(drop)
  try {
    return await readUntilEnd(url, options, connection.signal)
  } finally {
    stopWatchingCache()
    options.signal.removeEventListener("abort", drop)
  }
}

async function readUntilEnd(
  url: string,
  options: ReadOptions,
  connection: AbortSignal
): Promise<SseRead> {
  const parser = new SseParser(options.lastEventId)
  const finish = (end: SseEnd): SseRead => ({
    end: options.signal.aborted ? { kind: "aborted" } : end,
    lastEventId: parser.lastEventId,
    retryMs:
      parser.retryMs === null ? null : Math.max(minRetryMs, parser.retryMs),
  })
  const headers: Record<string, string> = { accept: "text/event-stream" }
  if (options.lastEventId !== "") headers["last-event-id"] = options.lastEventId
  let response: Response
  try {
    response = await fetch(url, {
      headers,
      credentials: "same-origin",
      cache: "no-store",
      signal: connection,
    })
  } catch {
    return finish({ kind: "dropped" })
  }
  if (!response.ok || response.body === null) {
    const body: unknown = await response.json().catch(() => null)
    return finish({ kind: "refused", response, body })
  }
  const contentType = mediaType(response)
  if (contentType !== "text/event-stream") {
    response.body.cancel().catch(() => {})
    return finish({ kind: "not-a-stream", contentType })
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      for (const frame of parser.push(
        decoder.decode(value, { stream: true })
      )) {
        if (options.signal.aborted) break
        options.onFrame(frame)
      }
      if (options.signal.aborted) break
    }
  } catch {
    return finish({ kind: "dropped" })
  } finally {
    reader.cancel().catch(() => {})
  }
  return finish({ kind: "closed" })
}
