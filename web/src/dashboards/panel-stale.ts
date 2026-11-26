import { errorMessage } from "@/panels/panel-error"
import { displayZone, zonedDateTime } from "@/lib/time"

const clockStart = 11
const clockEnd = 16

export function staleNotice(
  dataUpdatedAt: number,
  error: unknown,
  zone: string | undefined
): string {
  const clock = zonedDateTime(dataUpdatedAt, zone).slice(clockStart, clockEnd)
  return `Showing data from ${clock}; the last refresh failed: ${errorMessage(error)}`
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
