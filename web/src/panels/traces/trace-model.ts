import { assignSlots } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import type { PreparedField, PreparedFrame } from "../transforms"
import { rowCount } from "../transforms/names"

export const maxSpans = 5000

export const missingColumnsReason =
  "A traces panel needs one row per span with traceID, spanID, parentSpanID, startTime and duration columns."

type SpanAttribute = { key: string; text: string }

export type Span = {
  id: string
  service: string
  operation: string
  start: number
  duration: number
  offset: number
  width: number
  depth: number
  slot: number
  error: boolean
  hasChildren: boolean
  attributes: SpanAttribute[]
}

export type TraceModel =
  | { reason: string }
  | {
      reason: null
      traceId: string
      rootService: string
      rootOperation: string
      start: number
      duration: number
      spans: Span[]
      notes: string[]
      formatDuration: (ms: number) => string
    }

export type RowWindow = {
  start: number
  end: number
  before: number
  after: number
}

type Columns = {
  traceId?: PreparedField
  spanId: PreparedField
  parentId?: PreparedField
  operation?: PreparedField
  service?: PreparedField
  start: PreparedField
  duration?: PreparedField
  status?: PreparedField
  attributes: PreparedField[]
}

type SpanRow = {
  id: string
  parentId: string | null
  service: string
  operation: string
  start: number
  duration: number
  error: boolean
  attributes: SpanAttribute[]
}

const traceIdNames = new Set(["traceid"])
const spanIdNames = new Set(["spanid"])
const parentIdNames = new Set(["parentspanid"])
const operationNames = new Set(["operationname", "spanname"])
const serviceNames = new Set(["servicename"])
const startNames = new Set(["starttime", "timestamp"])
const durationNames = new Set(["duration"])
const statusNames = new Set(["statuscode"])
const attributeNames = new Set([
  "tags",
  "servicetags",
  "spanattributes",
  "resourceattributes",
])
const errorStatuses = new Set(["2", "status_code_error", "error"])
const nanosecondsColumn = "Duration"
const nanosecondsPerMs = 1e6

function named(
  frame: PreparedFrame,
  names: Set<string>
): PreparedField | undefined {
  return frame.fields.find((field) => names.has(field.name.toLowerCase()))
}

function columnsOf(frame: PreparedFrame): Columns | null {
  const spanId = named(frame, spanIdNames)
  const start = named(frame, startNames)
  if (!spanId || !start) return null
  return {
    traceId: named(frame, traceIdNames),
    spanId,
    parentId: named(frame, parentIdNames),
    operation: named(frame, operationNames),
    service: named(frame, serviceNames),
    start,
    duration: named(frame, durationNames),
    status: named(frame, statusNames),
    attributes: frame.fields.filter((field) =>
      attributeNames.has(field.name.toLowerCase())
    ),
  }
}

function textOf(value: unknown): string {
  if (typeof value === "string") return value
  if (typeof value === "object" && value !== null) return JSON.stringify(value)
  return String(value)
}

