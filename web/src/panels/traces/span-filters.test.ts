import { describe, expect, it } from "vitest"

import { criticalPath, filterSpans, spanFilters } from "./span-filters"
import type { Span } from "./trace-model"

function span(
  id: string,
  parentId: string | null,
  start: number,
  duration: number,
  extra: Partial<Span> = {}
): Span {
  return {
    id,
    parentId,
    service: "web",
    operation: id,
    start,
    duration,
    offset: 0,
    width: 0,
    depth: 0,
    slot: 0,
    error: false,
    hasChildren: false,
    attributes: [],
    ...extra,
  }
}

const trace = (): Span[] => [
  span("root", null, 0, 100, { hasChildren: true }),
  span("auth", "root", 0, 20, {
    depth: 1,
    service: "auth",
    attributes: [{ key: "http.status", text: "401" }],
  }),
  span("db", "root", 10, 40, {
    depth: 1,
    service: "db",
    hasChildren: true,
  }),
  span("query", "db", 15, 30, { depth: 2, service: "db" }),
  span("cache", "root", 55, 40, { depth: 1, service: "cache" }),
]

const none = spanFilters({})

function idsOf(spans: readonly Span[]) {
  return spans.map((item) => item.id)
}

describe("spanFilters", () => {
  it("reads every display key and ignores values of the wrong type", () => {
    expect(
      spanFilters({
        spanSearch: "  Checkout ",
        serviceNames: ["web", 3, "db"],
        excludeServices: true,
        spanNames: ["GET"],
        excludeSpanNames: true,
        minSpanDuration: 10,
        maxSpanDuration: 1500,
        matchesOnly: true,
        criticalPathOnly: true,
      })
    ).toEqual({
      search: "Checkout",
      serviceNames: ["web", "db"],
      excludeServices: true,
      spanNames: ["GET"],
      excludeSpanNames: true,
      minDuration: 10,
      maxDuration: 1500,
      matchesOnly: true,
      criticalPathOnly: true,
    })
    expect(
      spanFilters({
        spanSearch: 4,
        serviceNames: "web",
        minSpanDuration: "10",
        matchesOnly: "yes",
      })
    ).toEqual(none)
  })
})

describe("criticalPath", () => {
  it("follows the last finishing child, then the one that ended before it began", () => {
    expect([...criticalPath(trace())].sort()).toEqual([
      "cache",
      "db",
      "query",
      "root",
    ])
  })

  it("leaves out a child that overlaps a later finishing sibling", () => {
    const spans = [
      span("root", null, 0, 100, { hasChildren: true }),
      span("early", "root", 0, 60, { depth: 1 }),
      span("late", "root", 30, 70, { depth: 1 }),
    ]
    expect([...criticalPath(spans)].sort()).toEqual(["late", "root"])
  })

  it("steps back to the child that finished before a later one started", () => {
    const spans = [
      span("root", null, 0, 100, { hasChildren: true }),
      span("first", "root", 0, 40, { depth: 1 }),
      span("second", "root", 50, 50, { depth: 1 }),
      span("short", "root", 5, 10, { depth: 1 }),
    ]
    expect([...criticalPath(spans)].sort()).toEqual(["first", "root", "second"])
  })

  it("clamps a child that ends after its parent", () => {
    const spans = [
      span("root", null, 0, 50, { hasChildren: true }),
      span("skewed", "root", 10, 80, { depth: 1 }),
    ]
    expect([...criticalPath(spans)].sort()).toEqual(["root", "skewed"])
  })

  it("ignores a child that starts after its parent ended", () => {
    const spans = [
      span("root", null, 0, 50, { hasChildren: true }),
      span("late", "root", 60, 10, { depth: 1 }),
    ]
    expect([...criticalPath(spans)]).toEqual(["root"])
  })

  it("covers a trace with several roots and an empty trace", () => {
    const spans = [span("a", null, 0, 10), span("b", null, 20, 10)]
    expect([...criticalPath(spans)].sort()).toEqual(["a", "b"])
    expect(criticalPath([]).size).toBe(0)
  })

  it("walks a very deep chain without recursion", () => {
    const spans = Array.from({ length: 5000 }, (_, index) =>
      span(`s${index}`, index === 0 ? null : `s${index - 1}`, 0, 10, {
        depth: index,
      })
    )
    expect(criticalPath(spans).size).toBe(5000)
  })
})

