import { describe, expect, it } from "vitest"

import {
  costText,
  joinNames,
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

  it("adds how long the run took after the count", () => {
    expect(stepSummary([{ phase: "query" }, { phase: "save" }], "1m 12s")).toBe(
      "Ran 2 steps in 1m 12s"
    )
    expect(stepSummary([{ phase: "repair" }], "8s")).toBe(
      "Ran 1 step in 8s, repaired 1 query"
    )
  })
})

describe("costText", () => {
  it("leaves the cost out when the model has no price", () => {
    expect(costText(null)).toBeUndefined()
    expect(costText(undefined)).toBeUndefined()
  })

  it("shows a free run as zero dollars", () => {
    expect(costText(0)).toBe("$0.00")
  })

  it("shows less than a cent as under a cent", () => {
    expect(costText(0.0042)).toBe("under $0.01")
    expect(costText(0.0099)).toBe("under $0.01")
  })

  it("rounds everything else to cents", () => {
    expect(costText(0.01)).toBe("about $0.01")
    expect(costText(0.1234)).toBe("about $0.12")
    expect(costText(12.5)).toBe("about $12.50")
  })
})

describe("stepSummary with a cost", () => {
  const steps = [{ phase: "query" }, { phase: "save" }]

  it("adds the cost after the duration", () => {
    expect(stepSummary(steps, "1m 12s", "about $0.12")).toBe(
      "Ran 2 steps in 1m 12s for about $0.12"
    )
  })

  it("adds the cost after the count when there is no duration", () => {
    expect(stepSummary(steps, undefined, "under $0.01")).toBe(
      "Ran 2 steps for under $0.01"
    )
  })

  it("keeps the repairs last", () => {
    expect(
      stepSummary([{ phase: "repair" }, { phase: "save" }], "8s", "$0.00")
    ).toBe("Ran 2 steps in 8s for $0.00, repaired 1 query")
  })

  it("is unchanged without a cost", () => {
    expect(stepSummary(steps, "8s", undefined)).toBe("Ran 2 steps in 8s")
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
