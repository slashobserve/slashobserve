import type { ExploreParams } from "@/api/keys"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { matcherInsert } from "@/panel-editor/insert-text"

import { tracesTimeRange, type TraceSearch } from "./search"

const durationPattern = /^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/

const nanosecondsPer: Record<string, number> = {
  ns: 1,
  us: 1e3,
  µs: 1e3,
  ms: 1e6,
  s: 1e9,
  m: 60e9,
  h: 3600e9,
}

export function parseDuration(text: string): number | null {
  const match = durationPattern.exec(text)
  if (match === null) return null
  return Math.round(Number(match[1]) * nanosecondsPer[match[2]])
}

export type TraceForm = {
  service: string
  span: string
  min: string
  max: string
  errors: boolean
}

function traceFormOf(search: TraceSearch): TraceForm {
  return {
    service: search.service ?? "",
    span: search.span ?? "",
    min: search.min ?? "",
    max: search.max ?? "",
    errors: search.errors === "1",
  }
}

function quoted(text: string): string {
  return `"${text.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`
}

function traceQLDuration(text: string): string {
  return text.replace("µs", "us")
}

export function traceQLFor(form: TraceForm): string {
  const conditions = [
    form.service && `resource.service.name = ${quoted(form.service)}`,
    form.span && `name = ${quoted(form.span)}`,
    form.min && `duration >= ${traceQLDuration(form.min)}`,
    form.max && `duration <= ${traceQLDuration(form.max)}`,
    form.errors && "status = error",
  ].filter(Boolean)
  return `{${conditions.join(" && ")}}`
}

function clickhouseMatch(column: string, value: string): string {
  return ` AND ${matcherInsert("clickhouse", column, value)}`
}

export function clickhouseTraceSearch(form: TraceForm): string {
  const min = form.min ? parseDuration(form.min) : null
  const max = form.max ? parseDuration(form.max) : null
  const conditions = [
    form.service ? clickhouseMatch("ServiceName", form.service) : "",
    form.span ? clickhouseMatch("SpanName", form.span) : "",
    min !== null ? ` AND Duration >= ${min}` : "",
    max !== null ? ` AND Duration <= ${max}` : "",
    form.errors
      ? " AND lower(StatusCode) IN ('error', 'status_code_error')"
      : "",
  ].join("")
  return `SELECT TraceId, min(Timestamp) AS StartTime, argMax(ServiceName, Duration) AS ServiceName, argMax(SpanName, Duration) AS SpanName, max(Duration) AS Duration, count() AS Spans
FROM otel_traces
WHERE $__timeFilter(Timestamp)${conditions}
GROUP BY TraceId
ORDER BY StartTime DESC
LIMIT 100`
}

export const traceSearchRows = 100

export function traceSearchParams(
  type: string,
  search: TraceSearch
): ExploreParams {
  const form = traceFormOf(search)
  return {
    query:
      type === "tempo"
        ? (search.q ?? traceQLFor(form))
        : clickhouseTraceSearch(form),
    kind: "instant",
    ...tracesTimeRange(search),
    limit: traceSearchRows,
  }
}

export function traceByIdParams(
  type: string,
  traceId: string,
  range: TimeRangeValue
): ExploreParams {
  const query =
    type === "tempo"
      ? traceId
      : `SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp, Duration, StatusCode, SpanAttributes, ResourceAttributes
FROM otel_traces
WHERE $__timeFilter(Timestamp) AND TraceId = '${traceId}'
ORDER BY Timestamp
LIMIT 1000`
  return { query, kind: "instant", ...range, limit: 1000 }
}
