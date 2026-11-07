import { describe, expect, it } from "vitest"

import {
  foldGroups,
  foldLabel,
  foldPage,
  foldThreshold,
  foldedRows,
  moreLabel,
  noGroups,
  revealSpan,
  rowId,
  type FoldGroup,
} from "./fold"
import type { Span } from "./trace-model"

type SpanSeed = {
  id: string
  parentId?: string | null
  service?: string
  operation?: string
  start?: number
  duration?: number
  depth?: number
  error?: boolean
  hasChildren?: boolean
}

const traceMs = 1000

function span(seed: SpanSeed): Span {
  const start = seed.start ?? 0
  const duration = seed.duration ?? 1
  return {
    id: seed.id,
    parentId: seed.parentId ?? null,
    service: seed.service ?? "svc",
    operation: seed.operation ?? "op",
    start,
    duration,
    offset: start / traceMs,
    width: duration / traceMs,
    depth: seed.depth ?? 0,
    slot: 0,
    error: seed.error ?? false,
    hasChildren: seed.hasChildren ?? false,
    attributes: [],
    kind: null,
    events: [],
    links: [],
  }
}

function siblings(
  parentId: string,
  count: number,
  operation: string,
  depth: number,
  durationOf: (index: number) => number = () => 1
): Span[] {
  return Array.from({ length: count }, (_, index) =>
    span({
      id: `${operation}-${index}`,
      parentId,
      service: "catalog",
      operation,
      start: index * 2,
      duration: durationOf(index),
      depth,
    })
  )
}

function ids(rows: ReturnType<typeof foldedRows>): string[] {
  return rows.map(rowId)
}

const root = span({ id: "root", hasChildren: true, duration: traceMs })

function indexTrace(count: number): Span[] {
  const batch = span({
    id: "batch",
    parentId: "root",
    operation: "batch",
    depth: 1,
    hasChildren: true,
    duration: traceMs,
  })
  return [
    root,
    batch,
    ...siblings("batch", count, "catalog.index_item", 2, (index) =>
      index === 7 ? 412 : 1 + (index % 5)
    ),
  ]
}

const groupId = "fold:batch:catalog:catalog.index_item"

describe("foldGroups", () => {
  it("folds siblings with the same service and operation above the threshold", () => {
    const groups = foldGroups(indexTrace(2060), foldThreshold)
    expect([...groups.keys()]).toEqual([groupId])
    const group = groups.get(groupId)!
    expect(group.count).toBe(2060)
    expect(group.depth).toBe(2)
    expect(group.parentId).toBe("batch")
    expect(group.slowestMs).toBe(412)
    expect(group.slowest[0].id).toBe("catalog.index_item-7")
    expect(group.offset).toBe(0)
    expect(group.width).toBeCloseTo((2059 * 2 + 1) / traceMs)
  })

  it("leaves small sibling sets, roots and mixed operations alone", () => {
    const spans = [
      root,
      ...siblings("root", 3, "small", 1),
      ...siblings("root", 15, "a", 1),
      ...siblings("root", 15, "b", 1),
    ]
    expect(foldGroups(spans, foldThreshold).size).toBe(0)
    const roots = Array.from({ length: 30 }, (_, index) =>
      span({ id: `r${index}` })
    )
    expect(foldGroups(roots, foldThreshold).size).toBe(0)
  })

  it("sums durations and counts errors across the members", () => {
    const spans = [
      root,
      ...siblings("root", 21, "work", 1, () => 10).map((item, index) =>
        index < 2 ? { ...item, error: true } : item
      ),
    ]
    const group = foldGroups(spans, foldThreshold).get(
      "fold:root:catalog:work"
    )!
    expect(group.totalMs).toBe(210)
    expect(group.errors).toBe(2)
  })
})

