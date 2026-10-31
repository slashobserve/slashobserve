import { describe, expect, it } from "vitest"

import {
  compareValues,
  describeFilter,
  formatFilter,
  fromSpecFilter,
  keyHint,
  labelsBySavedFilter,
  listedFilterProblem,
  maxAdhocFilters,
  opsFor,
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
    for (const op of ["=", "!=", "=~", "!~", "<", ">"]) {
      expect(parseFilter(`k|${op}|v`)?.op).toBe(op)
    }
    expect(parseFilter("k|~|v")).toBeNull()
    expect(parseFilter("k||v")).toBeNull()
  })

  it("reads a leading pipe as the or condition", () => {
    expect(parseFilter("|job|=|api")).toEqual({
      key: "job",
      op: "=",
      value: "api",
      condition: "or",
    })
  })

  it("keeps a key named or apart from the condition", () => {
    expect(parseFilter("or|=|x")).toEqual({ key: "or", op: "=", value: "x" })
  })

  it("refuses text without two separators or without a key", () => {
    expect(parseFilter("")).toBeNull()
    expect(parseFilter("job")).toBeNull()
    expect(parseFilter("job|=")).toBeNull()
    expect(parseFilter("|=|v")).toBeNull()
    expect(parseFilter("||=|v")).toBeNull()
    expect(parseFilter("|job|=")).toBeNull()
  })
})

describe("formatFilter", () => {
  it("round-trips with parseFilter", () => {
    for (const text of [
      "job|=|api",
      "a|!~|x|y",
      "k|=~|",
      "n|<|5",
      "n|>|1.5",
      "|n|>|1",
      "|job|=|a|b",
    ]) {
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

  it("shows labels in place of the key and the value without changing the text that goes in the URL", () => {
    const filter = { key: "job", op: "=" as const, value: "api" }
    expect(describeFilter(filter, { keyLabel: "Job", valueLabel: "API" })).toBe(
      "Job = API"
    )
    expect(describeFilter(filter, { keyLabel: "Job" })).toBe("Job = api")
    expect(describeFilter(filter, { valueLabel: "API" })).toBe("job = API")
    expect(formatFilter(filter)).toBe("job|=|api")
  })
})

describe("saved filters from the Spec", () => {
  const saved = [
    { key: "job", operator: "=" as const, value: "api", keyLabel: "Job" },
    {
      key: "n",
      operator: ">" as const,
      value: "1",
      valueLabel: "one",
      condition: "or" as const,
    },
    {
      key: "plain",
      operator: "!=" as const,
      value: "x",
      condition: "and" as const,
    },
  ]

  it("maps a Spec filter to a chip filter, writing only the or condition", () => {
    expect(fromSpecFilter(saved[0])).toEqual({
      key: "job",
      op: "=",
      value: "api",
    })
    expect(fromSpecFilter(saved[1])).toEqual({
      key: "n",
      op: ">",
      value: "1",
      condition: "or",
    })
    expect(fromSpecFilter(saved[2])).toEqual({
      key: "plain",
      op: "!=",
      value: "x",
    })
  })

  it("finds the labels of a filter by the text the URL carries", () => {
    const labels = labelsBySavedFilter(saved)
    expect(labels.get("job|=|api")).toEqual({
      keyLabel: "Job",
      valueLabel: undefined,
    })
    expect(labels.get("|n|>|1")).toEqual({
      keyLabel: undefined,
      valueLabel: "one",
    })
    expect(labels.has("plain|!=|x")).toBe(false)
    expect(labelsBySavedFilter(undefined).size).toBe(0)
  })
})

describe("compareValues", () => {
  it.each([
    ["2", "10", -1],
    ["10", "2", 1],
    ["5", "5.0", 0],
    ["-3", "2", -1],
    ["1e2", "99", 1],
    ["abc", "abd", -1],
    ["b", "a", 1],
    ["a", "a", 0],
    ["2", "10 apples", 1],
    ["10", "2x", -1],
  ])("compares %s with %s as %i", (cell, value, want) => {
    expect(compareValues(cell, value)).toBe(want)
  })
})

describe("parseFilters", () => {
  it("drops invalid filters and keeps order", () => {
    expect(parseFilters(["b|=|1", "nope", "a|!=|2"])).toEqual([
      { key: "b", op: "=", value: "1" },
      { key: "a", op: "!=", value: "2" },
    ])
  })

  it("keeps ordering operators and the or condition in order", () => {
    expect(parseFilters(["a|<|1", "|b|>|2", "c|=|3"])).toEqual([
      { key: "a", op: "<", value: "1" },
      { key: "b", op: ">", value: "2", condition: "or" },
      { key: "c", op: "=", value: "3" },
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

  it("offers < and > on ClickHouse only", () => {
    expect(opsFor("clickhouse")).toContain("<")
    expect(opsFor("clickhouse")).toContain(">")
    expect(opsFor("prometheus")).toEqual(["=", "!=", "=~", "!~"])
    expect(opsFor(undefined)).toEqual(["=", "!=", "=~", "!~"])
    expect(validFilter({ key: "n", op: "<", value: "1" }, "clickhouse")).toBe(
      true
    )
    expect(validFilter({ key: "n", op: "<", value: "1" }, "prometheus")).toBe(
      false
    )
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

describe("listedFilterProblem", () => {
  const keys = ["job", "instance"]
  const values = ["api", "web"]

  it("accepts a listed key with a listed value", () => {
    expect(
      listedFilterProblem({ key: "job", value: "api" }, keys, values)
    ).toBe(null)
  })

  it("blames the key when it is not listed, even if the value is", () => {
    expect(
      listedFilterProblem({ key: "pod", value: "api" }, keys, values)
    ).toBe("key")
  })

  it("blames the value when only the key is listed", () => {
    expect(listedFilterProblem({ key: "job", value: "db" }, keys, values)).toBe(
      "value"
    )
  })

  it("blames the key first when neither is listed", () => {
    expect(listedFilterProblem({ key: "pod", value: "db" }, keys, values)).toBe(
      "key"
    )
  })

  it("treats lists that are still loading as not listing anything", () => {
    expect(
      listedFilterProblem({ key: "job", value: "api" }, undefined, values)
    ).toBe("key")
    expect(
      listedFilterProblem({ key: "job", value: "api" }, keys, undefined)
    ).toBe("value")
    expect(
      listedFilterProblem({ key: "job", value: "api" }, undefined, undefined)
    ).toBe("key")
  })

  it("matches text exactly", () => {
    expect(
      listedFilterProblem({ key: "Job", value: "api" }, keys, values)
    ).toBe("key")
  })
})
