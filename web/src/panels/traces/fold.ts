import { visibleSpans, type Span } from "./trace-model"

export const foldThreshold = 20
export const foldPage = 50

export type FoldGroup = {
  id: string
  parentId: string
  service: string
  operation: string
  slot: number
  depth: number
  spans: Span[]
  slowest: Span[]
  count: number
  totalMs: number
  slowestMs: number
  errors: number
  offset: number
  width: number
}

export type Row =
  | { kind: "span"; span: Span }
  | { kind: "fold"; group: FoldGroup }
  | { kind: "more"; group: FoldGroup; shown: number }

export const noGroups: ReadonlyMap<string, FoldGroup> = new Map()

function groupKey(span: Span): string | null {
  return span.parentId === null
    ? null
    : `fold:${span.parentId}:${span.service}:${span.operation}`
}

function groupOf(
  span: Span,
  groups: ReadonlyMap<string, FoldGroup>
): FoldGroup | undefined {
  const key = groupKey(span)
  return key === null ? undefined : groups.get(key)
}

export function foldGroups(
  spans: readonly Span[],
  threshold: number
): Map<string, FoldGroup> {
  const members = new Map<string, Span[]>()
  for (const span of spans) {
    const key = groupKey(span)
    if (key === null) continue
    const list = members.get(key)
    if (list) list.push(span)
    else members.set(key, [span])
  }
  const groups = new Map<string, FoldGroup>()
  for (const [id, list] of members) {
    if (list.length <= threshold) continue
    const first = list[0]
    let totalMs = 0
    let slowestMs = 0
    let errors = 0
    let offset = Infinity
    let end = -Infinity
    for (const span of list) {
      totalMs += span.duration
      slowestMs = Math.max(slowestMs, span.duration)
      if (span.error) errors += 1
      offset = Math.min(offset, span.offset)
      end = Math.max(end, span.offset + span.width)
    }
    groups.set(id, {
      id,
      parentId: first.parentId ?? "",
      service: first.service,
      operation: first.operation,
      slot: first.slot,
      depth: first.depth,
      spans: list,
      slowest: [...list].sort((a, b) => b.duration - a.duration),
      count: list.length,
      totalMs,
      slowestMs,
      errors,
      offset,
      width: end - offset,
    })
  }
  return groups
}

export function foldedRows(
  spans: readonly Span[],
  groups: ReadonlyMap<string, FoldGroup>,
  collapsed: ReadonlySet<string>,
  opened: ReadonlyMap<string, number>
): Row[] {
  if (groups.size === 0) {
    return visibleSpans(spans, collapsed).map((span) => ({
      kind: "span",
      span,
    }))
  }
  const indexOf = new Map<string, number>()
  spans.forEach((span, index) => indexOf.set(span.id, index))
  const subtreeEnd = (index: number) => {
    const depth = spans[index].depth
    let end = index + 1
    while (end < spans.length && spans[end].depth > depth) end += 1
    return end
  }
  const rows: Row[] = []
  const emitted = new Set<string>()
  const walk = (from: number, to: number) => {
    let hiddenBelow = Infinity
    let index = from
    while (index < to) {
      const span = spans[index]
      if (span.depth > hiddenBelow) {
        index += 1
        continue
      }
      const group = groupOf(span, groups)
      if (group) {
        if (!emitted.has(group.id)) {
          emitted.add(group.id)
          rows.push({ kind: "fold", group })
          const shown = Math.min(opened.get(group.id) ?? 0, group.count)
          for (const member of group.slowest.slice(0, shown)) {
            const start = indexOf.get(member.id)
            if (start === undefined) continue
            rows.push({ kind: "span", span: member })
            if (!collapsed.has(member.id)) walk(start + 1, subtreeEnd(start))
          }
          if (shown > 0 && shown < group.count) {
            rows.push({ kind: "more", group, shown })
          }
        }
        hiddenBelow = Infinity
        index = subtreeEnd(index)
        continue
      }
      rows.push({ kind: "span", span })
      hiddenBelow = collapsed.has(span.id) ? span.depth : Infinity
      index += 1
    }
  }
  walk(0, spans.length)
  return rows
}

export function rowId(row: Row): string {
  switch (row.kind) {
    case "span":
      return row.span.id
    case "fold":
      return row.group.id
    case "more":
      return `${row.group.id}:more`
  }
}

export function foldLabel(
  group: FoldGroup,
  formatDuration: (ms: number) => string
): string {
  const errors =
    group.errors === 0
      ? ""
      : `, ${group.errors.toLocaleString("en")} ${group.errors === 1 ? "error" : "errors"}`
  return `${formatDuration(group.totalMs)} total, slowest ${formatDuration(group.slowestMs)}${errors}`
}

export function moreLabel(group: FoldGroup, shown: number): string {
  const left = group.count - shown
  return `Show ${Math.min(foldPage, left).toLocaleString("en")} more of ${left.toLocaleString("en")}`
}

export function revealSpan(
  opened: ReadonlyMap<string, number>,
  groups: ReadonlyMap<string, FoldGroup>,
  spans: readonly Span[],
  id: string
): ReadonlyMap<string, number> {
  if (groups.size === 0) return opened
  const byId = new Map(spans.map((span) => [span.id, span]))
  const seen = new Set<string>()
  let next: Map<string, number> | null = null
  let span = byId.get(id)
  while (span && !seen.has(span.id)) {
    seen.add(span.id)
    const group = groupOf(span, groups)
    if (group) {
      const current = span
      const rank =
        group.slowest.findIndex((member) => member.id === current.id) + 1
      const needed = Math.ceil(rank / foldPage) * foldPage
      if ((opened.get(group.id) ?? 0) < needed) {
        next ??= new Map(opened)
        next.set(group.id, needed)
      }
    }
    span = span.parentId === null ? undefined : byId.get(span.parentId)
  }
  return next ?? opened
}
