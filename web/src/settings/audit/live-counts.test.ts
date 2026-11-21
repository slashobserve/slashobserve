import { describe, expect, test } from "vitest"

import { liveFacetValues } from "./live-counts"
import type { AuditEvent } from "./stream"

const live = [
  { outcome: "success", origin: "web" },
  { outcome: "success", origin: "cli" },
  { outcome: "denied", origin: null },
] as unknown as AuditEvent[]

describe("liveFacetValues", () => {
  test("adds the streamed events of each value and keeps the server order", () => {
    expect(
      liveFacetValues(
        [
          { value: "denied", count: 5 },
          { value: "success", count: 3 },
          { value: "failed", count: 1 },
        ],
        live,
        "outcome"
      )
    ).toEqual([
      { value: "denied", count: 6 },
      { value: "success", count: 5 },
      { value: "failed", count: 1 },
    ])
  })

  test("skips an event that has no value for the field", () => {
    expect(
      liveFacetValues([{ value: "web", count: 1 }], live, "origin")
    ).toEqual([{ value: "web", count: 2 }])
  })

  test("returns the same counts when nothing was streamed", () => {
    const values = [{ value: "web", count: 1 }]
    expect(liveFacetValues(values, [], "origin")).toEqual(values)
  })
})
