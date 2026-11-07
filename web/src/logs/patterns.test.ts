import { describe, expect, it } from "vitest"

import {
  patternParts,
  patternRegex,
  patternShare,
  patternsFooter,
  patternsHeading,
  shareText,
  smallPatterns,
  sortPatterns,
  trendChange,
  withPatternFilter,
  type LogPattern,
  type LogPatterns,
} from "./patterns"

function pattern(overrides: Partial<LogPattern>): LogPattern {
  return {
    pattern: "a",
    count: 1,
    sample: "",
    first: "",
    last: "",
    trend: [],
    level: "",
    ...overrides,
  }
}

function result(overrides: Partial<LogPatterns>): LogPatterns {
  return {
    source: "server",
    lines_sampled: 0,
    lines_matched: 0,
    patterns: [],
    truncated: false,
    ...overrides,
  }
}

function buckets(counts: number[]) {
  return counts.map((count) => ({ start: "2026-11-07T10:00:00Z", count }))
}

describe("patternParts", () => {
  it("splits a template into literal and wildcard parts", () => {
    expect(patternParts("a <*> b")).toEqual([
      { text: "a ", wildcard: false },
      { text: "<*>", wildcard: true },
      { text: " b", wildcard: false },
    ])
  })

  it("keeps a pattern without wildcards as one part", () => {
    expect(patternParts("plain text")).toEqual([
      { text: "plain text", wildcard: false },
    ])
  })
})

describe("patternRegex", () => {
  it.each([
    ["GET /a.b took <*>ms", "GET /a\\.b took .*?ms"],
    ["call f(<*>)", "call f\\(.*?\\)"],
    ["ids [<*>] done", "ids \\[.*?\\] done"],
    ["a|b+c", "a\\|b\\+c"],
  ])("turns %s into %s", (input, expected) => {
    expect(patternRegex(input)).toBe(expected)
  })
})

describe("withPatternFilter on Loki", () => {
  const query = '{service_name="checkout"} | json'

  it("appends a regex line filter after the pipeline to keep the lines", () => {
    expect(
      withPatternFilter("loki", query, "paid order <*>", "in", "loki")
    ).toBe('{service_name="checkout"} | json |~ "paid order .*?"')
  })

  it("appends a negated regex line filter to drop the lines", () => {
    expect(
      withPatternFilter("loki", query, "paid order <*>", "out", "loki")
    ).toBe('{service_name="checkout"} | json !~ "paid order .*?"')
  })

  it("escapes a backslash and a quote for the LogQL string", () => {
    expect(
      withPatternFilter("loki", '{a="b"}', 'say "hi" (<*>)', "in", "loki")
    ).toBe('{a="b"} |~ "say \\"hi\\" \\\\(.*?\\\\)"')
  })

  it.each([
    ["a filter already in the query", '{a="b"} |~ "x .*?"', "x <*>"],
    ["a query that is not a stream selector", 'rate({a="b"}[1m])', "x <*>"],
    ["a pattern that is only a wildcard", '{a="b"}', "<*>"],
  ])("returns null for %s", (_name, text, input) => {
    expect(withPatternFilter("loki", text, input, "in", "loki")).toBeNull()
  })
})

describe("withPatternFilter on ClickHouse", () => {
  it("inserts match before ORDER BY and keeps an OR condition grouped", () => {
    expect(
      withPatternFilter(
        "clickhouse",
        "SELECT Timestamp, Body FROM otel_logs WHERE a = 1 OR b = 2 ORDER BY Timestamp DESC",
        "it's <*> done",
        "out",
        "server"
      )
    ).toBe(
      "SELECT Timestamp, Body FROM otel_logs WHERE (a = 1 OR b = 2) AND NOT match(Body, 'it\\'s .*? done') ORDER BY Timestamp DESC"
    )
  })

  it("adds the positive match for in", () => {
    expect(
      withPatternFilter(
        "clickhouse",
        "SELECT Body FROM otel_logs WHERE a = 1",
        "paid <*>",
        "in",
        "server"
      )
    ).toBe(
      "SELECT Body FROM otel_logs WHERE (a = 1) AND match(Body, 'paid .*?')"
    )
  })

  it("returns null without a WHERE", () => {
    expect(
      withPatternFilter(
        "clickhouse",
        "SELECT Body FROM otel_logs",
        "paid <*>",
        "in",
        "server"
      )
    ).toBeNull()
  })
})

