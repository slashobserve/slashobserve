import { describe, expect, it } from "vitest"

import {
  MAX_IMPORT_BYTES,
  formatBytes,
  requestBytes,
  summaryLine,
} from "./read-source"

const summary = {
  format: "classic" as const,
  title: "t",
  uid: "u",
  schema_version: 39,
  bytes: 412_000,
  panels: 31,
  rows: 6,
  variables: 4,
}

describe("requestBytes", () => {
  it("counts UTF-8 bytes of the whole request body", () => {
    const ascii = requestBytes("abc", {})
    expect(ascii).toBe(JSON.stringify({ json: "abc", mapping: {} }).length)
    expect(requestBytes("é", {}) - requestBytes("e", {})).toBe(1)
    expect(requestBytes("日", {}) - requestBytes("e", {})).toBe(2)
  })

  it("includes the mapping", () => {
    expect(requestBytes("x", { a: "b" })).toBeGreaterThan(requestBytes("x", {}))
  })

  it("limits to 8 MiB", () => {
    expect(MAX_IMPORT_BYTES).toBe(8_388_608)
  })
})

describe("formatBytes", () => {
  it("uses KB below a million bytes, at least 1", () => {
    expect(formatBytes(0)).toBe("1 KB")
    expect(formatBytes(412_000)).toBe("412 KB")
    expect(formatBytes(999_999)).toBe("1000 KB")
  })

  it("uses one decimal MB from a million bytes", () => {
    expect(formatBytes(1_000_000)).toBe("1.0 MB")
    expect(formatBytes(2_450_000)).toBe("2.5 MB")
  })
})

describe("summaryLine", () => {
  it("joins size, schema, panels with rows and variables", () => {
    expect(summaryLine(summary)).toBe(
      "412 KB · Grafana schema 39 · 31 panels in 6 rows · 4 variables"
    )
  })

  it("uses singular nouns", () => {
    expect(summaryLine({ ...summary, panels: 1, rows: 1, variables: 1 })).toBe(
      "412 KB · Grafana schema 39 · 1 panel in 1 row · 1 variable"
    )
  })

  it("omits rows and schema when there are none", () => {
    expect(summaryLine({ ...summary, rows: 0, schema_version: 0 })).toBe(
      "412 KB · 31 panels · 4 variables"
    )
  })
})
