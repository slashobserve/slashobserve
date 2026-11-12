import { describe, expect, it } from "vitest"

import {
  isSearchFamily,
  logsTimeRange,
  queryModeLabel,
  validAsk,
  validateLogsSearch,
} from "./search"

describe("validateLogsSearch", () => {
  it("keeps a valid search", () => {
    const search = {
      ds: "loki",
      q: '{service_name="checkout"}',
      ask: "errors from checkout",
      from: "now-6h",
      to: "1760000000000",
    }
    expect(validateLogsSearch(search)).toEqual(search)
  })

  it("lists every key so a dropped one overrides the raw URL", () => {
    expect(validateLogsSearch({})).toEqual({
      ds: undefined,
      q: undefined,
      ask: undefined,
      from: undefined,
      to: undefined,
      f: undefined,
      cols: undefined,
      line: undefined,
      view: undefined,
      tail: undefined,
      observation: undefined,
    })
  })

  it("keeps an observation id", () => {
    const observation = "8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5b"
    expect(validateLogsSearch({ observation }).observation).toBe(observation)
  })

  it.each([
    ["is empty", ""],
    ["is not a uuid", "checkout-slow"],
    ["has a path in it", "../8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5b"],
    ["is a uuid with a suffix", "8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5bz"],
  ])("drops an observation that %s", (_, observation) => {
    expect(validateLogsSearch({ observation }).observation).toBeUndefined()
  })

  it("keeps the pills, columns, open line, view and tail keys", () => {
    const search = {
      f: "W3sia2V5IjoiYSJ9XQ",
      cols: "service,level",
      line: "1760000000000000000:a1B2_c-3",
      view: "patterns" as const,
      tail: "1" as const,
    }
    expect(validateLogsSearch(search)).toMatchObject(search)
  })

  it.each([
    ["is empty", ""],
    ["holds a plus sign", "ab+c"],
    ["holds padding", "abc="],
    ["is over 4000 characters", "a".repeat(4001)],
  ])("drops pills that %s", (_, f) => {
    expect(validateLogsSearch({ f }).f).toBeUndefined()
  })

  it("keeps pills of 4000 characters", () => {
    const f = "a".repeat(4000)
    expect(validateLogsSearch({ f }).f).toBe(f)
  })

  it.each([
    ["is empty", ""],
    ["has an empty name", "a,,b"],
    ["ends in a comma", "a,"],
    ["has 21 names", Array.from({ length: 21 }, (_, i) => `c${i}`).join(",")],
    ["has a name over 256 characters", "x".repeat(257)],
  ])("drops columns that %s", (_, cols) => {
    expect(validateLogsSearch({ cols }).cols).toBeUndefined()
  })

  it("keeps 20 columns and a name of 256 characters", () => {
    const twenty = Array.from({ length: 20 }, (_, i) => `c${i}`).join(",")
    expect(validateLogsSearch({ cols: twenty }).cols).toBe(twenty)
    const long = "x".repeat(256)
    expect(validateLogsSearch({ cols: long }).cols).toBe(long)
  })

  it.each([
    ["has no hash", "1760000000000000000"],
    ["has a letter in the time", "17600a:abc"],
    ["has an empty hash", "1760000000000000000:"],
    ["has a hash over 64 characters", `1:${"a".repeat(65)}`],
    ["has a time over 20 digits", `${"1".repeat(21)}:abc`],
    ["has a space in the hash", "1:a b"],
  ])("drops an open line that %s", (_, line) => {
    expect(validateLogsSearch({ line }).line).toBeUndefined()
  })

  it.each([
    ["view", { view: "table" }],
    ["tail", { tail: "0" }],
    ["tail", { tail: "true" }],
  ])("drops a bad %s value", (_, raw) => {
    const search = validateLogsSearch(raw)
    expect(search.view).toBeUndefined()
    expect(search.tail).toBeUndefined()
  })

  it.each([
    ["starts with a dash", "-loki"],
    ["has a slash", "lo/ki"],
    ["is too long", "a".repeat(41)],
  ])("drops a datasource uid that %s", (_, ds) => {
    expect(validateLogsSearch({ ds }).ds).toBeUndefined()
  })

  it.each([
    ["is empty", ""],
    ["is over 8000 characters", "x".repeat(8001)],
    ["holds a NUL", '{a="b"}\0'],
  ])("drops a query that %s", (_, q) => {
    expect(validateLogsSearch({ q }).q).toBeUndefined()
  })

  it.each([
    ["is empty", ""],
    ["is over 2000 characters", "x".repeat(2001)],
    ["holds a newline", "errors\nfrom checkout"],
    ["holds a NUL", "errors\0"],
  ])("drops a question that %s", (_, ask) => {
    expect(validateLogsSearch({ ask }).ask).toBeUndefined()
  })

  it("keeps a question of 2000 characters", () => {
    const ask = "x".repeat(2000)
    expect(validAsk(ask)).toBe(ask)
  })

  it("keeps the first of repeated values", () => {
    expect(validateLogsSearch({ ds: ["a", "b"] }).ds).toBe("a")
  })

  it.each([
    ["only from", { from: "now-1h" }],
    ["only to", { to: "now" }],
    ["an unreadable from", { from: "yesterday", to: "now" }],
  ])("drops both bounds for %s", (_, raw) => {
    const search = validateLogsSearch(raw)
    expect(search.from).toBeUndefined()
    expect(search.to).toBeUndefined()
  })
})

describe("logsTimeRange", () => {
  it("defaults to the last hour", () => {
    expect(logsTimeRange({})).toEqual({ from: "now-1h", to: "now" })
  })

  it("uses the range in the search", () => {
    expect(logsTimeRange({ from: "now-2d", to: "now-1d" })).toEqual({
      from: "now-2d",
      to: "now-1d",
    })
  })
})

describe("queryModeLabel", () => {
  it.each([
    ["elasticsearch", "KQL"],
    ["opensearch", "KQL"],
    ["loki", "LogQL"],
    ["clickhouse", "SQL"],
  ])("names the code mode of %s", (type, label) => {
    expect(queryModeLabel(type)).toBe(label)
  })
})

describe("isSearchFamily", () => {
  it("is true for Elasticsearch and OpenSearch only", () => {
    expect(isSearchFamily("elasticsearch")).toBe(true)
    expect(isSearchFamily("opensearch")).toBe(true)
    expect(isSearchFamily("loki")).toBe(false)
    expect(isSearchFamily("clickhouse")).toBe(false)
  })
})