describe("withPatternFilter on the search family", () => {
  it("requires every literal part of a template to filter in", () => {
    expect(
      withPatternFilter(
        "opensearch",
        "level:error",
        "paid order <*> for <*>",
        "in",
        "server"
      )
    ).toBe('level:error and message:"paid order" and message:"for"')
  })

  it("drops only lines that carry all parts to filter out", () => {
    expect(
      withPatternFilter(
        "opensearch",
        "level:error",
        "paid order <*> for <*>",
        "out",
        "server"
      )
    ).toBe('level:error and not (message:"paid order" and message:"for")')
  })

  it("negates a single part directly and groups an OR query", () => {
    expect(
      withPatternFilter(
        "elasticsearch",
        "a:1 or b:2",
        "refused",
        "out",
        "elasticsearch"
      )
    ).toBe('(a:1 or b:2) and not message:"refused"')
  })

  it("matches each token of a categorize_text key", () => {
    expect(
      withPatternFilter(
        "elasticsearch",
        "",
        "connection refused",
        "in",
        "elasticsearch"
      )
    ).toBe('message:"connection" and message:"refused"')
  })

  it("keeps the query form prefix", () => {
    expect(
      withPatternFilter("elasticsearch", "kql: a:1", "refused", "in", "server")
    ).toBe('kql: a:1 and message:"refused"')
  })

  it.each([
    ["a Lucene query", "lucene: a:1"],
    ["a DSL query", '{"query":{"match_all":{}}}'],
    ["a clause already present", 'message:"refused"'],
  ])("returns null for %s", (_name, text) => {
    expect(
      withPatternFilter("elasticsearch", text, "refused", "in", "server")
    ).toBeNull()
  })

  it("returns null when no part carries a word", () => {
    expect(
      withPatternFilter("elasticsearch", "", "<*> - <*>", "in", "server")
    ).toBeNull()
  })
})

describe("trendChange", () => {
  it.each([
    [
      "growth",
      [...Array(10).fill(1), ...Array(10).fill(0), ...Array(10).fill(4.4)],
      "+340%",
    ],
    [
      "decline",
      [...Array(10).fill(10), ...Array(10).fill(0), ...Array(10).fill(5)],
      "-50%",
    ],
    ["a new pattern", [...Array(20).fill(0), ...Array(10).fill(2)], "new"],
    ["a flat pattern", Array(30).fill(3), "0%"],
    ["a silent pattern", Array(30).fill(0), null],
    ["no buckets", [], null],
  ])("labels %s", (_name, counts, expected) => {
    expect(trendChange(buckets(counts))).toBe(expected)
  })
})

describe("patternsHeading", () => {
  it("counts the share of the sampled lines for a server result", () => {
    expect(
      patternsHeading(
        result({
          source: "server",
          lines_sampled: 1000,
          lines_matched: 980,
          patterns: [pattern({ count: 600 }), pattern({ count: 380 })],
        })
      )
    ).toBe(
      "2 patterns cover 98% of 1,000 documents. Numbers and ids become <*>."
    )
  })

  it("counts the share of the matched lines when the index answered", () => {
    expect(
      patternsHeading(
        result({
          source: "loki",
          lines_matched: 200,
          patterns: [pattern({ count: 100 })],
        })
      )
    ).toBe("1 pattern covers 50% of 200 documents. Numbers and ids become <*>.")
  })

  it("never rounds a partial cover up to 100%", () => {
    expect(
      patternsHeading(
        result({
          lines_sampled: 1000,
          patterns: [pattern({ count: 999 })],
        })
      )
    ).toContain("99%")
  })
})

describe("shares", () => {
  it.each([
    [0.004, "0.4%"],
    [0.0951, "9.5%"],
    [0.1, "10%"],
    [0.987, "99%"],
  ])("writes %s as %s", (share, text) => {
    expect(shareText(share)).toBe(text)
  })

  it("is zero without a total", () => {
    expect(patternShare(5, 0)).toBe(0)
  })
})

describe("smallPatterns", () => {
  const counts = [500, 100, 80, 70, 60, 50, 40, 30, 20, 15, 8, 4, 3, 2, 1, 1, 1]
  const all = counts.map((count, index) =>
    pattern({ pattern: `p${index}`, count })
  )
  const total = counts.reduce((sum, count) => sum + count, 0)

  it("folds only the tail under 1% after the first nine", () => {
    const { shown, folded } = smallPatterns(all, total)
    expect(shown.map((item) => item.pattern)).toEqual(
      all.slice(0, 10).map((item) => item.pattern)
    )
    expect(folded).toHaveLength(7)
  })

  it("keeps the first nine even when each is under 1%", () => {
    const flat = Array.from({ length: 12 }, (_, index) =>
      pattern({ pattern: `q${index}`, count: 1 })
    )
    const { shown, folded } = smallPatterns(flat, 10000)
    expect(shown).toHaveLength(9)
    expect(folded).toHaveLength(3)
  })

  it("words the footer", () => {
    expect(patternsFooter(9, 14, 5)).toBe(
      "9 of 14 patterns shown, 5 cover under 1% each"
    )
  })
})

describe("sortPatterns", () => {
  const older = pattern({
    pattern: "older",
    count: 9,
    last: "2026-11-07T09:00:00Z",
  })
  const newer = pattern({
    pattern: "newer",
    count: 2,
    last: "2026-11-07T10:00:00Z",
  })
  const unknown = pattern({ pattern: "unknown", count: 5, last: "" })

  it("keeps the served order for most lines first", () => {
    expect(
      sortPatterns([older, unknown, newer], "count").map((p) => p.pattern)
    ).toEqual(["older", "unknown", "newer"])
  })

  it("puts the newest last-seen first and unknown times last", () => {
    expect(
      sortPatterns([older, unknown, newer], "newest").map((p) => p.pattern)
    ).toEqual(["newer", "older", "unknown"])
  })
})
