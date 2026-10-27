import { describe, expect, it } from "vitest"

import {
  describeFilter,
  formatFilter,
  keyHint,
  maxAdhocFilters,
  parseFilter,
  parseFilters,
  validFilter,
  validKey,
} from "./adhoc"

describe("parseFilter", () => {
  it("splits key, operator and value", () => {
    expect(parseFilter("job|=|prometheus")).toEqual({
      key: "job",
      op: "=",
      value: "prometheus",
    })
  })

  it("keeps pipes inside the value", () => {
    expect(parseFilter("job|=~|a|b|c")).toEqual({
      key: "job",
      op: "=~",
      value: "a|b|c",
    })
  })

  it("accepts an empty value", () => {
    expect(parseFilter("job|!=|")).toEqual({ key: "job", op: "!=", value: "" })
  })

  it("accepts every operator and refuses anything else", () => {
    for (const op of ["=", "!=", "=~", "!~"]) {
      expect(parseFilter(`k|${op}|v`)?.op).toBe(op)
    }
    expect(parseFilter("k|>|v")).toBeNull()
    expect(parseFilter("k||v")).toBeNull()
  })

  it("refuses text without two separators or without a key", () => {
    expect(parseFilter("")).toBeNull()
    expect(parseFilter("job")).toBeNull()
    expect(parseFilter("job|=")).toBeNull()
    expect(parseFilter("|=|v")).toBeNull()
  })
})

describe("formatFilter", () => {
  it("round-trips with parseFilter", () => {
    for (const text of ["job|=|api", "a|!~|x|y", "k|=~|"]) {
      const filter = parseFilter(text)
      expect(filter).not.toBeNull()
      expect(formatFilter(filter!)).toBe(text)
    }
  })

  it("describes a filter for people", () => {
    expect(describeFilter({ key: "job", op: "!=", value: "api" })).toBe(
      "job != api"
    )
  })
})

describe("parseFilters", () => {
  it("drops invalid filters and keeps order", () => {
    expect(parseFilters(["b|=|1", "nope", "a|!=|2"])).toEqual([
      { key: "b", op: "=", value: "1" },
      { key: "a", op: "!=", value: "2" },
    ])
  })

  it("keeps at most 20", () => {
    const many = Array.from({ length: 30 }, (_, i) => `k${i}|=|v`)
    expect(parseFilters(many)).toHaveLength(maxAdhocFilters)
  })
})

describe("validFilter", () => {
  const filter = (key: string) => ({ key, op: "=" as const, value: "v" })

  it("takes only Prometheus label names for Prometheus", () => {
    expect(validFilter(filter("job"), "prometheus")).toBe(true)
    expect(validFilter(filter("_x1"), "prometheus")).toBe(true)
    expect(validFilter(filter("http.status"), "prometheus")).toBe(false)
    expect(validFilter(filter("1job"), "prometheus")).toBe(false)
    expect(validFilter(filter("my-label"), "prometheus")).toBe(false)
    expect(validFilter(filter(""), "prometheus")).toBe(false)
  })

  it("takes dotted column names for ClickHouse", () => {
    expect(
      validFilter(filter("ResourceAttributes.service"), "clickhouse")
    ).toBe(true)
    expect(validFilter(filter("a..b"), "clickhouse")).toBe(false)
  })

  it("treats an unknown datasource type like Prometheus", () => {
    expect(validKey("a.b", undefined)).toBe(false)
    expect(keyHint(undefined)).toBe("Use a label name.")
    expect(keyHint("clickhouse")).toBe("Use a column name.")
  })
})
