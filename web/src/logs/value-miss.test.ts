import { describe, expect, it } from "vitest"

import { firstValueMiss, valueClauses, valueMissText } from "./value-miss"

describe("valueClauses", () => {
  it("takes every field and value of a KQL query", () => {
    expect(
      valueClauses("elasticsearch", "service.name:checkout and log.level:error")
    ).toEqual([
      { field: "service.name", value: "checkout" },
      { field: "log.level", value: "error" },
    ])
  })

  it("has nothing to say for a query the builder cannot show or another language", () => {
    expect(valueClauses("elasticsearch", "a:1 or b:2")).toEqual([])
    expect(valueClauses("loki", '{service_name="a"}')).toEqual([])
    expect(valueClauses("elasticsearch", undefined)).toEqual([])
  })
})

describe("firstValueMiss", () => {
  const clauses = valueClauses(
    "elasticsearch",
    "service.name:checkout and log.level:error"
  )

  it("finds the clause that misses after one that matches", () => {
    const seen = new Map([
      ["service.name", { values: ["checkout", "frontend"], complete: true }],
      ["log.level", { values: ["INFO", "ERROR"], complete: true }],
    ])
    expect(firstValueMiss(clauses, seen)).toContain(
      "No document has log.level = error in this range, but it has ERROR"
    )
  })

  it("does not claim a miss from a truncated list of values", () => {
    const seen = new Map([
      ["service.name", { values: ["a", "b"], complete: false }],
      ["log.level", { values: ["INFO", "ERROR"], complete: true }],
    ])
    expect(firstValueMiss(clauses, seen)).toContain("log.level = error")
    expect(firstValueMiss(clauses.slice(0, 1), seen)).toBeNull()
  })
})

describe("valueMissText", () => {
  const clause = { field: "log.level", value: "error" }

  it("lists the values seen when none matches", () => {
    expect(valueMissText({ ...clause, value: "fatal" }, ["INFO", "WARN"])).toBe(
      "No document has log.level = fatal in this range; values seen: INFO, WARN."
    )
  })

  it("says the values are case sensitive when only the letters differ", () => {
    expect(valueMissText(clause, ["INFO", "ERROR"])).toBe(
      "No document has log.level = error in this range, but it has ERROR; values are case sensitive, so seen: INFO, ERROR."
    )
  })

  it("stays silent when the value exists or nothing was seen", () => {
    expect(valueMissText(clause, ["error", "INFO"])).toBeNull()
    expect(valueMissText(clause, [])).toBeNull()
  })
})
