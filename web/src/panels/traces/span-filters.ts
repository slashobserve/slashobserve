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
    span.statusMessage,
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

function lastFinishing(children: readonly Span[]): Span[] {
  const ending = children
    .map((span) => ({ span, end: span.start + span.duration }))
    .sort((a, b) => b.end - a.end || a.span.start - b.span.start)
  const chosen: Span[] = []
  let cursor = Infinity
  for (const { span, end } of ending) {
    if (end > cursor) continue
    chosen.push(span)
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
  const path = new Set<string>()
  const pending = lastFinishing(children.get(null) ?? [])
  while (pending.length > 0) {
    const span = pending.pop()!
    path.add(span.id)
    pending.push(...lastFinishing(children.get(span.id) ?? []))
  }
  return path
}

function parentsOf(spans: readonly Span[]): Map<string, string | null> {
  return new Map(spans.map((span) => [span.id, span.parentId]))
}

function walkUp(
  parents: Map<string, string | null>,
  id: string,
  visit: (ancestor: string) => boolean
) {
  let current = parents.get(id) ?? null
  while (current !== null && visit(current))
    current = parents.get(current) ?? null
}

export function ancestorIds(spans: readonly Span[], id: string): string[] {
  const ancestors: string[] = []
  walkUp(parentsOf(spans), id, (ancestor) => {
    if (ancestors.includes(ancestor)) return false
    ancestors.push(ancestor)
    return true
  })
  return ancestors
}

export function errorAncestors(spans: readonly Span[]): Set<string> {
  const parents = parentsOf(spans)
  const marked = new Set<string>()
  for (const span of spans) {
    if (!span.error) continue
    marked.add(span.id)
    walkUp(parents, span.id, (ancestor) => {
      if (marked.has(ancestor)) return false
      marked.add(ancestor)
      return true
    })
  }
  return marked
}

function withChildrenMarked(spans: Span[]): Span[] {
  return spans.map((span, index) => {
    const next = spans[index + 1]
    const hasChildren = next !== undefined && next.depth > span.depth
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