describe("filterSpans", () => {
  it("shows every span and no note when no filter is set", () => {
    const spans = trace()
    const result = filterSpans(spans, none)
    expect(result.spans).toBe(spans)
    expect(result.matched).toBeNull()
    expect(result.notes).toEqual([])
  })

  it("matches the search text in services, operations, ids and attributes in any case", () => {
    const find = (search: string) =>
      [
        ...(filterSpans(trace(), { ...none, search }).matched ?? new Set()),
      ].sort()
    expect(find("AUTH")).toEqual(["auth"])
    expect(find("http.status")).toEqual(["auth"])
    expect(find("401")).toEqual(["auth"])
    expect(find("que")).toEqual(["query"])
    expect(find("nothing like this")).toEqual([])
  })

  it("matches or excludes listed services and operations exactly", () => {
    const matched = (filters: Partial<typeof none>) =>
      [...(filterSpans(trace(), { ...none, ...filters }).matched ?? [])].sort()
    expect(matched({ serviceNames: ["db"] })).toEqual(["db", "query"])
    expect(matched({ serviceNames: ["db"], excludeServices: true })).toEqual([
      "auth",
      "cache",
      "root",
    ])
    expect(matched({ spanNames: ["auth", "cache"] })).toEqual(["auth", "cache"])
    expect(matched({ spanNames: ["au"] })).toEqual([])
    expect(
      matched({ spanNames: ["root"], excludeSpanNames: true })
    ).toHaveLength(4)
  })

  it("keeps the spans inside the duration bounds, ends included", () => {
    const matched = (min: number | null, max: number | null) =>
      [
        ...(filterSpans(trace(), {
          ...none,
          minDuration: min,
          maxDuration: max,
        }).matched ?? []),
      ].sort()
    expect(matched(30, null)).toEqual(["cache", "db", "query", "root"])
    expect(matched(null, 30)).toEqual(["auth", "query"])
    expect(matched(20, 30)).toEqual(["auth", "query"])
    expect(matched(0, 0)).toEqual([])
  })

  it("requires every filter at once", () => {
    const result = filterSpans(trace(), {
      ...none,
      serviceNames: ["db"],
      minDuration: 35,
    })
    expect([...(result.matched ?? [])]).toEqual(["db"])
  })

  it("keeps the other spans, with the matches marked, unless matches only is on", () => {
    const filters = { ...none, serviceNames: ["db"] }
    const kept = filterSpans(trace(), filters)
    expect(idsOf(kept.spans)).toEqual(["root", "auth", "db", "query", "cache"])
    expect(kept.notes).toEqual(["2 of 5 spans match the filters."])
    const only = filterSpans(trace(), { ...filters, matchesOnly: true })
    expect(idsOf(only.spans)).toEqual(["db", "query"])
  })

  it("ignores matches only when nothing is filtered", () => {
    const spans = trace()
    expect(filterSpans(spans, { ...none, matchesOnly: true }).spans).toBe(spans)
  })

  it("shows nothing and says so when no span matches", () => {
    const result = filterSpans(trace(), {
      ...none,
      search: "absent",
      matchesOnly: true,
    })
    expect(result.spans).toEqual([])
    expect(result.notes).toEqual(["0 of 5 spans match the filters."])
  })

  it("shows the critical path only, then narrows it to the matches", () => {
    const spans = [
      span("root", null, 0, 100, { hasChildren: true }),
      span("early", "root", 0, 60, { depth: 1 }),
      span("late", "root", 30, 70, { depth: 1 }),
    ]
    const path = filterSpans(spans, { ...none, criticalPathOnly: true })
    expect(idsOf(path.spans)).toEqual(["root", "late"])
    expect(path.notes).toEqual(["Showing the 2 spans on the critical path."])
    const both = filterSpans(spans, {
      ...none,
      criticalPathOnly: true,
      matchesOnly: true,
      search: "late",
    })
    expect(idsOf(both.spans)).toEqual(["late"])
  })

  it("recomputes which spans have children from what is shown", () => {
    const result = filterSpans(trace(), {
      ...none,
      matchesOnly: true,
      serviceNames: ["db", "web"],
      spanNames: ["db", "root", "auth"],
    })
    expect(idsOf(result.spans)).toEqual(["root", "db"])
    expect(result.spans.map((item) => item.hasChildren)).toEqual([true, false])
  })
})
