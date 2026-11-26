import { errorMessage } from "@/panels/panel-error"
import { displayZone, zonedDateTime } from "@/lib/time"

const dateLength = 10
const clockStart = 11
const clockEnd = 16

function shownSince(
  dataUpdatedAt: number,
  zone: string | undefined,
  now: number
): string {
  const at = zonedDateTime(dataUpdatedAt, zone)
  const today = zonedDateTime(now, zone)
  const clock = at.slice(clockStart, clockEnd)
  return at.slice(0, dateLength) === today.slice(0, dateLength)
    ? clock
    : `${at.slice(0, dateLength)} ${clock}`
}

export function staleNotice(
  dataUpdatedAt: number,
  error: unknown,
  zone: string | undefined,
  now = Date.now()
): string {
  const reason = `the last refresh failed: ${errorMessage(error)}`
  if (dataUpdatedAt <= 0) return `Showing earlier data; ${reason}`
  return `Showing data from ${shownSince(dataUpdatedAt, zone, now)}; ${reason}`
}

export type FailingQuery = {
  data: unknown
  isError: boolean
  failureCount: number
  error: unknown
  failureReason: unknown
  dataUpdatedAt: number
}

export function staleNoticeOf(
  query: FailingQuery,
  panelZone: string | undefined,
  dashboardZone: string | undefined
): string | undefined {
  if (!query.isError || query.data === undefined) return undefined
  return staleNotice(
    query.dataUpdatedAt,
    query.error,
    displayZone(panelZone, dashboardZone)
  )
}

export function failedWithoutData(query: FailingQuery): boolean {
  return query.data === undefined && (query.isError || query.failureCount > 0)
}

export function failureOf(query: FailingQuery): unknown {
  return query.error ?? query.failureReason
}
