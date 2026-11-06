import type { ObserveRequest } from "@/observe/queries"

export function resultFinishedAt(
  request: Pick<ObserveRequest, "status" | "finished_at" | "build_saved_at">
): string | null {
  return request.status === "failed" && request.build_saved_at !== null
    ? request.build_saved_at
    : request.finished_at
}

export function showsWorkingRow(
  request: Pick<ObserveRequest, "status">,
  stalled: boolean
): boolean {
  return request.status !== "queued" && !stalled
}

export function turnDurationMs(
  startedAt: string | null,
  finishedAt: string | null,
  now: number
): number | null {
  if (startedAt === null) return null
  const start = Date.parse(startedAt)
  const end = finishedAt === null ? now : Date.parse(finishedAt)
  if (Number.isNaN(start) || Number.isNaN(end)) return null
  return Math.max(0, end - start)
}

export function formatRunDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

export function tookText(
  startedAt: string | null,
  finishedAt: string | null
): string | undefined {
  if (finishedAt === null) return undefined
  const ms = turnDurationMs(startedAt, finishedAt, 0)
  return ms === null ? undefined : formatRunDuration(ms)
}
