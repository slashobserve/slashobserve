import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"
import { addPill, encodePills } from "@/logs/pills"

import {
  defaultTitle,
  logsDraft,
  saveTargets,
  tracesDraft,
  type SearchDraft,
} from "./draft"

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

const pills = encodePills(
  addPill([], {
    kind: "is",
    field: "service",
    value: "checkout",
    negate: false,
    pinned: false,
    paused: false,
  })
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

  it("keeps the filters and the query on Tempo", () => {
    expect(tracesDraft({ uid: "tempo", type: "tempo", search, range })).toEqual(
      {
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
      }
    )
  })

  it("leaves the query out on ClickHouse, which ignores it", () => {
    const draft = tracesDraft({ uid: "ch", type: "clickhouse", search, range })
    expect(draft.query).toBeUndefined()
    expect(JSON.parse(JSON.stringify(draft))).not.toHaveProperty("query")
  })

  it("leaves out errors when it is not set", () => {
    const draft = tracesDraft({
      uid: "tempo",
      type: "tempo",
      search: { service: "checkout" },
      range,
    })
    expect(JSON.parse(JSON.stringify(draft)).filters).toEqual({
      service: "checkout",
    })
  })
})
