import { describe, expect, it } from "vitest"

import type { DatasourceStatus } from "./queries"
import {
  carriesSearch,
  defaultSource,
  listedSources,
  logsSwitchSearch,
  orderSources,
  sourceLanguage,
  sourceRows,
  sourcesOf,
  tracesSwitchSearch,
  withoutSource,
} from "./sources"

function source(uid: string, type: string, name = uid): DatasourceStatus {
  return {
    uid,
    type,
    name,
    healthy: true,
    checked_at: "2026-11-14T00:00:00Z",
    dashboard_count: 0,
  }
}

const uids = (list: readonly DatasourceStatus[]) => list.map((s) => s.uid)

describe("sourcesOf", () => {
  const list = [
    source("prom", "prometheus"),
    source("ch", "clickhouse"),
    source("loki", "loki"),
    source("tempo", "tempo"),
    source("es", "elasticsearch"),
    source("os", "opensearch"),
  ]

  it("keeps the log datasource types for logs", () => {
    expect(uids(sourcesOf("logs", list))).toEqual(["ch", "loki", "es", "os"])
  })

  it("keeps the trace datasource types for traces", () => {
    expect(uids(sourcesOf("traces", list))).toEqual(["ch", "tempo"])
  })

  it("returns nothing before the list loads", () => {
    expect(sourcesOf("logs", undefined)).toEqual([])
  })
})

describe("sourceLanguage", () => {
  it.each([
    ["logs", "clickhouse", "SQL"],
    ["logs", "loki", "LogQL"],
    ["logs", "elasticsearch", "KQL"],
    ["logs", "opensearch", "KQL"],
    ["traces", "tempo", "TraceQL"],
    ["traces", "clickhouse", "Search"],
    ["traces", "other", "Query"],
  ] as const)("names the %s language of %s as %s", (kind, type, language) => {
    expect(sourceLanguage(kind, type)).toBe(language)
  })
})

describe("orderSources", () => {
  const list = [
    source("b", "loki", "beta"),
    source("a", "loki", "Alpha"),
    source("c", "loki", "charlie"),
  ]

  it("sorts by name without regard to case when nothing was used", () => {
    expect(uids(orderSources(list, []))).toEqual(["a", "b", "c"])
  })

  it("puts recently used sources first in recent order", () => {
    expect(uids(orderSources(list, ["c", "b"]))).toEqual(["c", "b", "a"])
  })

  it("ignores recent uids that are not sources", () => {
    expect(uids(orderSources(list, ["gone", "b"]))).toEqual(["b", "a", "c"])
  })

  it("breaks name ties by uid", () => {
    const tied = [source("z", "loki", "Same"), source("y", "loki", "same")]
    expect(uids(orderSources(tied, []))).toEqual(["y", "z"])
  })

  it("slots a source that appeared by name and keeps the others in place", () => {
    const loaded = ["c", "b"]
    const appeared = [source("n", "loki", "Aardvark"), ...list]
    expect(uids(orderSources(list, loaded))).toEqual(["c", "b", "a"])
    expect(uids(orderSources(appeared, loaded))).toEqual(["c", "b", "n", "a"])
  })

  it("closes the gap of a source that disappeared without moving the others", () => {
    const loaded = ["c", "b"]
    const remaining = list.filter((candidate) => candidate.uid !== "b")
    expect(uids(orderSources(remaining, loaded))).toEqual(["c", "a"])
  })

  it("does not change its input", () => {
    const copy = [...list]
    orderSources(list, ["c"])
    expect(list).toEqual(copy)
  })
})

describe("defaultSource", () => {
  const ch = source("ch", "clickhouse", "ClickHouse")
  const tempo = source("tempo", "tempo", "Tempo")
  const alpha = source("alpha", "loki", "Alpha")

  it("opens the most recently used source that still exists", () => {
    expect(defaultSource("logs", [ch, alpha], ["gone", "ch"])?.uid).toBe("ch")
  })

  it("opens the first log source by name when none was used", () => {
    expect(defaultSource("logs", [ch, alpha], [])?.uid).toBe("alpha")
  })

  it("prefers Tempo for traces when none was used", () => {
    expect(defaultSource("traces", [ch, tempo], [])?.uid).toBe("tempo")
  })

  it("falls back to the first trace source by name without Tempo", () => {
    const other = source("ch2", "clickhouse", "Another")
    expect(defaultSource("traces", [ch, other], [])?.uid).toBe("ch2")
  })

  it("keeps a recently used ClickHouse over Tempo for traces", () => {
    expect(defaultSource("traces", [ch, tempo], ["ch"])?.uid).toBe("ch")
  })

  it("returns nothing for an empty list", () => {
    expect(defaultSource("logs", [], ["ch"])).toBeUndefined()
  })
})

