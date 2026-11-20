import type { ExploreParams } from "@/api/keys"
import type { WireFrame } from "@/dashboards/queries"
import { defaultPageSize } from "@/lib/paging"

import { normaliseTraceId, tracesTimeRange, type TraceSearch } from "./search"
import { clickhouseTraceSearch, traceForm } from "./search-query"

const traceIdName = /^trace_?id$/i

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
    ...tracesTimeRange(search),
    pageSize: search.size ?? defaultPageSize,
    page: search.page ?? 1,
  }
}

export function frameRows(frame: WireFrame | undefined): number {
  if (frame === undefined) return 0
  return frame.fields.reduce(
    (most, field) => Math.max(most, field.values.length),
    0
  )
}
