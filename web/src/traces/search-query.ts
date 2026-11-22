import type { ExploreParams } from "@/api/keys"
import { defaultPageSize } from "@/lib/paging"
import { matcherInsert } from "@/panel-editor/insert-text"

import type { Selection } from "./scatter"
import { hasQueryText, tracesTimeRange, type TraceSearch } from "./search"
import { decodeSort, pageSortParam } from "./trace-rows"

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

export function durationField(ms: number, round: "down" | "up"): string {
  const whole = round === "down" ? Math.floor : Math.ceil
  const tenths = whole(ms * 10)
  return tenths < 10_000 ? `${tenths / 10}ms` : `${whole(ms) / 1000}s`
}

export type TraceForm = {
  service: string
  span: string
  min: string
  max: string
  errors: boolean
}

export function traceForm(search: TraceSearch): TraceForm {
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

function traceQLValues(value: string): string[] {
  if (value === "true" || value === "false") return [value]
  if (numberPattern.test(value) && Number.isFinite(Number(value))) {
    return [value, quoted(value)]
  }
  return [quoted(value)]
}

function traceQLCondition(
  scope: "span" | "resource",
  key: string,
  value: string,
  negate: boolean
): string {
  const name = bareAttributeName.test(key) ? key : quoted(key)
  const comparisons = traceQLValues(value).map(
    (typed) => `${scope}.${name} ${negate ? "!=" : "="} ${typed}`
  )
  if (comparisons.length === 1) return comparisons[0]
  return `(${comparisons.join(negate ? " && " : " || ")})`
}

export function traceQLAttribute(
  scope: "span" | "resource",
  key: string,
  value: string,
  negate: boolean
): string {
  return `{ ${traceQLCondition(scope, key, value, negate)} }`
}

function withSpansetConditions(q: string, conditions: string[]): string | null {
  const open = q.indexOf("{")
  const close = open === -1 ? -1 : q.indexOf("}", open)
  if (close === -1) return null
  const inside = q.slice(open + 1, close)
  const existing = inside.trimEnd()
  const trailing = inside.slice(existing.length)
  const joined = conditions.join(" && ")
  const body =
    existing.trim() === "" ? joined : `${existing} && ${joined}${trailing}`
  return `${q.slice(0, open + 1)}${body}${q.slice(close)}`
}

export function withDurationBounds(
  q: string,
  min: string,
  max: string | undefined
): string | null {
  const conditions = [`duration >= ${goDuration(min)}`]
  if (max !== undefined) conditions.push(`duration <= ${goDuration(max)}`)
  return withSpansetConditions(q, conditions)
}

const intrinsicEnums: Record<string, true> = { kind: true, status: true }

function compareCondition(attribute: string, value: string): string | null {
  if (attribute === "name") return `name = ${quoted(value)}`
  if (intrinsicEnums[attribute] !== undefined) {
    return `${attribute} = ${value.toLowerCase()}`
  }
  const dot = attribute.indexOf(".")
  const scope = attribute.slice(0, dot)
  if (scope !== "span" && scope !== "resource") return null
  return traceQLCondition(scope, attribute.slice(dot + 1), value, false)
}

export function attributeFilter(
  type: string,
  search: TraceSearch,
  attribute: string,
  value: string
): Partial<TraceSearch> | null {
  if (type !== "tempo") {
    if (attribute === "name") return { span: value }
    if (attribute === "resource.service.name") return { service: value }
    return null
  }
  const condition = compareCondition(attribute, value)
  if (condition === null) return null
  const q = withSpansetConditions(search.q ?? traceQLFor(traceForm(search)), [
    condition,
  ])
  return q === null ? null : { q }
}

function tighterBound(
  current: string | undefined,
  candidate: string,
  currentWins: (currentNs: number, candidateNs: number) => boolean
): string {
  const currentNs = current === undefined ? null : parseDuration(current)
  const candidateNs = parseDuration(candidate)
  return current !== undefined &&
    currentNs !== null &&
    candidateNs !== null &&
    currentWins(currentNs, candidateNs)
    ? current
    : candidate
}

export function keepSelectionSearch(
  type: string,
  search: TraceSearch,
  selection: Selection
): Partial<TraceSearch> | null {
  const floor = durationField(selection.minMs, "down")
  const ceiling =
    selection.maxMs === null ? undefined : durationField(selection.maxMs, "up")
  const min = tighterBound(search.min, floor, (a, b) => a > b)
  const max =
    ceiling === undefined
      ? search.max
      : tighterBound(search.max, ceiling, (a, b) => a < b)
  const q =
    search.q === undefined
      ? undefined
      : type === "jaeger"
        ? withJaegerDurationBounds(search.q, floor, ceiling)
        : withDurationBounds(search.q, floor, ceiling)
  if (q === null) return null
  return {
    from: new Date(selection.from).toISOString(),
    to: new Date(selection.to).toISOString(),
    min,
    max,
    q,
  }
}

function goDuration(text: string): string {
  return text.replace("µs", "us")
}

export function traceQLFor(form: TraceForm): string {
  const conditions = [
    form.service && `resource.service.name = ${quoted(form.service)}`,
    form.span && `name = ${quoted(form.span)}`,
    form.min && `duration >= ${goDuration(form.min)}`,
    form.max && `duration <= ${goDuration(form.max)}`,
    form.errors && "status = error",
  ].filter(Boolean)
  return `{${conditions.join(" && ")}}`
}

const jaegerNoFilter = "minDuration=0s"

function jaegerValue(text: string): string {
  return /[\s"=\\]/.test(text) ? quoted(text) : text
}

export function jaegerSearchFor(form: TraceForm): string {
  const pairs = [
    form.service && `service=${jaegerValue(form.service)}`,
    form.span && `operation=${jaegerValue(form.span)}`,
    form.min && `minDuration=${goDuration(form.min)}`,
    form.max && `maxDuration=${goDuration(form.max)}`,
    form.errors && "error=true",
  ].filter(Boolean)
  return pairs.length === 0 ? jaegerNoFilter : pairs.join(" ")
}

const jaegerDurationPair =
  /(?:^|\s)(?:minDuration|maxDuration)=(?:"(?:[^"\\]|\\.)*"|\S*)/g

export function withJaegerDurationBounds(
  q: string,
  min: string,
  max: string | undefined
): string {
  const kept = q.replace(jaegerDurationPair, "").trim()
  return [
    kept === jaegerNoFilter ? "" : kept,
    `minDuration=${goDuration(min)}`,
    max === undefined ? "" : `maxDuration=${goDuration(max)}`,
  ]
    .filter(Boolean)
    .join(" ")
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

const traceSpans = {
  parent: "otel_traces.ParentSpanId",
  time: "otel_traces.Timestamp",
}

function rootOf(column: string): string {
  const root = `${traceSpans.parent} = ''`
  return `if(countIf(${root}) > 0, argMinIf(otel_traces.${column}, ${traceSpans.time}, ${root}), argMin(otel_traces.${column}, ${traceSpans.time}))`
}

export function clickhouseTraceSearch(form: TraceForm): string {
  const conditions = clickhouseConditions(form, "otel_traces.")
  const joined = conditions.join(" AND ")
  const matching =
    conditions.length > 0 ? `, countIf(${joined}) AS Matching` : ""
  const scope =
    conditions.length > 0
      ? ` AND TraceId IN (SELECT TraceId FROM otel_traces WHERE $__timeFilter(otel_traces.Timestamp) AND ${joined})`
      : ""
  return `SELECT TraceId, min(${traceSpans.time}) AS StartTime, min(${traceSpans.time}) AS Timestamp, ${rootOf("ServiceName")} AS ServiceName, ${rootOf("SpanName")} AS SpanName, toUInt64(max(toUnixTimestamp64Nano(${traceSpans.time}) + toInt64(otel_traces.Duration)) - min(toUnixTimestamp64Nano(${traceSpans.time}))) AS Duration, count() AS Spans, countIf(${clickhouseErrorStatus}) AS Errors${matching}
FROM otel_traces
WHERE $__timeFilter(otel_traces.Timestamp)${scope}
GROUP BY TraceId`
}

const clickhouseSpanSelect =
  "SELECT TraceId AS traceID, SpanId AS spanID, ParentSpanId AS parentSpanID, ServiceName AS serviceName, SpanName AS operationName, Timestamp AS startTime, Duration / 1000000 AS duration, SpanAttributes AS tags FROM otel_traces"

export function clickhouseLatestTraceSpans(form: TraceForm): string {
  const where = ["$__timeFilter(Timestamp)", ...clickhouseConditions(form, "")]
  return `${clickhouseSpanSelect} WHERE TraceId = (SELECT TraceId FROM otel_traces WHERE ${where.join(" AND ")} ORDER BY Timestamp DESC LIMIT 1) ORDER BY startTime LIMIT 1000`
}

export function clickhouseTraceSpans(traceId: string): string {
  return `${clickhouseSpanSelect} WHERE TraceId = '${traceId}' AND $__timeFilter(Timestamp) ORDER BY startTime LIMIT 1000`
}

export function traceStatsFilter(type: string, search: TraceSearch): string {
  if (type === "tempo") return textSearchFor(type, search)
  return clickhouseConditions(traceForm(search), "").join(" AND ")
}

export function textSearchFor(type: string, search: TraceSearch): string {
  return (
    search.q ??
    (type === "jaeger"
      ? jaegerSearchFor(traceForm(search))
      : traceQLFor(traceForm(search)))
  )
}

export function traceSearchParams(
  type: string,
  search: TraceSearch
): ExploreParams {
  return {
    query: hasQueryText(type)
      ? textSearchFor(type, search)
      : clickhouseTraceSearch(traceForm(search)),
    kind: "instant",
    ...tracesTimeRange(search),
    pageSize: search.size ?? defaultPageSize,
    page: search.page ?? 1,
    ...(type === "clickhouse" && sortFor(search)),
  }
}

function sortFor(search: TraceSearch): { sort?: string } {
  const sort = pageSortParam(decodeSort(search.sort))
  return sort === undefined ? {} : { sort }
}

export function traceSearchParamsWithCursor(
  params: ExploreParams,
  cursor: string | undefined
): ExploreParams {
  return cursor === undefined ? params : { ...params, cursor }
}
