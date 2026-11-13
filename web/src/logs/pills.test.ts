// @vitest-environment node
import { describe, expect, it } from "vitest"

import {
  addPill,
  applyPills,
  decodePills,
  encodePills,
  lineFilter,
  maxPills,
  pillLabel,
  pillOptions,
  pillSupported,
  removePill,
  replacePill,
  unpinnedCleared,
  type NewPill,
  type Pill,
} from "./pills"

function pill(next: Partial<Pill> & Pick<Pill, "kind" | "field">): Pill {
  return {
    id: "0",
    negate: false,
    pinned: false,
    paused: false,
    ...next,
  }
}

const everyKind: Pill[] = [
  pill({ id: "0", kind: "is", field: "service.name", value: "checkout" }),
  pill({
    id: "1",
    kind: "is",
    field: "host.name",
    value: "canary-*",
    negate: true,
    pinned: true,
  }),
  pill({
    id: "2",
    kind: "oneOf",
    field: "log.level",
    values: ["error", "warn"],
    paused: true,
  }),
  pill({ id: "3", kind: "range", field: "status", op: ">=", value: "500" }),
  pill({ id: "4", kind: "exists", field: "retry", negate: true }),
  pill({ id: "5", kind: "is", field: "message", value: "café ☃" }),
]

const labelCases: [Pill, string][] = [
  [everyKind[0], "service.name: checkout"],
  [everyKind[1], "NOT host.name: canary-*"],
  [everyKind[2], "log.level: error, warn"],
  [everyKind[3], "status >= 500"],
  [pill({ kind: "exists", field: "retry" }), "retry: exists"],
]

describe("pill encoding", () => {
  it("round trips every kind with its flags", () => {
    const encoded = encodePills(everyKind)
    expect(encoded).toMatch(/^[A-Za-z0-9_-]+$/)
    expect(decodePills(encoded)).toEqual(everyKind)
  })

  it("encodes no pills as undefined", () => {
    expect(encodePills([])).toBeUndefined()
  })

  it("decodes missing or broken text to no pills", () => {
    for (const text of [undefined, "", "!!!", "e30", "bm90LWpzb24"]) {
      expect(decodePills(text)).toEqual([])
    }
  })

  it("drops a bad entry and keeps the good ones", () => {
    const text = JSON.stringify([
      ["is", "a", "1", 0],
      ["nope", "b", "2", 0],
      ["is", "", "3", 0],
      ["is", "c", 4, 0],
      ["oneOf", "d", [], 0],
      ["range", "e", "5", 0, "!="],
      ["is", "f", "6", 99],
      "garbage",
      ["exists", "g", null, 2],
    ])
    const encoded = btoa(text)
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "")
    expect(decodePills(encoded).map((item) => item.field)).toEqual(["a", "g"])
  })

  it("cuts a list over the limit", () => {
    const many = Array.from({ length: maxPills + 5 }, (_, index) => [
      "is",
      `f${index}`,
      "v",
      0,
    ])
    const encoded = btoa(JSON.stringify(many))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "")
    expect(decodePills(encoded)).toHaveLength(maxPills)
  })

  it("ignores a negate flag on a range", () => {
    const encoded = encodePills([
      pill({ kind: "range", field: "n", op: "<", value: "3", negate: true }),
    ])
    expect(decodePills(encoded)[0].negate).toBe(false)
  })
})

describe("pillLabel", () => {
  it.each(labelCases)("labels %j", (item, label) => {
    expect(pillLabel(item)).toBe(label)
  })
})

describe("addPill", () => {
  const base: NewPill = {
    kind: "is",
    field: "service.name",
    value: "checkout",
    negate: false,
    pinned: false,
    paused: false,
  }

  it("adds a new pill with a fresh id", () => {
    const next = addPill(everyKind.slice(1, 3), base)
    expect(next).toHaveLength(3)
    expect(next[2]).toMatchObject({ id: "2", field: "service.name" })
  })

  it("does not add the same clause twice", () => {
    const once = addPill([], base)
    expect(addPill(once, base)).toEqual(once)
  })

  it("adds the negated twin as a different pill", () => {
    const once = addPill([], base)
    expect(addPill(once, { ...base, negate: true })).toHaveLength(2)
  })

  it("resumes a paused twin instead of adding another", () => {
    const paused = addPill([], { ...base, paused: true })
    const resumed = addPill(paused, base)
    expect(resumed).toHaveLength(1)
    expect(resumed[0].paused).toBe(false)
  })

  it("stops at the limit", () => {
    const full = Array.from({ length: maxPills }, (_, index) =>
      pill({ id: String(index), kind: "is", field: `f${index}`, value: "v" })
    )
    expect(addPill(full, base)).toHaveLength(maxPills)
  })
})

