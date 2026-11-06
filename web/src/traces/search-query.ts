import type { ExploreParams } from "@/api/keys"
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

const bareAttributeName = /^[\w.]+$/
const numberPattern = /^-?\d+(?:\.\d+)?$/

function traceQLValue(value: string): string {
  if (value === "true" || value === "false") return value
  if (numberPattern.test(value) && Number.isFinite(Number(value))) return value
  return quoted(value)
}

export function traceQLAttribute(
  scope: "span" | "resource",
  key: string,
  value: string,
  negate: boolean
): string {
  const name = bareAttributeName.test(key) ? key : quoted(key)
  return `{ ${scope}.${name} ${negate ? "!=" : "="} ${traceQLValue(value)} }`
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

const clickhouseErrorStatus =
  "lower(StatusCode) IN ('error', 'status_code_error')"

function clickhouseConditions(form: TraceForm, column: string): string[] {
  const min = form.min ? parseDuration(form.min) : null
  const max = form.max ? parseDuration(form.max) : null
  return [
    form.service
      ? matcherInsert("clickhouse", column + "ServiceName", form.service)
      : "",
    form.span
      ? matcherInsert("clickhouse", column + "SpanName", form.span)
      : "",
    min !== null ? `${column}Duration >= ${min}` : "",
    max !== null ? `${column}Duration <= ${max}` : "",
    form.errors ? clickhouseErrorStatus : "",
  ].filter((condition) => condition !== "")
}

export function clickhouseTraceSearch(form: TraceForm): string {
  const conditions = clickhouseConditions(form, "otel_traces.")
    .map((condition) => ` AND ${condition}`)
    .join("")
  return `SELECT TraceId, min(Timestamp) AS StartTime, argMax(otel_traces.ServiceName, otel_traces.Duration) AS ServiceName, argMax(otel_traces.SpanName, otel_traces.Duration) AS SpanName, max(otel_traces.Duration) AS Duration, count() AS Spans, countIf(${clickhouseErrorStatus}) AS Errors
FROM otel_traces
WHERE $__timeFilter(Timestamp)${conditions}
GROUP BY TraceId
ORDER BY StartTime DESC
LIMIT 100`
}

export function traceStatsFilter(type: string, search: TraceSearch): string {
  const form = traceFormOf(search)
  if (type === "tempo") return search.q ?? traceQLFor(form)
  return clickhouseConditions(form, "").join(" AND ")
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