describe("listedSources", () => {
  const many = (count: number) =>
    Array.from({ length: count }, (_, index) => source(`s${index}`, "loki"))

  it.each([0, 1])("lists nothing for %i sources", (count) => {
    expect(listedSources(many(count))).toEqual({ shown: [], more: false })
  })

  it("lists every source up to the cap", () => {
    const result = listedSources(many(5))
    expect(result.shown).toHaveLength(5)
    expect(result.more).toBe(false)
  })

  it("lists the first five and flags More above the cap", () => {
    const result = listedSources(many(7))
    expect(uids(result.shown)).toEqual(["s0", "s1", "s2", "s3", "s4"])
    expect(result.more).toBe(true)
  })
})

describe("logsSwitchSearch", () => {
  const current = {
    q: "error",
    ask: "errors",
    line: "1:a",
    cols: "a,b",
    page: 3,
    from: "now-6h",
    to: "now",
    f: "pills",
    view: "patterns",
    tail: "1",
    observation: "8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5b",
    size: 50,
  } as const

  it("clears the query, question, open line, columns and page", () => {
    const next = logsSwitchSearch(current, "loki", "clickhouse")
    expect(next.q).toBeUndefined()
    expect(next.ask).toBeUndefined()
    expect(next.line).toBeUndefined()
    expect(next.cols).toBeUndefined()
    expect(next.page).toBeUndefined()
  })

  it("keeps the range, pills, view, tail, observation and size", () => {
    expect(logsSwitchSearch(current, "loki", "clickhouse")).toMatchObject({
      from: "now-6h",
      to: "now",
      f: "pills",
      view: "patterns",
      tail: "1",
      observation: current.observation,
      size: 50,
    })
  })

  it.each([
    ["elasticsearch", "opensearch"],
    ["opensearch", "elasticsearch"],
  ])("keeps the query between %s and %s", (from, to) => {
    expect(logsSwitchSearch(current, from, to).q).toBe("error")
  })

  it("clears the query from a search source to a non-search one", () => {
    expect(logsSwitchSearch(current, "elasticsearch", "loki").q).toBeUndefined()
  })
})

describe("tracesSwitchSearch", () => {
  it("drops the question and the page and keeps the rest", () => {
    expect(
      tracesSwitchSearch({
        ask: "slow",
        page: 2,
        service: "checkout",
        from: "now-6h",
        to: "now",
      })
    ).toMatchObject({
      ask: undefined,
      page: undefined,
      service: "checkout",
      from: "now-6h",
      to: "now",
    })
  })
})

describe("sourceRows", () => {
  it("gives each source the path of its own page", () => {
    const list = [source("loki", "loki"), source("a/b", "clickhouse")]
    expect(sourceRows("logs", list)).toEqual([
      { id: "loki", paths: ["/logs/loki"] },
      { id: "a/b", paths: ["/logs/a%2Fb"] },
    ])
    expect(sourceRows("traces", list)).toEqual([
      { id: "loki", paths: ["/traces/loki"] },
      { id: "a/b", paths: ["/traces/a%2Fb"] },
    ])
  })
})

describe("withoutSource", () => {
  it("splits the uid from the rest of the search", () => {
    expect(withoutSource({ ds: "x", q: "a" })).toEqual({
      uid: "x",
      search: { q: "a" },
    })
  })

  it("has no uid without ds", () => {
    const search: { ds?: string; q: string } = { q: "a" }
    expect(withoutSource(search)).toEqual({
      uid: undefined,
      search: { q: "a" },
    })
  })
})

describe("carriesSearch", () => {
  it("is false for an empty search", () => {
    expect(carriesSearch({})).toBe(false)
  })

  it("is false when every key is undefined", () => {
    expect(carriesSearch({ q: undefined, page: undefined })).toBe(false)
  })

  it.each([{ observation: "x" }, { q: "a" }])("is true for %o", (search) => {
    expect(carriesSearch(search)).toBe(true)
  })
})