function idOf(value: unknown): string | null {
  if (value === null || value === undefined) return null
  const text = String(value)
  return text === "" ? null : text
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function parseJson(text: string): unknown {
  const trimmed = text.trim()
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return undefined
  try {
    return JSON.parse(trimmed)
  } catch {
    return undefined
  }
}

function pairsOf(value: unknown): SpanAttribute[] {
  const parsed = typeof value === "string" ? parseJson(value) : value
  if (Array.isArray(parsed)) {
    return parsed.flatMap((tag) =>
      isRecord(tag) && "key" in tag
        ? [{ key: String(tag.key), text: textOf(tag.value) }]
        : []
    )
  }
  if (isRecord(parsed)) {
    return Object.entries(parsed).map(([key, item]) => ({
      key,
      text: textOf(item),
    }))
  }
  return []
}

function compareKeys(a: SpanAttribute, b: SpanAttribute): number {
  return a.key < b.key ? -1 : a.key > b.key ? 1 : 0
}

function convertsNanoseconds(field: PreparedField | undefined): boolean {
  return (
    field?.name === nanosecondsColumn &&
    field.values.every(
      (value) => value === null || Number.isInteger(value as number)
    )
  )
}

function finite(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

function rowsOf(frame: PreparedFrame, columns: Columns) {
  const nanoseconds = convertsNanoseconds(columns.duration)
  const rows: { traceId: string; span: SpanRow }[] = []
  const count = rowCount(frame)
  for (let index = 0; index < count; index++) {
    const id = idOf(columns.spanId.values[index])
    const start = finite(columns.start.values[index])
    if (id === null || start === null) continue
    const duration = finite(columns.duration?.values[index]) ?? 0
    const status = columns.status?.values[index]
    rows.push({
      traceId: idOf(columns.traceId?.values[index]) ?? "",
      span: {
        id,
        parentId: idOf(columns.parentId?.values[index]),
        service: textOf(columns.service?.values[index] ?? ""),
        operation: textOf(columns.operation?.values[index] ?? ""),
        start,
        duration: nanoseconds ? duration / nanosecondsPerMs : duration,
        error:
          status !== null &&
          status !== undefined &&
          errorStatuses.has(String(status).toLowerCase()),
        attributes: columns.attributes
          .flatMap((field) => pairsOf(field.values[index]))
          .sort(compareKeys),
      },
    })
  }
  return rows
}

function treeOrder(rows: SpanRow[]) {
  const byId = new Map<string, SpanRow>()
  for (const row of rows) if (!byId.has(row.id)) byId.set(row.id, row)
  const unique = [...byId.values()]
  const children = new Map<string, SpanRow[]>()
  const roots: SpanRow[] = []
  for (const row of unique) {
    if (
      row.parentId !== null &&
      row.parentId !== row.id &&
      byId.has(row.parentId)
    ) {
      const siblings = children.get(row.parentId) ?? []
      siblings.push(row)
      children.set(row.parentId, siblings)
    } else {
      roots.push(row)
    }
  }
  const ordered: { row: SpanRow; depth: number }[] = []
  const visited = new Set<string>()
  const walk = (root: SpanRow) => {
    const stack = [{ row: root, depth: 0 }]
    while (stack.length > 0) {
      const item = stack.pop()!
      if (visited.has(item.row.id)) continue
      visited.add(item.row.id)
      ordered.push(item)
      const below = children.get(item.row.id) ?? []
      for (let index = below.length - 1; index >= 0; index--) {
        stack.push({ row: below[index], depth: item.depth + 1 })
      }
    }
  }
  for (const root of roots) walk(root)
  for (const row of unique) if (!visited.has(row.id)) walk(row)
  return { ordered, children }
}

export function traceModel(
  frames: PreparedFrame[],
  fields: FieldResolver
): TraceModel {
  const located = frames.flatMap((frame) => {
    const columns = columnsOf(frame)
    return columns ? [{ frame, columns }] : []
  })
  const traces = new Map<string, SpanRow[]>()
  for (const { frame, columns } of located) {
    for (const { traceId, span } of rowsOf(frame, columns)) {
      const spans = traces.get(traceId) ?? []
      spans.push(span)
      traces.set(traceId, spans)
    }
  }
  const first = traces.entries().next()
  if (first.done) return { reason: missingColumnsReason }
  const [traceId, all] = first.value
  const { frame, columns } = located[0]

  const notes: string[] = []
  if (traces.size > 1) notes.push(`Showing trace 1 of ${traces.size}.`)
  const byStart = [...all].sort((a, b) => a.start - b.start)
  const rows = byStart.slice(0, maxSpans)
  if (byStart.length > maxSpans) {
    notes.push(
      `Showing the first ${maxSpans.toLocaleString("en")} of ${byStart.length.toLocaleString("en")} spans.`
    )
  }

  const { ordered, children } = treeOrder(rows)
  const traceStart = Math.min(...rows.map((row) => row.start))
  const traceEnd = Math.max(...rows.map((row) => row.start + row.duration))
  const total = traceEnd - traceStart
  const slots = assignSlots(
    new Map(),
    ordered.map(({ row }) => row.service)
  )
  const spans = ordered.map(({ row, depth }): Span => ({
    id: row.id,
    service: row.service,
    operation: row.operation,
    start: row.start,
    duration: row.duration,
    offset: total > 0 ? (row.start - traceStart) / total : 0,
    width: total > 0 ? row.duration / total : 0,
    depth,
    slot: slots.get(row.service) ?? 0,
    error: row.error,
    hasChildren: children.has(row.id),
    attributes: row.attributes,
  }))

  const durationConfig = {
    ...fields.configFor(frame, columns.duration ?? columns.start),
    unit: "ms",
  }
  return {
    reason: null,
    traceId,
    rootService: spans[0].service,
    rootOperation: spans[0].operation,
    start: traceStart,
    duration: total,
    spans,
    notes,
    formatDuration: (ms) => displayValue(ms, durationConfig).text,
  }
}

export function visibleSpans(
  spans: readonly Span[],
  collapsed: ReadonlySet<string>
): Span[] {
  const visible: Span[] = []
  let hiddenBelow = Infinity
  for (const span of spans) {
    if (span.depth > hiddenBelow) continue
    hiddenBelow = collapsed.has(span.id) ? span.depth : Infinity
    visible.push(span)
  }
  return visible
}

export function windowRows(input: {
  count: number
  rowHeight: number
  scrollTop: number
  viewport: number
  openIndex: number
  detailHeight: number
  overscan: number
}): RowWindow {
  const { count, rowHeight, openIndex, detailHeight, overscan } = input
  const hasDetail = openIndex >= 0 && openIndex < count
  const detailStart = (openIndex + 1) * rowHeight
  const indexAt = (offset: number) => {
    if (!hasDetail || offset < detailStart)
      return Math.floor(offset / rowHeight)
    if (offset < detailStart + detailHeight) return openIndex
    return Math.floor((offset - detailHeight) / rowHeight)
  }
  const clamp = (index: number) => Math.min(Math.max(index, 0), count)
  const start = clamp(indexAt(input.scrollTop) - overscan)
  const end = clamp(indexAt(input.scrollTop + input.viewport) + overscan + 1)
  return {
    start,
    end,
    before:
      start * rowHeight + (hasDetail && openIndex < start ? detailHeight : 0),
    after:
      (count - end) * rowHeight +
      (hasDetail && openIndex >= end ? detailHeight : 0),
  }
}