describe("foldedRows", () => {
  it("draws one row for a closed group and keeps the rest of the tree", () => {
    const spans = indexTrace(2060)
    const groups = foldGroups(spans, foldThreshold)
    const rows = foldedRows(spans, groups, new Set(), new Map())
    expect(ids(rows)).toEqual(["root", "batch", groupId])
    expect(rows[2]).toMatchObject({ kind: "fold" })
  })

  it("opens a group to its slowest members in descending order with a show-more row", () => {
    const spans = indexTrace(2060)
    const groups = foldGroups(spans, foldThreshold)
    const rows = foldedRows(
      spans,
      groups,
      new Set(),
      new Map([[groupId, foldPage]])
    )
    const members = rows.filter((row) => row.kind === "span").slice(2)
    expect(members).toHaveLength(foldPage)
    expect(members[0].kind === "span" && members[0].span.id).toBe(
      "catalog.index_item-7"
    )
    const durations = members.map((row) =>
      row.kind === "span" ? row.span.duration : -1
    )
    expect(durations).toEqual([...durations].sort((a, b) => b - a))
    const last = rows[rows.length - 1]
    expect(last).toMatchObject({ kind: "more", shown: foldPage })
    expect(rowId(last)).toBe(`${groupId}:more`)
    expect(moreLabel(groups.get(groupId)!, foldPage)).toBe(
      "Show 50 more of 2,010"
    )
  })

  it("has no show-more row once every member is open", () => {
    const spans = indexTrace(60)
    const groups = foldGroups(spans, foldThreshold)
    const rows = foldedRows(spans, groups, new Set(), new Map([[groupId, 60]]))
    expect(rows.filter((row) => row.kind === "more")).toEqual([])
    expect(rows).toHaveLength(2 + 1 + 60)
  })

  it("hides a member's children while the group is closed and shows them when open", () => {
    const members = siblings("root", 25, "work", 1).map((item) => ({
      ...item,
      hasChildren: true,
    }))
    const spans = members.flatMap((member) => [
      member,
      span({
        id: `${member.id}-child`,
        parentId: member.id,
        operation: "db",
        depth: 2,
      }),
    ])
    const all = [root, ...spans]
    const groups = foldGroups(all, foldThreshold)
    const id = "fold:root:catalog:work"
    expect(ids(foldedRows(all, groups, new Set(), new Map()))).toEqual([
      "root",
      id,
    ])
    const open = ids(foldedRows(all, groups, new Set(), new Map([[id, 1]])))
    expect(open).toEqual(["root", id, "work-0", "work-0-child", `${id}:more`])
    const collapsed = ids(
      foldedRows(all, groups, new Set(["work-0"]), new Map([[id, 1]]))
    )
    expect(collapsed).toEqual(["root", id, "work-0", `${id}:more`])
  })

  it("hides a group under a collapsed ancestor", () => {
    const spans = indexTrace(100)
    const groups = foldGroups(spans, foldThreshold)
    expect(
      ids(foldedRows(spans, groups, new Set(["batch"]), new Map()))
    ).toEqual(["root", "batch"])
  })

  it("keeps a 12,000-span batch trace under 300 rows before any fold opens", () => {
    const spans = [
      root,
      ...Array.from({ length: 6 }, (_, parent) => [
        span({
          id: `job${parent}`,
          parentId: "root",
          operation: "job",
          depth: 1,
          hasChildren: true,
        }),
        ...siblings(`job${parent}`, 1000, "catalog.index_item", 2),
        ...siblings(`job${parent}`, 1000, "search.bulk_upsert", 2),
      ]).flat(),
    ]
    expect(spans.length).toBeGreaterThan(12000)
    const rows = foldedRows(
      spans,
      foldGroups(spans, foldThreshold),
      new Set(),
      new Map()
    )
    expect(rows.length).toBeLessThan(300)
  })

  it("matches visibleSpans when there are no groups", () => {
    const spans = [
      root,
      span({ id: "a", parentId: "root", depth: 1, hasChildren: true }),
      span({ id: "b", parentId: "a", depth: 2 }),
    ]
    expect(ids(foldedRows(spans, noGroups, new Set(["a"]), new Map()))).toEqual(
      ["root", "a"]
    )
  })
})

describe("foldLabel", () => {
  const format = (ms: number) => (ms >= 1000 ? `${ms / 1000} s` : `${ms} ms`)
  const group = (errors: number): FoldGroup => ({
    id: groupId,
    parentId: "batch",
    service: "catalog",
    operation: "catalog.index_item",
    slot: 0,
    depth: 2,
    spans: [],
    slowest: [],
    count: 2060,
    totalMs: 15700,
    slowestMs: 412,
    errors,
    offset: 0,
    width: 1,
  })

  it("reads total and slowest, with errors only when some failed", () => {
    expect(foldLabel(group(0), format)).toBe("15.7 s total, slowest 412 ms")
    expect(foldLabel(group(2), format)).toBe(
      "15.7 s total, slowest 412 ms, 2 errors"
    )
    expect(foldLabel(group(1), format)).toBe(
      "15.7 s total, slowest 412 ms, 1 error"
    )
  })
})

describe("revealSpan", () => {
  it("opens the fold holding a span, or an ancestor of it, to a page that includes it", () => {
    const spans = indexTrace(2060)
    const groups = foldGroups(spans, foldThreshold)
    const opened = new Map<string, number>()
    expect(revealSpan(opened, groups, spans, "catalog.index_item-7")).toEqual(
      new Map([[groupId, foldPage]])
    )
    const slow = groups.get(groupId)!.slowest[foldPage + 3].id
    expect(revealSpan(opened, groups, spans, slow).get(groupId)).toBe(
      2 * foldPage
    )
  })

  it("returns the same map when nothing needs opening", () => {
    const spans = indexTrace(2060)
    const groups = foldGroups(spans, foldThreshold)
    const opened = new Map([[groupId, foldPage]])
    expect(revealSpan(opened, groups, spans, "batch")).toBe(opened)
    expect(revealSpan(opened, groups, spans, "catalog.index_item-7")).toBe(
      opened
    )
    expect(revealSpan(opened, noGroups, spans, "catalog.index_item-7")).toBe(
      opened
    )
  })
})