describe("replacePill and removePill", () => {
  it("replaces one pill by several and renumbers", () => {
    const next = replacePill(everyKind.slice(0, 3), "1", [
      {
        kind: "range",
        field: "n",
        op: ">=",
        value: "1",
        negate: false,
        pinned: false,
        paused: false,
      },
      {
        kind: "range",
        field: "n",
        op: "<=",
        value: "9",
        negate: false,
        pinned: false,
        paused: false,
      },
    ])
    expect(next.map((item) => item.field)).toEqual([
      "service.name",
      "log.level",
      "n",
      "n",
    ])
    expect(next.map((item) => item.id)).toEqual(["0", "1", "2", "3"])
  })

  it("removes a pill and renumbers", () => {
    const next = removePill(everyKind.slice(0, 3), "0")
    expect(next.map((item) => item.id)).toEqual(["0", "1"])
    expect(next[0].field).toBe("host.name")
  })
})

describe("unpinnedCleared", () => {
  it("keeps only pinned pills", () => {
    const kept = unpinnedCleared(everyKind)
    expect(kept.map((item) => item.field)).toEqual(["host.name"])
    expect(kept[0].id).toBe("0")
  })
})

describe("applyPills on the search family", () => {
  const pills = [
    pill({ kind: "is", field: "service.name", value: "checkout" }),
    pill({ kind: "is", field: "host.name", value: "canary-*", negate: true }),
    pill({ kind: "oneOf", field: "log.level", values: ["error", "warn"] }),
    pill({ kind: "exists", field: "retry" }),
    pill({ kind: "range", field: "status", op: ">=", value: "500" }),
  ]

  it("appends each active pill in order", () => {
    expect(applyPills("timeout", "elasticsearch", pills)).toBe(
      'timeout and service.name:"checkout" and not host.name:"canary-*" and log.level:("error" or "warn") and retry:* and status >= 500'
    )
  })

  it("wraps a top-level or", () => {
    expect(applyPills("a or b", "opensearch", pills.slice(0, 1))).toBe(
      '(a or b) and service.name:"checkout"'
    )
  })

  it("starts from nothing when the query is empty", () => {
    expect(applyPills("", "elasticsearch", pills.slice(0, 1))).toBe(
      'service.name:"checkout"'
    )
  })

  it("skips paused pills", () => {
    const paused = pills.map((item) => ({ ...item, paused: true }))
    paused[0].paused = false
    expect(applyPills("timeout", "elasticsearch", paused)).toBe(
      'timeout and service.name:"checkout"'
    )
  })

  it("returns the query untouched when no pill is active", () => {
    expect(
      applyPills("lucene: a AND b", "elasticsearch", [
        { ...pills[0], paused: true },
      ])
    ).toBe("lucene: a AND b")
  })

  it("keeps a kql prefix", () => {
    expect(applyPills("kql: timeout", "elasticsearch", pills.slice(0, 1))).toBe(
      'kql: timeout and service.name:"checkout"'
    )
  })

  it("cannot narrow a Lucene or Query DSL query", () => {
    expect(
      applyPills("lucene: a AND b", "elasticsearch", pills.slice(0, 1))
    ).toBeNull()
    expect(
      applyPills('{"query":{}}', "elasticsearch", pills.slice(0, 1))
    ).toBeNull()
  })
})

