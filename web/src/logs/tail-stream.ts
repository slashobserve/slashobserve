import type { WireFrame } from "@/dashboards/queries"

export type TailEvent =
  | { type: "ready"; from: string }
  | { type: "lines"; frames: WireFrame[]; newest: string; truncated: boolean }
  | { type: "error"; code: string; error: string }

const linesPerEvent = 500
const firstStartMs = 10_000

export function tailUrl(
  uid: string,
  query: string,
  from: string | null,
  timezone: string | undefined
): string {
  const params = new URLSearchParams({ query })
  if (from !== null) params.set("from", from)
  params.set("limit", String(linesPerEvent))
  if (timezone !== undefined) params.set("timezone", timezone)
  return `/api/v1/datasources/${encodeURIComponent(uid)}/tail?${params}`
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

export function parseTailEvent(name: string, data: string): TailEvent | null {
  const raw = record(data)
  if (raw === null) return null
  switch (name) {
    case "ready":
      return typeof raw.from === "string"
        ? { type: "ready", from: raw.from }
        : null
    case "lines":
      return Array.isArray(raw.frames) && typeof raw.newest === "string"
        ? {
            type: "lines",
            frames: raw.frames as WireFrame[],
            newest: raw.newest,
            truncated: raw.truncated === true,
          }
        : null
    case "error":
      return typeof raw.code === "string" && typeof raw.error === "string"
        ? { type: "error", code: raw.code, error: raw.error }
        : null
    default:
      return null
  }
}

export function tailStartFrom(newest: number | null, now: number): string {
  return new Date(newest ?? now - firstStartMs).toISOString()
}

const tailsPerPerson = 5

export function tailFailureText(
  status: number,
  code: string,
  message: string
): string {
  if (status === 429 && code === "too_many_streams") {
    return `You have ${tailsPerPerson} live tails open. Stop one to start another.`
  }
  return message
}
