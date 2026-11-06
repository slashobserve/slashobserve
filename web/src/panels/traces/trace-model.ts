import { assignSlots } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import type { PreparedField, PreparedFrame } from "../transforms"
import { rowCount } from "../transforms/names"

export const maxSpans = 5000

export const missingColumnsReason =
  "A traces panel needs one row per span with traceID, spanID, parentSpanID, startTime and duration columns."

type AttributeSource = "span" | "resource"

export type SpanAttribute = {
  key: string
  text: string
  source: AttributeSource
}

export type SpanEvent = {
  time: number
  name: string
  attributes: Record<string, string>
}

export type SpanLink = {
  traceId: string
  spanId: string
  attributes: Record<string, string>
}

export type Span = {
  id: string
  parentId: string | null
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
  kind: string | null
  events: SpanEvent[]
  links: SpanLink[]
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
  kind?: PreparedField
  events?: PreparedField
  links?: PreparedField
  attributes: { field: PreparedField; source: AttributeSource }[]
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
  kind: string | null
  events: SpanEvent[]
  links: SpanLink[]
}

const traceIdNames = new Set(["traceid"])
const spanIdNames = new Set(["spanid"])
const parentIdNames = new Set(["parentspanid"])
const operationNames = new Set(["operationname", "spanname"])
const serviceNames = new Set(["servicename"])
const startNames = new Set(["starttime", "timestamp"])
const durationNames = new Set(["duration"])
const statusNames = new Set(["statuscode"])
const kindNames = new Set(["spankind"])
const eventNames = new Set(["spanevents"])
const linkNames = new Set(["spanlinks"])
const attributeSources = new Map<string, AttributeSource>([
  ["tags", "span"],
  ["spanattributes", "span"],
  ["servicetags", "resource"],
  ["resourceattributes", "resource"],
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
    kind: named(frame, kindNames),
    events: named(frame, eventNames),
    links: named(frame, linkNames),
    attributes: frame.fields.flatMap((field) => {
      const source = attributeSources.get(field.name.toLowerCase())
      return source ? [{ field, source }] : []
    }),
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

function finite(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
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

function pairsOf(value: unknown, source: AttributeSource): SpanAttribute[] {
  const parsed = typeof value === "string" ? parseJson(value) : value
  if (Array.isArray(parsed)) {
    return parsed.flatMap((tag) =>
      isRecord(tag) && "key" in tag
        ? [{ key: String(tag.key), text: textOf(tag.value), source }]
        : []
    )
  }
  if (isRecord(parsed)) {
    return Object.entries(parsed).map(([key, item]) => ({
      key,
      text: textOf(item),
      source,
    }))
  }
  return []
}

function eventsOf(value: unknown): SpanEvent[] {
  const parsed = typeof value === "string" ? parseJson(value) : value
  if (!Array.isArray(parsed)) return []
  return parsed.flatMap((event) => {
    if (!isRecord(event)) return []
    const time = finite(event.time_ms)
    if (time === null) return []
    return [
      {
        time,
        name: textOf(event.name ?? ""),
        attributes: textPairs(event.attributes),
      },
    ]
  })
}

function textPairs(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {}
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [key, textOf(item)])
  )
}

function linksOf(value: unknown): SpanLink[] {
  const parsed = typeof value === "string" ? parseJson(value) : value
  if (!Array.isArray(parsed)) return []
  return parsed.flatMap((link) => {
    if (!isRecord(link)) return []
    const traceId = idOf(link.trace_id)
    const spanId = idOf(link.span_id)
    if (traceId === null || spanId === null) return []
    return [{ traceId, spanId, attributes: textPairs(link.attributes) }]
  })
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
          .flatMap(({ field, source }) => pairsOf(field.values[index], source))
          .sort(compareKeys),
        kind: idOf(columns.kind?.values[index]),
        events: eventsOf(columns.events?.values[index]),
        links: linksOf(columns.links?.values[index]),
      },
    })
  }
  return rows
}

function treeOrder(rows: SpanRow[]) {
  const byId = new Map<string, SpanRow>()
  for (const row of rows) if (!byId.has(row.id)) byId.set(row.id, row)
  const unique = [...byId.values()]
  const parentOf = (row: SpanRow): string | null =>
    row.parentId !== null && row.parentId !== row.id && byId.has(row.parentId)
      ? row.parentId
      : null
  const children = new Map<string, SpanRow[]>()
  const roots: SpanRow[] = []
  for (const row of unique) {
    const parentId = parentOf(row)
    if (parentId !== null) {
      const siblings = children.get(parentId) ?? []
      siblings.push(row)
      children.set(parentId, siblings)
    } else {
      roots.push(row)
    }
  }
  const ordered: { row: SpanRow; depth: number; parentId: string | null }[] = []
  const visited = new Set<string>()
  const walk = (root: SpanRow) => {
    const stack = [{ row: root, depth: 0 }]
    while (stack.length > 0) {
      const item = stack.pop()!
      if (visited.has(item.row.id)) continue
      visited.add(item.row.id)
      ordered.push({ ...item, parentId: parentOf(item.row) })
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
  fields: FieldResolver,
  options: { maxSpans?: number } = {}
): TraceModel {
  const cap = options.maxSpans ?? maxSpans
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
  const rows = byStart.slice(0, cap)
  if (byStart.length > cap) {
    notes.push(
      `Showing the first ${cap.toLocaleString("en")} of ${byStart.length.toLocaleString("en")} spans.`
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
  const spans = ordered.map(({ row, depth, parentId }): Span => ({
    id: row.id,
    parentId,
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
    kind: row.kind,
    events: row.events,
    links: row.links,
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

export const maxEventTicks = 20

export function eventTicks(
  events: readonly SpanEvent[],
  trace: { start: number; duration: number },
  zoom: { start: number; end: number }
): number[] {
  const window = zoom.end - zoom.start
  if (trace.duration <= 0 || window <= 0) return []
  const ticks: number[] = []
  for (const event of events) {
    if (ticks.length === maxEventTicks) break
    const at =
      ((event.time - trace.start) / trace.duration - zoom.start) / window
    if (at >= 0 && at <= 1) ticks.push(at)
  }
  return ticks
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
