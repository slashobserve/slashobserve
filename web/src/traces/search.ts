import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import {
  firstString,
  validAsk,
  validDatasource,
  validQuery,
  validRange,
} from "@/logs/search"

import { parseDuration } from "./search-query"

export type TraceSearch = {
  ds?: string
  service?: string
  span?: string
  min?: string
  max?: string
  errors?: "1"
  q?: string
  ask?: string
  from?: string
  to?: string
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
}

const traceIdPattern = /^(?:[0-9a-f]{16}|[0-9a-f]{32})$/
const spanIdPattern = /^[0-9a-f]{16}$/

export const traceDatasourceTypes = ["tempo", "clickhouse"] as const

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

export function validateTraceSearch(raw: Record<string, unknown>): TraceSearch {
  return {
    ds: validDatasource(raw.ds),
    service: validName(raw.service),
    span: validName(raw.span),
    min: validDuration(raw.min),
    max: validDuration(raw.max),
    errors: firstString(raw.errors) === "1" ? "1" : undefined,
    q: validQuery(raw.q),
    ask: validAsk(raw.ask),
    ...validRange(raw),
  }
}

export function validateTraceViewSearch(
  raw: Record<string, unknown>
): TraceViewSearch {
  return {
    ds: validDatasource(raw.ds),
    span: validSpanId(raw.span),
    ...validRange(raw),
  }
}

export function tracesTimeRange(search: TraceSearch): TimeRangeValue {
  return { from: search.from ?? "now-1h", to: search.to ?? "now" }
}

export function traceViewRange(search: TraceViewSearch): TimeRangeValue {
  return { from: search.from ?? "now-24h", to: search.to ?? "now" }
}

export function normaliseTraceId(param: string): string | null {
  const id = param.toLowerCase()
  return traceIdPattern.test(id) ? id : null
}
