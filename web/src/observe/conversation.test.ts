import { describe, expect, it } from "vitest"

import {
  joinNames,
  lowerFirst,
  orderTurns,
  shortRequestId,
  stepSummary,
} from "./conversation"

describe("orderTurns", () => {
  it("is just the root while revisions are unknown", () => {
    expect(orderTurns("root", undefined)).toEqual(["root"])
  })

  it("sorts by creation time", () => {
    expect(
      orderTurns("root", [
        { id: "c", created_at: "2026-01-03T00:00:00Z" },
        { id: "root", created_at: "2026-01-01T00:00:00Z" },
        { id: "b", created_at: "2026-01-02T00:00:00Z" },
      ])
    ).toEqual(["root", "b", "c"])
  })

  it("breaks equal timestamps by id", () => {
    expect(
      orderTurns("root", [
        { id: "z", created_at: "2026-01-02T00:00:00Z" },
        { id: "a", created_at: "2026-01-02T00:00:00Z" },
        { id: "root", created_at: "2026-01-01T00:00:00Z" },
      ])
    ).toEqual(["root", "a", "z"])
  })

  it("prepends the root when the list lacks it", () => {
    expect(
      orderTurns("root", [{ id: "a", created_at: "2026-01-02T00:00:00Z" }])
    ).toEqual(["root", "a"])
    expect(orderTurns("root", [])).toEqual(["root"])
  })

  it("includes the root once", () => {
    expect(
      orderTurns("root", [
        { id: "root", created_at: "2026-01-01T00:00:00Z" },
        { id: "root", created_at: "2026-01-01T00:00:00Z" },
      ])
    ).toEqual(["root"])
  })
})

describe("stepSummary", () => {
  it("reports an empty list", () => {
    expect(stepSummary([])).toBe("No steps recorded")
  })

  it("counts steps with singular and plural", () => {
    expect(stepSummary([{ phase: "query" }])).toBe("Ran 1 step")
    expect(stepSummary([{ phase: "query" }, { phase: "save" }])).toBe(
      "Ran 2 steps"
    )
  })

  it("adds repaired queries", () => {
    expect(stepSummary([{ phase: "query" }, { phase: "repair" }])).toBe(
      "Ran 2 steps, repaired 1 query"
    )
    expect(
      stepSummary([{ phase: "repair" }, { phase: "repair" }, { phase: "save" }])
    ).toBe("Ran 3 steps, repaired 2 queries")
  })
})

describe("shortRequestId", () => {
  it("keeps the first eight characters", () => {
    expect(shortRequestId("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")).toBe(
      "0190a1b2"
    )
    expect(shortRequestId("abc")).toBe("abc")
  })
})

describe("joinNames", () => {
  it("joins zero to many names", () => {
    expect(joinNames([])).toBe("")
    expect(joinNames(["a"])).toBe("a")
    expect(joinNames(["a", "b"])).toBe("a and b")
    expect(joinNames(["a", "b", "c"])).toBe("a, b and c")
    expect(joinNames(["a", "b", "c", "d"])).toBe("a, b, c and d")
  })
})

describe("lowerFirst", () => {
  it("lowercases the first character only", () => {
    expect(lowerFirst("Yesterday")).toBe("yesterday")
    expect(lowerFirst("In 74 days")).toBe("in 74 days")
    expect(lowerFirst("")).toBe("")
  })
})
