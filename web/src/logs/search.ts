import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { isTimeValue } from "@/lib/time"

export type LogsSearch = {
  ds?: string
  q?: string
  from?: string
  to?: string
}

const datasourcePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/
const maxQueryLength = 8000

export const logsDatasourceTypes = ["loki", "clickhouse"] as const

export function firstString(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  return typeof item === "string" ? item : undefined
}

export function validDatasource(value: unknown): string | undefined {
  const uid = firstString(value)
  return uid !== undefined && datasourcePattern.test(uid) ? uid : undefined
}

export function validQuery(value: unknown): string | undefined {
  const query = firstString(value)
  return query !== undefined &&
    query.length >= 1 &&
    query.length <= maxQueryLength &&
    !query.includes("\0")
    ? query
    : undefined
}

export function validRange(raw: Record<string, unknown>): {
  from: string | undefined
  to: string | undefined
} {
  const from = firstString(raw.from)
  const to = firstString(raw.to)
  return from !== undefined &&
    to !== undefined &&
    isTimeValue(from) &&
    isTimeValue(to)
    ? { from, to }
    : { from: undefined, to: undefined }
}

export function validateLogsSearch(raw: Record<string, unknown>): LogsSearch {
  return {
    ds: validDatasource(raw.ds),
    q: validQuery(raw.q),
    ...validRange(raw),
  }
}

export function logsTimeRange(search: LogsSearch): TimeRangeValue {
  return { from: search.from ?? "now-1h", to: search.to ?? "now" }
}
