import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"
import { addPill, encodePills } from "@/logs/pills"

import {
  defaultTitle,
  logsDraft,
  matchesSaved,
  metricsDraft,
  noSaveFolderText,
  saveBlockedText,
  saveTargets,
  tracesDraft,
  type SearchDraft,
} from "./draft"
import { must } from "@/test/must"

const range = { from: "now-6h", to: "now" }

function folder(id: string, extra: Partial<Folder> = {}, create = true) {
  return {
    id,
    title: id,
    general: false,
    personal: false,
    permissions: { comment: true, edit: create, manage: create, create },
    ...extra,
  } as Folder
}

const pills = must(
  encodePills(
    addPill([], {
      kind: "is",
      field: "service",
      value: "checkout",
      negate: false,
      pinned: false,
      paused: false,
    })
  )
)

describe("defaultTitle", () => {
  it("is the first 80 characters of the query on one line", () => {
    const draft = {
      kind: "logs",
      datasource_uid: "es",
      query: `${"a".repeat(100)}\nb`,
    } as SearchDraft
    expect(defaultTitle(draft)).toBe("a".repeat(80))
    expect(defaultTitle({ ...draft, query: "level:error\n\tand  x" })).toBe(
      "level:error and x"
    )
  })

  it("cuts by characters, not code units", () => {
    const draft = {
      kind: "logs",
      datasource_uid: "es",
      query: "\u{1F600}".repeat(90),
    } as SearchDraft
    expect([...defaultTitle(draft)]).toHaveLength(80)
  })

  it("names a trace search without a query by its service and span", () => {
    const draft = {
      kind: "traces",
      datasource_uid: "tempo",
      filters: { service: "checkout", span: "POST /pay" },
    } as SearchDraft
    expect(defaultTitle(draft)).toBe("checkout POST /pay")
  })

  it("is empty when there is nothing to name it by", () => {
    const draft = { kind: "traces", datasource_uid: "tempo" } as SearchDraft
    expect(defaultTitle(draft)).toBe("")
  })
})

describe("saveTargets", () => {
  it("offers only folders where the caller can create", () => {
    const targets = saveTargets([folder("a"), folder("b", {}, false)])
    expect(targets.choices.map((choice) => choice.id)).toEqual(["a"])
  })

  it("starts on the personal folder, then General, then the first", () => {
    const personal = folder("mine", { personal: true })
    const general = folder("general", { general: true })
    const other = folder("other")
    expect(saveTargets([other, general, personal]).initial).toBe("mine")
    expect(saveTargets([other, general]).initial).toBe("general")
    expect(saveTargets([other]).initial).toBe("other")
    expect(saveTargets([]).initial).toBeNull()
  })
})

describe("saveBlockedText", () => {
  it("says why when no folder takes a new search", () => {
    expect(saveBlockedText([folder("a", {}, false)])).toBe(noSaveFolderText)
    expect(saveBlockedText([])).toBe(noSaveFolderText)
  })

  it("stays quiet while folders load and when one takes a search", () => {
    expect(saveBlockedText(undefined)).toBeUndefined()
    expect(saveBlockedText([folder("a")])).toBeUndefined()
  })
})

describe("logsDraft", () => {
  it("has nothing to save before a query runs", () => {
    expect(
      logsDraft({ uid: "es", type: "elasticsearch", search: {}, range })
    ).toEqual({ blocked: "no-query" })
  })

  it("folds the filter pills into the query and keeps the columns", () => {
    const result = logsDraft({
      uid: "es",
      type: "elasticsearch",
      search: { q: "level:error", f: pills, cols: "host,message" },
      range,
    })
    expect(result).toEqual({
      draft: {
        kind: "logs",
        datasource_uid: "es",
        query: 'level:error and service:"checkout"',
        filters: { columns: ["host", "message"] },
        from: "now-6h",
        to: "now",
      },
    })
  })

  it("sends no filters when no column is chosen", () => {
    const result = logsDraft({
      uid: "loki",
      type: "loki",
      search: { q: '{app="shop"}' },
      range,
    })
    expect(result).toEqual({
      draft: {
        kind: "logs",
        datasource_uid: "loki",
        query: '{app="shop"}',
        filters: undefined,
        from: "now-6h",
        to: "now",
      },
    })
  })

  it("is blocked when the pills cannot be applied to the query", () => {
    expect(
      logsDraft({
        uid: "es",
        type: "elasticsearch",
        search: { q: "lucene: level:error", f: pills },
        range,
      })
    ).toEqual({ blocked: "pills" })
  })
})

