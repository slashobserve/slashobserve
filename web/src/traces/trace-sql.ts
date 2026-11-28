import type { ExploreParams } from "@/api/keys"
import type { WireFrame } from "@/dashboards/queries"
import { defaultPageSize } from "@/lib/paging"
import { zonedDateTimeMs } from "@/lib/time"

import { normaliseTraceId, searchRange, type TraceSearch } from "./search"
import { clickhouseTraceSearch, traceForm } from "./search-query"
import { durationText } from "./trace-rows"

const traceIdName = /^trace_?id$/i
const durationName = /^duration$/i

export type ReadableCell = { text: string; raw: string }

export function readableCell(
  name: string,
  type: string,
  value: unknown
): ReadableCell | null {
  if (typeof value !== "number" || !Number.isFinite(value)) return null
  if (type === "time") {
    return { text: zonedDateTimeMs(value, undefined), raw: `${value} ms` }
  }
  if (type === "number" && durationName.test(name)) {
    return { text: durationText(value / 1e6), raw: `${value} ns` }
  }
  return null
}

export function traceIdField(frame: WireFrame): string | undefined {
  return frame.fields.find((field) => traceIdName.test(field.name))?.name
}

export function traceIdOf(value: unknown): string | null {
  return typeof value === "string" ? normaliseTraceId(value) : null
}

export function traceSqlStart(search: TraceSearch): string {
  return search.q ?? clickhouseTraceSearch(traceForm(search))
}

export function traceSqlSearch(query: string): Partial<TraceSearch> {
  return {
    q: query,
    service: undefined,
    span: undefined,
    min: undefined,
    max: undefined,
    errors: undefined,
    sort: undefined,
    ask: undefined,
  }
}

export function traceSqlParams(
  query: string,
  search: TraceSearch
): ExploreParams {
  return {
    query,
    kind: "instant",
    ...searchRange(search),
    pageSize: search.size ?? defaultPageSize,
    page: search.page ?? 1,
    asWritten: true,
  }
}

export function frameRows(frame: WireFrame | undefined): number {
  if (frame === undefined) return 0
  return frame.fields.reduce(
    (most, field) => Math.max(most, field.values.length),
    0
  )
}
