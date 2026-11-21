import type { components } from "@/api/schema.gen"

export type AuditEvent = components["schemas"]["AuditEvent"]

const streamPath = "/api/v1/audit/stream"
const streamsPerCaller = 5
export const liveRowLimit = 500

export function auditStreamUrl(q: string): string {
  return q.trim() === ""
    ? streamPath
    : `${streamPath}?${new URLSearchParams({ q })}`
}

function record(data: string): Record<string, unknown> | null {
  let raw: unknown
  try {
    raw = JSON.parse(data)
  } catch {
    return null
  }
  return typeof raw === "object" && raw !== null && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : null
}

export function parseAuditFrame(data: string): AuditEvent | null {
  const raw = record(data)
  return raw !== null &&
    typeof raw.id === "string" &&
    typeof raw.occurred_at === "string"
    ? (raw as AuditEvent)
    : null
}

export const forbiddenText =
  "Live stopped: only Admins can follow the audit log."
const tooManyText = `Live stopped: you already follow ${streamsPerCaller} audit streams in other tabs or tools. Close one and turn Live on again.`
const tooExpensiveText =
  "Live stopped: this query is too slow to follow. Narrow it and turn Live on again."

export function stopFrameText(name: "forbidden" | "error", data: string) {
  if (name === "forbidden") return forbiddenText
  const raw = record(data)
  if (raw?.code === "query_too_expensive") return tooExpensiveText
  return typeof raw?.error === "string"
    ? `Live stopped: ${raw.error}`
    : "Live stopped: the stream reported an error."
}

export type StreamRefusal =
  { kind: "stop"; message: string } | { kind: "gap" } | { kind: "retry" }

export function streamRefusal(
  status: number,
  code: string,
  message: string
): StreamRefusal {
  switch (status) {
    case 400:
      return { kind: "gap" }
    case 401:
      return { kind: "stop", message: "Live stopped: sign in again." }
    case 403:
      return { kind: "stop", message: forbiddenText }
    case 422:
      return { kind: "stop", message: `Live stopped: ${message}` }
    case 429:
      return code === "too_many_streams"
        ? { kind: "stop", message: tooManyText }
        : { kind: "retry" }
    default:
      return { kind: "retry" }
  }
}

export type LiveRows = { anchor: number; live: readonly AuditEvent[] }

export function withLiveEvent(
  current: LiveRows,
  event: AuditEvent,
  loaded: ReadonlySet<string>,
  now: number
): LiveRows {
  if (loaded.has(event.id) || current.live.some((row) => row.id === event.id)) {
    return current
  }
  const live = [event, ...current.live]
  if (live.length <= liveRowLimit) return { anchor: current.anchor, live }
  return {
    anchor: now,
    live: live.filter((row) => Date.parse(row.occurred_at) >= now),
  }
}

export function notAStreamText(contentType: string): string {
  const answer = contentType === "" ? "no content type" : contentType
  return `Live stopped: the server answered with ${answer} instead of an event stream.`
}
