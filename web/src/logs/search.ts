import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import {
  defaultPageSize,
  validPage,
  validPageSize,
  type PageSize,
} from "@/lib/paging"
import { isTimeValue } from "@/lib/time"

export type LogsSearch = {
  q?: string
  ask?: string
  from?: string
  to?: string
  f?: string
  cols?: string
  line?: string
  view?: "patterns"
  tail?: "1"
  observation?: string
  page?: number
  size?: PageSize
}

const pageResetKeys = ["q", "f", "from", "to", "size"] as const

export function searchAfter(
  state: LogsSearch,
  next: Partial<LogsSearch>
): LogsSearch {
  const reset = pageResetKeys.some((key) => key in next)
  return { ...state, ...(reset ? { page: undefined } : {}), ...next }
}

const datasourcePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/
const maxQueryLength = 8000
const maxAskLength = 2000
const controlCharacter = /\p{Cc}/u
const pillsPattern = /^[A-Za-z0-9_-]{1,4000}$/
const linePattern = /^\d{1,20}:[A-Za-z0-9_-]{1,64}$/
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const maxColumns = 20
const maxColumnLength = 256

export const logsDatasourceTypes = [
  "loki",
  "clickhouse",
  "elasticsearch",
  "opensearch",
] as const

export function isSearchFamily(type: string): boolean {
  return type === "elasticsearch" || type === "opensearch"
}

export function queryModeLabel(type: string): string {
  if (isSearchFamily(type)) return "KQL"
  if (type === "loki") return "LogQL"
  if (type === "clickhouse") return "SQL"
  return "Query"
}

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

export function validAsk(value: unknown): string | undefined {
  const ask = firstString(value)
  return ask !== undefined &&
    ask.length >= 1 &&
    ask.length <= maxAskLength &&
    !controlCharacter.test(ask)
    ? ask
    : undefined
}

function validPills(value: unknown): string | undefined {
  const pills = firstString(value)
  return pills !== undefined && pillsPattern.test(pills) ? pills : undefined
}

function validColumns(value: unknown): string | undefined {
  const columns = firstString(value)
  if (columns === undefined) return undefined
  const names = columns.split(",")
  return names.length <= maxColumns &&
    names.every((name) => name.length >= 1 && name.length <= maxColumnLength)
    ? columns
    : undefined
}

function validLine(value: unknown): string | undefined {
  const line = firstString(value)
  return line !== undefined && linePattern.test(line) ? line : undefined
}

function validObservation(value: unknown): string | undefined {
  const id = firstString(value)
  return id !== undefined && uuidPattern.test(id) ? id : undefined
}

function unlessStandard<T>(value: T | undefined, standard: T): T | undefined {
  return value === standard ? undefined : value
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
    q: validQuery(raw.q),
    ask: validAsk(raw.ask),
    ...validRange(raw),
    f: validPills(raw.f),
    cols: validColumns(raw.cols),
    line: validLine(raw.line),
    view: firstString(raw.view) === "patterns" ? "patterns" : undefined,
    tail: firstString(raw.tail) === "1" ? "1" : undefined,
    observation: validObservation(raw.observation),
    page: unlessStandard(validPage(raw.page), 1),
    size: unlessStandard(validPageSize(raw.size), defaultPageSize),
  }
}

export type LogsEntrySearch = LogsSearch & { ds?: string }

export function validateLogsEntrySearch(
  raw: Record<string, unknown>
): LogsEntrySearch {
  return { ...validateLogsSearch(raw), ds: validDatasource(raw.ds) }
}

export function logsTimeRange(search: LogsSearch): TimeRangeValue {
  return { from: search.from ?? "now-1h", to: search.to ?? "now" }
}
