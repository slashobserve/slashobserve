import type { Span } from "./trace-model"

export type SpanFilters = {
  search: string
  serviceNames: readonly string[]
  excludeServices: boolean
  spanNames: readonly string[]
  excludeSpanNames: boolean
  minDuration: number | null
  maxDuration: number | null
  matchesOnly: boolean
  criticalPathOnly: boolean
}

export type FilteredSpans = {
  spans: Span[]
  matched: ReadonlySet<string> | null
  notes: string[]
}

function names(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((name): name is string => typeof name === "string")
    : []
}

function milliseconds(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

export function spanFilters(display: Record<string, unknown>): SpanFilters {
  return {
    search:
      typeof display.spanSearch === "string" ? display.spanSearch.trim() : "",
    serviceNames: names(display.serviceNames),
    excludeServices: display.excludeServices === true,
    spanNames: names(display.spanNames),
    excludeSpanNames: display.excludeSpanNames === true,
    minDuration: milliseconds(display.minSpanDuration),
    maxDuration: milliseconds(display.maxSpanDuration),
    matchesOnly: display.matchesOnly === true,
    criticalPathOnly: display.criticalPathOnly === true,
  }
}

function filtersMatch(filters: SpanFilters): boolean {
  return (
    filters.search !== "" ||
    filters.serviceNames.length > 0 ||
    filters.spanNames.length > 0 ||
    filters.minDuration !== null ||
    filters.maxDuration !== null
  )
}

function searchText(span: Span): string {
  return [
    span.service,
    span.operation,
    span.id,
    ...span.attributes.flatMap((attribute) => [attribute.key, attribute.text]),
  ]
    .join("\n")
    .toLowerCase()
}

function listed(
  list: readonly string[],
  value: string,
  exclude: boolean
): boolean {
  return list.length === 0 || list.includes(value) !== exclude
}

function matches(span: Span, filters: SpanFilters, needle: string): boolean {
  return (
    (needle === "" || searchText(span).includes(needle)) &&
    listed(filters.serviceNames, span.service, filters.excludeServices) &&
    listed(filters.spanNames, span.operation, filters.excludeSpanNames) &&
    (filters.minDuration === null || span.duration >= filters.minDuration) &&
    (filters.maxDuration === null || span.duration <= filters.maxDuration)
  )
}

function lastFinishing(
  children: readonly Span[],
  until: number
): { span: Span; until: number }[] {
  const ending = children
    .map((span) => ({ span, end: Math.min(span.start + span.duration, until) }))
    .sort((a, b) => b.end - a.end || a.span.start - b.span.start)
  const chosen: { span: Span; until: number }[] = []
  let cursor = until
  for (const { span, end } of ending) {
    if (end > cursor || span.start > end) continue
    chosen.push({ span, until: end })
    cursor = span.start
  }
  return chosen
}

export function criticalPath(spans: readonly Span[]): Set<string> {
  const children = new Map<string | null, Span[]>()
  for (const span of spans) {
    const siblings = children.get(span.parentId) ?? []
    siblings.push(span)
    children.set(span.parentId, siblings)
  }
  const roots = children.get(null) ?? []
  const traceEnd = Math.max(
    ...roots.map((span) => span.start + span.duration),
    -Infinity
  )
  const path = new Set<string>()
  const pending = lastFinishing(roots, traceEnd)
  while (pending.length > 0) {
    const { span, until } = pending.pop()!
    path.add(span.id)
    pending.push(...lastFinishing(children.get(span.id) ?? [], until))
  }
  return path
}

function withChildrenMarked(spans: Span[]): Span[] {
  return spans.map((span, index) => {
    const hasChildren =
      index + 1 < spans.length && spans[index + 1].depth > span.depth
    return hasChildren === span.hasChildren ? span : { ...span, hasChildren }
  })
}

export function filterSpans(
  spans: Span[],
  filters: SpanFilters
): FilteredSpans {
  const needle = filters.search.toLowerCase()
  const matched = filtersMatch(filters)
    ? new Set(
        spans
          .filter((span) => matches(span, filters, needle))
          .map((span) => span.id)
      )
    : null
  const notes: string[] = []
  if (matched !== null) {
    notes.push(
      `${matched.size.toLocaleString("en")} of ${spans.length.toLocaleString("en")} spans match the filters.`
    )
  }
  let shown = spans
  if (filters.criticalPathOnly) {
    const path = criticalPath(spans)
    shown = shown.filter((span) => path.has(span.id))
    notes.push(
      `Showing the ${shown.length.toLocaleString("en")} spans on the critical path.`
    )
  }
  if (filters.matchesOnly && matched !== null) {
    shown = shown.filter((span) => matched.has(span.id))
  }
  return {
    spans: shown === spans ? spans : withChildrenMarked(shown),
    matched,
    notes,
  }
}
