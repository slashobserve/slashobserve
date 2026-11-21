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
        "outcome",
        8
      )
    ).toEqual([
      { value: "denied", count: 6 },
      { value: "success", count: 5 },
      { value: "failed", count: 1 },
    ])
  })

  test("skips an event that has no value for the field", () => {
    expect(
      liveFacetValues([{ value: "web", count: 1 }], live, "origin", 8)
    ).toEqual([
      { value: "web", count: 2 },
      { value: "cli", count: 1 },
    ])
  })

  test("returns the same counts when nothing was streamed", () => {
    const values = [{ value: "web", count: 1 }]
    expect(liveFacetValues(values, [], "origin", 8)).toEqual(values)
  })

  test("adds a value the list does not have yet, counted from the stream", () => {
    expect(
      liveFacetValues([{ value: "web", count: 4 }], live, "origin", 8)
    ).toEqual([
      { value: "web", count: 5 },
      { value: "cli", count: 1 },
    ])
  })

  test("sorts by count then value and keeps the cap", () => {
    const values = [
      { value: "b", count: 2 },
      { value: "c", count: 1 },
    ]
    const events = [
      { origin: "a" },
      { origin: "a" },
      { origin: "a" },
    ] as unknown as AuditEvent[]
    expect(liveFacetValues(values, events, "origin", 2)).toEqual([
      { value: "a", count: 3 },
      { value: "b", count: 2 },
    ])
  })
})
