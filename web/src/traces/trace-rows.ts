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
