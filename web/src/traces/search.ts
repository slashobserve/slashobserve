import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { resolveTime } from "@/lib/time"
import {
  firstString,
  validAsk,
  validDatasource,
  validObservation,
  validQuery,
  validRange,
  validSaved,
} from "@/logs/search"

import {
  defaultPageSize,
  validPage,
  validPageSize,
  type PageSize,
} from "@/lib/paging"

import { parseDuration } from "./search-query"
import { encodeSort, decodeSort, isSortText } from "./trace-rows"

export type TraceSearch = {
  service?: string
  span?: string
  min?: string
  max?: string
  errors?: "1"
  q?: string
  ask?: string
  from?: string
  to?: string
  observation?: string
  saved?: string
  page?: number
  size?: PageSize
  sort?: string
}

export type TraceFormSearch = Pick<
  TraceSearch,
  "service" | "span" | "min" | "max" | "errors" | "q"
>

export type TraceViewSearch = {
  ds?: string
  span?: string
  from?: string
  to?: string
  observation?: string
}

const traceIdPattern = /^(?:[0-9a-f]{16}|[0-9a-f]{32})$/
const spanIdPattern = /^[0-9a-f]{16}$/

export const traceDatasourceTypes = ["tempo", "jaeger", "clickhouse"] as const

export function hasQueryText(type: string): boolean {
  return type === "tempo" || type === "jaeger"
}

export function hasTraceStats(type: string): boolean {
  return type === "tempo" || type === "clickhouse"
}

const maxNameLength = 200
const controlCharacters = /\p{Cc}/u

function validName(value: unknown): string | undefined {
  const text = firstString(value)
  return text !== undefined &&
    text.length >= 1 &&
    text.length <= maxNameLength &&
    !controlCharacters.test(text)
    ? text
    : undefined
}

function validSpanId(value: unknown): string | undefined {
  const id = firstString(value)?.toLowerCase()
  return id !== undefined && spanIdPattern.test(id) ? id : undefined
}

function validDuration(value: unknown): string | undefined {
  const text = firstString(value)
  return text !== undefined && parseDuration(text) !== null ? text : undefined
}

function validSort(value: unknown): string | undefined {
  const text = firstString(value)
  if (text === undefined || !isSortText(text)) return undefined
  return encodeSort(decodeSort(text)) === text ? text : undefined
}

function unlessStandard<T>(value: T | undefined, standard: T): T | undefined {
  return value === standard ? undefined : value
}

export function validateTraceSearch(raw: Record<string, unknown>): TraceSearch {
  return {
    service: validName(raw.service),
    span: validName(raw.span),
    min: validDuration(raw.min),
    max: validDuration(raw.max),
    errors: firstString(raw.errors) === "1" ? "1" : undefined,
    q: validQuery(raw.q),
    ask: validAsk(raw.ask),
    ...validRange(raw),
    observation: validObservation(raw.observation),
    saved: validSaved(raw.saved),
    page: unlessStandard(validPage(raw.page), 1),
    size: unlessStandard(validPageSize(raw.size), defaultPageSize),
    sort: validSort(raw.sort),
  }
}

const formKeys = ["service", "span", "min", "max", "errors", "q"] as const

export function askAfterSubmit(
  search: TraceSearch,
  next: TraceFormSearch
): string | undefined {
  return formKeys.every((key) => next[key] === search[key])
    ? search.ask
    : undefined
}

export function changesQuery(next: Partial<TraceSearch>): boolean {
  return formKeys.some((key) => key in next)
}

export type TraceEntrySearch = TraceSearch & { ds?: string }

export function validateTraceEntrySearch(
  raw: Record<string, unknown>
): TraceEntrySearch {
  return { ...validateTraceSearch(raw), ds: validDatasource(raw.ds) }
}

export function validateTraceViewSearch(
  raw: Record<string, unknown>
): TraceViewSearch {
  return {
    ds: validDatasource(raw.ds),
    span: validSpanId(raw.span),
    ...validRange(raw),
    observation: validObservation(raw.observation),
  }
}

export function tracesTimeRange(search: TraceSearch): TimeRangeValue {
  return { from: search.from ?? "now-1h", to: search.to ?? "now" }
}

export function rangeInFuture(range: TimeRangeValue, now: number): boolean {
  return resolveTime(range.from, now, false) > now
}

export function linkWithoutPage(href: string): string {
  const url = new URL(href)
  url.searchParams.delete("page")
  return url.toString()
}

export function traceViewDatasource<Source extends { uid: string }>(
  eligible: readonly Source[],
  ds: string | undefined
): Source | undefined {
  if (ds !== undefined) return eligible.find((source) => source.uid === ds)
  return eligible.length === 1 ? eligible[0] : undefined
}

export function traceViewRange(search: TraceViewSearch): TimeRangeValue {
  return { from: search.from ?? "now-24h", to: search.to ?? "now" }
}

export function normaliseTraceId(param: string): string | null {
  const id = param.toLowerCase()
  return traceIdPattern.test(id) ? id : null
}