describe("applyPills on Loki and ClickHouse", () => {
  const sql = `SELECT Timestamp, Body
FROM otel_logs
WHERE $__timeFilter(Timestamp)
ORDER BY Timestamp DESC
LIMIT 1000`

  it("puts an is pill in the Loki selector", () => {
    expect(
      applyPills('{service_name="checkout"}', "loki", [
        pill({ kind: "is", field: "pod", value: "checkout-1", negate: true }),
      ])
    ).toBe('{service_name="checkout", pod!="checkout-1"}')
  })

  it("puts a per-line label after the selector", () => {
    expect(
      applyPills('{service_name="checkout"}', "loki", [
        pill({ kind: "is", field: "trace_id", value: "abc" }),
      ])
    ).toBe('{service_name="checkout"} | trace_id="abc"')
  })

  it("adds an is pill to the WHERE clause", () => {
    expect(
      applyPills(sql, "clickhouse", [
        pill({ kind: "is", field: "ServiceName", value: "checkout" }),
      ])
    ).toContain("AND ServiceName = 'checkout'")
  })

  it("leaves the query alone for kinds it cannot express", () => {
    const range = pill({ kind: "range", field: "n", op: ">", value: "1" })
    const exists = pill({ kind: "exists", field: "n" })
    const oneOf = pill({ kind: "oneOf", field: "n", values: ["a", "b"] })
    expect(
      applyPills('{service_name="checkout"}', "loki", [range, exists, oneOf])
    ).toBe('{service_name="checkout"}')
    expect(pillSupported("loki", range)).toBe(false)
    expect(pillSupported("clickhouse", oneOf)).toBe(false)
    expect(pillSupported("loki", pill({ kind: "is", field: "a" }))).toBe(true)
  })

  it("skips a pill the query cannot take", () => {
    expect(
      applyPills("not a selector", "loki", [
        pill({ kind: "is", field: "pod", value: "x" }),
      ])
    ).toBe("not a selector")
  })

  it("cannot narrow an unknown datasource type", () => {
    expect(
      applyPills("up", "prometheus", [pill({ kind: "is", field: "a" })])
    ).toBeNull()
  })
})

describe("lineFilter", () => {
  const loki = { type: "loki", query: '{a="b"}', stream: false }
  const search = (query: string) => ({
    type: "elasticsearch",
    query,
    stream: false,
  })
  const added = (result: ReturnType<typeof lineFilter>) =>
    "pills" in result ? result.pills : []

  it("adds a per-line Loki field as a stage after the selector", () => {
    const pills = added(lineFilter(loki, [], "in", "status", "500"))
    expect(pills).toMatchObject([{ kind: "is", field: "status", line: true }])
    expect(applyPills('{a="b"}', "loki", pills)).toBe('{a="b"} | status="500"')
  })

  it("adds a Loki stream label to the selector", () => {
    const pills = added(
      lineFilter({ ...loki, stream: true }, [], "out", "pod", "x")
    )
    expect(applyPills('{a="b"}', "loki", pills)).toBe('{a="b", pod!="x"}')
  })

  it("keeps the stage flag through the address", () => {
    const pills = added(lineFilter(loki, [], "in", "status", "500"))
    expect(decodePills(encodePills(pills))).toEqual(pills)
  })

  it("adds an include, an exclude and an exists pill on a KQL query", () => {
    const query = search("level:error")
    const first = added(lineFilter(query, [], "in", "http.status", "503"))
    const second = added(lineFilter(query, first, "out", "level", "debug"))
    const third = added(lineFilter(query, second, "exists", "user.id", ""))
    expect(third).toMatchObject([
      { kind: "is", field: "http.status", value: "503", negate: false },
      { kind: "is", field: "level", value: "debug", negate: true },
      { kind: "exists", field: "user.id", negate: false },
    ])
  })

  it.each([
    [
      "a pill already there",
      loki,
      [pill({ kind: "is", field: "status", value: "500", line: true })],
      "in",
      "500",
      "Already a filter",
    ],
    ["a long value", loki, [], "in", "x".repeat(201), "Too long to filter"],
    ["exists on Loki", loki, [], "exists", "", "Cannot edit this query"],
    [
      "a Lucene query",
      search("lucene: a:1"),
      [],
      "in",
      "2",
      "Switch to KQL to filter from a line",
    ],
    [
      "a Query DSL query",
      search('{"query":{"match_all":{}}}'),
      [],
      "in",
      "2",
      "Switch to KQL to filter from a line",
    ],
    [
      "a full list",
      loki,
      Array.from({ length: maxPills }, (_, index) =>
        pill({ id: String(index), kind: "is", field: `f${index}`, value: "v" })
      ),
      "in",
      "500",
      "Too many filters",
    ],
  ] as const)("blocks %s", (_label, context, pills, action, value, blocked) => {
    expect(lineFilter(context, [...pills], action, "status", value)).toEqual({
      blocked,
    })
  })
})

describe("pillOptions", () => {
  it("offers every kind on the search family and is on the others", () => {
    expect(pillOptions("elasticsearch").kinds).toEqual([
      "is",
      "oneOf",
      "range",
      "exists",
    ])
    expect(pillOptions("loki").kinds).toEqual(["is"])
    expect(pillOptions("clickhouse").kinds).toEqual(["is"])
  })
})
