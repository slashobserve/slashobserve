import type { ApiError } from "@/api/errors"

const minBackoffMs = 1_000
const maxBackoffMs = 30_000
const checkEveryFailures = 3

export const reconnectingGraceMs = 2_000

export function backoffDelay(
  attempt: number,
  random: () => number = Math.random
): number {
  const base = Math.min(minBackoffMs * 2 ** attempt, maxBackoffMs)
  return base / 2 + random() * (base / 2)
}

export function shouldCheckDashboard(failures: number): boolean {
  return failures > 0 && failures % checkEveryFailures === 0
}

export type AfterClosed = "reopen" | "stop" | "retry"

function closeReason(data: string): unknown {
  try {
    return (JSON.parse(data) as { reason?: unknown } | null)?.reason
  } catch {
    return undefined
  }
}

export function afterClosed(data: string): AfterClosed {
  switch (closeReason(data)) {
    case "lifetime":
      return "reopen"
    case "access_lost":
      return "stop"
    default:
      return "retry"
  }
}

export type GoneOutcome = "deleted" | "reload" | "keep"

export function isGone(error: ApiError): boolean {
  return error.status === 404 || error.status === 403
}

export function goneOutcome(
  error: ApiError,
  announcedDeleted: boolean
): GoneOutcome {
  if (!isGone(error)) return "keep"
  return error.status === 404 && announcedDeleted ? "deleted" : "reload"
}
