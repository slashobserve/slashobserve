import { beforeEach, describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { clickhouseLogsTemplate } from "./context-query"
import {
  barPercent,
  basisNote,
  facetBasis,
  facetCountText,
  facetsErrorText,
  fieldRows,
  fieldsNote,
  filterQuery,
  kindBadge,
  splitFields,
  storedFacetsOpen,
  storeFacetsOpen,
  type FacetField,
  type LogFacets,
} from "./facets"

function field(overrides: Partial<FacetField> = {}): FacetField {
  return {
    name: "level",
    kind: "keyword",
    exact: true,
    truncated: false,
    values: [
      { value: "error", count: 200 },
      { value: "info", count: 50 },
    ],
    ...overrides,
  }
}

function facets(overrides: Partial<LogFacets> = {}): LogFacets {
  return {
    sampled: false,
    lines_sampled: 0,
    fields: [],
    truncated: false,
    ...overrides,
  }
}

describe("facet counts", () => {
  it("groups thousands and reads Loki index bytes as a size", () => {
    expect(facetCountText(field(), { value: "a", count: 1234 })).toBe("1,234")
    expect(
      facetCountText(field({ unit: "bytes" }), { value: "a", count: 1572864 })
    ).toBe("1.5 MiB")
  })

  it("scales the bar to the field's top count", () => {
    const level = field()
    expect(barPercent(level, level.values[0])).toBe(100)
    expect(barPercent(level, level.values[1])).toBe(25)
    const empty = field({ values: [{ value: "x", count: 0 }] })
    expect(barPercent(empty, empty.values[0])).toBe(0)
  })

  it.each([
    ["exact", facets(), field(), "exact", "Counted over every matching line."],
    [
      "bytes",
      facets(),
      field({ unit: "bytes" }),
      "exact",
      "Counted in bytes from the stream index.",
    ],
    [
      "sampled",
      facets({ sampled: true, lines_sampled: 1000 }),
      field({ exact: false }),
      "sampled",
      "Counted over the newest 1,000 lines.",
    ],
    [
      "approximate",
      facets(),
      field({ exact: false }),
      "approximate",
      "The counts are approximate.",
    ],
  ] as const)("says how a %s field was counted", (_, all, one, basis, note) => {
    expect(facetBasis(all, one)).toBe(basis)
    expect(basisNote(all, one)).toBe(note)
  })
})

describe("field list", () => {
  it.each([
    ["elasticsearch", "logs-*", 31, "From the logs-* mapping, 31 fields"],
    ["opensearch", undefined, 1, "From the index mapping, 1 field"],
    ["loki", undefined, 4, "From the stream labels, 4 fields"],
    ["clickhouse", undefined, 12, "From the table columns, 12 fields"],
  ])("notes where the %s fields come from", (type, index, count, expected) => {
    expect(fieldsNote(type, index, count)).toBe(expected)
  })

  it("names the kinds with a badge", () => {
    expect(kindBadge("keyword")).toBe("k")
    expect(kindBadge("number")).toBe("#")
    expect(kindBadge("ip")).toBe("ip")
    expect(kindBadge("metadata")).toBe("m")
    expect(kindBadge("something-new")).toBe("?")
  })

  it("lists mapped names and counted fields once, by name", () => {
    const counted = field({ name: "level" })
    const rows = fieldRows(["service", "level", "@timestamp"], [counted])
    expect(rows.map((row) => row.name)).toEqual([
      "@timestamp",
      "level",
      "service",
    ])
    expect(rows[1].facet).toBe(counted)
    expect(rows[0].facet).toBeNull()
    expect(
      fieldRows([], [field({ name: "b" }), field({ name: "a" })]).map(
        (row) => row.name
      )
    ).toEqual(["a", "b"])
  })

  it("puts the chosen columns first in their order and filters both lists", () => {
    const rows = fieldRows(["a", "b", "http.route", "http.method"], [])
    const all = splitFields(rows, ["http.route", "gone"], "")
    expect(all.selected.map((row) => row.name)).toEqual(["http.route", "gone"])
    expect(all.available.map((row) => row.name)).toEqual([
      "a",
      "b",
      "http.method",
    ])
    const narrowed = splitFields(rows, ["http.route", "gone"], " HTTP ")
    expect(narrowed.selected.map((row) => row.name)).toEqual(["http.route"])
    expect(narrowed.available.map((row) => row.name)).toEqual(["http.method"])
  })

  it("reads a Loki-style query that is not a log query as needing no retry", () => {
    const invalid = new ApiError(422, "query_invalid", "metric query")
    expect(facetsErrorText(invalid)).toEqual({
      text: "Fields need a log query.",
      retry: false,
    })
    expect(facetsErrorText(new ApiError(500, "internal", "boom")).retry).toBe(
      true
    )
  })
})

describe("filterQuery", () => {
  it.each([
    ["in", "", 'level:"error"'],
    ["in", "service.name:checkout", 'service.name:checkout and level:"error"'],
    [
      "out",
      "service.name:checkout",
      'service.name:checkout and not level:"error"',
    ],
    ["in", "a:1 or b:2", '(a:1 or b:2) and level:"error"'],
    ["in", "kql: a:1", 'a:1 and level:"error"'],
  ] as const)("appends a %s clause to KQL %j", (mode, query, expected) => {
    expect(filterQuery("elasticsearch", query, "level", "error", mode)).toBe(
      expected
    )
  })

  it("escapes a quote in a KQL value", () => {
    expect(filterQuery("opensearch", "a:1", "message", 'say "hi"', "in")).toBe(
      'a:1 and message:"say \\"hi\\""'
    )
  })

  it.each([
    ["Lucene", "lucene: level:error"],
    ["Query DSL", '{"query":{"match_all":{}}}'],
    ["a repeat", 'level:"error"'],
  ])("leaves %s queries alone", (_, query) => {
    expect(
      filterQuery("elasticsearch", query, "level", "error", "in")
    ).toBeNull()
  })

  it.each([
    ["in", '{service_name="checkout"} | level="error"'],
    ["out", '{service_name="checkout"} | level!="error"'],
  ] as const)(
    "adds a Loki label filter stage to filter %s",
    (mode, expected) => {
      expect(
        filterQuery("loki", '{service_name="checkout"}', "level", "error", mode)
      ).toBe(expected)
    }
  )

  it("quotes Loki values and refuses metric queries, bad names and repeats", () => {
    expect(filterQuery("loki", '{a="b"} |= "x"', "msg", 'a "b"\\c', "in")).toBe(
      '{a="b"} |= "x" | msg="a \\"b\\"\\\\c"'
    )
    expect(
      filterQuery("loki", 'rate({a="b"}[5m])', "level", "error", "in")
    ).toBeNull()
    expect(filterQuery("loki", '{a="b"}', "http.route", "/", "in")).toBeNull()
    expect(
      filterQuery("loki", '{a="b"} | level="error"', "level", "error", "in")
    ).toBeNull()
  })

  it("adds a ClickHouse condition to the WHERE clause before ORDER BY", () => {
    expect(
      filterQuery(
        "clickhouse",
        clickhouseLogsTemplate,
        "ServiceName",
        "checkout",
        "in"
      )
    ).toBe(
      clickhouseLogsTemplate.replace(
        "WHERE $__timeFilter(Timestamp)",
        "WHERE ($__timeFilter(Timestamp)) AND ServiceName = 'checkout'"
      )
    )
  })

  it("reads LogAttributes keys as map lookups and escapes single quotes", () => {
    expect(
      filterQuery(
        "clickhouse",
        "SELECT * FROM otel_logs WHERE a = 1 OR b = 2",
        "LogAttributes.http.route",
        "it's",
        "out"
      )
    ).toBe(
      "SELECT * FROM otel_logs WHERE (a = 1 OR b = 2) AND LogAttributes['http.route'] != 'it\\'s'"
    )
  })

  it.each([
    ["no WHERE", "SELECT * FROM otel_logs LIMIT 10"],
    [
      "a subquery",
      "SELECT * FROM (SELECT * FROM otel_logs WHERE a = 1) WHERE b = 2",
    ],
    ["a repeat", "SELECT * FROM t WHERE ServiceName = 'checkout'"],
  ])("refuses a ClickHouse query with %s", (_, query) => {
    expect(
      filterQuery("clickhouse", query, "ServiceName", "checkout", "in")
    ).toBeNull()
  })

  it("refuses a column name that is not an identifier", () => {
    expect(
      filterQuery("clickhouse", "SELECT 1 WHERE 1", "a b", "x", "in")
    ).toBeNull()
  })

  it("refuses a datasource type that is not a log store", () => {
    expect(filterQuery("prometheus", "up", "a", "b", "in")).toBeNull()
  })
})

describe("facets open flag", () => {
  beforeEach(() => localStorage.clear())

  it("opens by default and remembers the choice", () => {
    expect(storedFacetsOpen()).toBe(true)
    storeFacetsOpen(false)
    expect(storedFacetsOpen()).toBe(false)
    storeFacetsOpen(true)
    expect(storedFacetsOpen()).toBe(true)
  })
})
