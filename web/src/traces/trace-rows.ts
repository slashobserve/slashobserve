import type { WireFrame } from "@/dashboards/queries"

export type TraceRow = {
  traceId: string
  start: number | null
  service: string
  span: string
  durationMs: number | null
  spans: number | null
  errors: number | null
}

function column(frame: WireFrame, name: string) {
  const wanted = name.toLowerCase()
  return frame.fields.find((field) => field.name.toLowerCase() === wanted)
}

function numberAt(values: unknown[] | undefined, row: number): number | null {
  const value = values?.[row]
  if (typeof value === "number" && Number.isFinite(value)) return value
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

function textAt(values: unknown[] | undefined, row: number): string {
  const value = values?.[row]
  return value === null || value === undefined ? "" : String(value)
}

export function traceRows(frames: readonly WireFrame[]): TraceRow[] {
  const rows: TraceRow[] = []
  for (const frame of frames) {
    const ids = column(frame, "TraceId")
    if (!ids) continue
    const start = column(frame, "StartTime")?.values
    const service = column(frame, "ServiceName")?.values
    const span = column(frame, "SpanName")?.values
    const duration = column(frame, "Duration")?.values
    const spans = column(frame, "Spans")?.values
    const errors = column(frame, "Errors")?.values
    for (let row = 0; row < ids.values.length; row++) {
      const traceId = textAt(ids.values, row)
      if (traceId === "") continue
      const ns = numberAt(duration, row)
      rows.push({
        traceId,
        start: numberAt(start, row),
        service: textAt(service, row),
        span: textAt(span, row),
        durationMs: ns === null ? null : ns / 1e6,
        spans: numberAt(spans, row),
        errors: numberAt(errors, row),
      })
    }
  }
  return rows
}

export function hasErrorCounts(rows: readonly TraceRow[]): boolean {
  return rows.length > 0 && rows.every((row) => row.errors !== null)
}

export function hasSpanCounts(rows: readonly TraceRow[]): boolean {
  return rows.length > 0 && rows.every((row) => row.spans !== null)
}

export function countText(count: number, noun: string): string {
  return `${count.toLocaleString("en")} ${noun}${count === 1 ? "" : "s"}`
}

export function durationText(ms: number | null): string {
  return ms === null
    ? "-"
    : `${ms.toLocaleString("en", { maximumFractionDigits: 1 })} ms`
}

export function openTraceLabel(row: TraceRow): string {
  const parts = [`Open trace ${row.traceId.slice(0, 8)}`]
  if (row.span !== "") parts.push(row.span)
  if (row.durationMs !== null) {
    parts.push(`${(row.durationMs / 1000).toFixed(2)} seconds`)
  }
  return parts.join(", ")
}

export type TraceSortKey =
  | "start"
  | "service"
  | "span"
  | "duration"
  | "spans"
  | "errors"
export type TraceSortDirection = "ascending" | "descending"
export type TraceSort = { key: TraceSortKey; direction: TraceSortDirection }

export const defaultTraceSort: TraceSort = {
  key: "duration",
  direction: "descending",
}

const sortValues: Record<TraceSortKey, (row: TraceRow) => number | string | null> = {
  start: (row) => row.start,
  service: (row) => row.service || null,
  span: (row) => row.span || null,
  duration: (row) => row.durationMs,
  spans: (row) => row.spans,
  errors: (row) => row.errors,
}

function compareValues(a: number | string, b: number | string): number {
  if (typeof a === "string" || typeof b === "string") {
    return String(a).localeCompare(String(b))
  }
  return a - b
}

export function sortTraceRows(
  rows: readonly TraceRow[],
  sort: TraceSort | null
): TraceRow[] {
  if (sort === null) return [...rows]
  const valueOf = sortValues[sort.key]
  const sign = sort.direction === "ascending" ? 1 : -1
  return [...rows].sort((a, b) => {
    const left = valueOf(a)
    const right = valueOf(b)
    if (left === null) return right === null ? 0 : 1
    if (right === null) return -1
    return compareValues(left, right) * sign
  })
}

function firstDirection(key: TraceSortKey): TraceSortDirection {
  return key === "duration" ? "descending" : "ascending"
}

export function nextSort(
  current: TraceSort | null,
  key: TraceSortKey
): TraceSort | null {
  const first = firstDirection(key)
  if (current?.key !== key) return { key, direction: first }
  if (current.direction !== first) return null
  return {
    key,
    direction: first === "ascending" ? "descending" : "ascending",
  }
}

const sortHints: Record<TraceSortKey, [string, string]> = {
  start: ["Oldest first.", "Newest first."],
  service: ["Root service A to Z.", "Root service Z to A."],
  span: ["Root span A to Z.", "Root span Z to A."],
  duration: ["Fastest first.", "Slowest first."],
  spans: ["Fewest spans first.", "Most spans first."],
  errors: ["Fewest errors first.", "Most errors first."],
}

export function sortHint(sort: TraceSort | null): string {
  if (sort === null) return "Click a column to sort."
  const [ascending, descending] = sortHints[sort.key]
  return `${sort.direction === "ascending" ? ascending : descending} Click a column to sort.`
}

export function percentile(
  values: readonly number[],
  p: number
): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const rank = Math.min(
    sorted.length,
    Math.max(1, Math.ceil((p / 100) * sorted.length))
  )
  return sorted[rank - 1] ?? null
}