describe("tracesDraft", () => {
  const search = {
    service: "checkout",
    span: "POST /pay",
    min: "250ms",
    max: "2s",
    errors: "1" as const,
    q: '{ resource.service.name = "checkout" }',
  }

  it("keeps the filters and the query", () => {
    expect(tracesDraft({ uid: "tempo", search, range })).toEqual({
      kind: "traces",
      datasource_uid: "tempo",
      query: search.q,
      filters: {
        service: "checkout",
        span: "POST /pay",
        min: "250ms",
        max: "2s",
        errors: true,
      },
      from: "now-6h",
      to: "now",
    })
  })

  it("keeps the SQL of the SQL view on ClickHouse", () => {
    const sql = "SELECT TraceId FROM otel_traces WHERE $__timeFilter(Timestamp)"
    const draft = tracesDraft({
      uid: "ch",
      search: { q: sql },
      range,
    })
    expect(draft.query).toBe(sql)
  })

  it("leaves the query out of a ClickHouse search of fields", () => {
    const draft = tracesDraft({
      uid: "ch",
      search: { service: "checkout" },
      range,
    })
    expect(JSON.parse(JSON.stringify(draft))).not.toHaveProperty("query")
  })

  it("leaves out errors when it is not set", () => {
    const draft = tracesDraft({
      uid: "tempo",
      search: { service: "checkout" },
      range,
    })
    expect(JSON.parse(JSON.stringify(draft)).filters).toEqual({
      service: "checkout",
    })
  })
})

describe("metricsDraft", () => {
  it("saves the query that ran with its range", () => {
    expect(metricsDraft({ uid: "prom", search: { q: "up" }, range })).toEqual({
      draft: {
        kind: "metrics",
        datasource_uid: "prom",
        query: "up",
        filters: undefined,
        from: "now-6h",
        to: "now",
      },
    })
  })

  it("keeps the table mode", () => {
    const saved = metricsDraft({
      uid: "prom",
      search: { q: "up", mode: "table" },
      range,
    })
    expect("draft" in saved && saved.draft.filters).toEqual({ mode: "table" })
  })

  it("is blocked until a query has run", () => {
    expect(metricsDraft({ uid: "prom", search: {}, range })).toEqual({
      blocked: "no-query",
    })
  })
})

describe("matchesSaved", () => {
  const saved = {
    query: "level:error",
    filters: { columns: ["host"] },
    from: "now-6h",
    to: "now",
  }
  const draft: SearchDraft = {
    kind: "logs",
    datasource_uid: "es",
    query: "level:error",
    filters: { columns: ["host"] },
    from: "now-6h",
    to: "now",
  }

  it("is true for the search as it was saved", () => {
    expect(matchesSaved(draft, saved)).toBe(true)
  })

  it("treats no columns the same whether the filters are missing or empty", () => {
    const bare = { ...draft }
    delete bare.filters
    expect(matchesSaved(bare, { ...saved, filters: {} })).toBe(true)
  })

  it.each([
    ["query", { query: "level:warn" }],
    ["range", { from: "now-1h" }],
    ["columns", { filters: { columns: ["host", "message"] } }],
    ["trace filters", { filters: { service: "checkout" } }],
    ["metrics mode", { filters: { mode: "table" as const } }],
  ])("is false when the %s changed", (_name, change) => {
    expect(matchesSaved({ ...draft, ...change }, saved)).toBe(false)
  })
})
