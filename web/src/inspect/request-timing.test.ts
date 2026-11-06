import { describe, expect, it } from "vitest"

import { noTiming, timingReducer, type FetchEvent } from "./request-timing"

function replay(events: FetchEvent[]) {
  return events.reduce(timingReducer, noTiming)
}

describe("timingReducer", () => {
  it("records the time from fetch start to success", () => {
    expect(
      replay([
        { kind: "start", at: 100 },
        { kind: "success", at: 340 },
      ])
    ).toEqual({ startedAt: null, lastMs: 240, failed: false })
  })

  it("marks a fetch that ended in an error", () => {
    expect(
      replay([
        { kind: "start", at: 10 },
        { kind: "error", at: 60 },
      ])
    ).toEqual({ startedAt: null, lastMs: 50, failed: true })
  })

  it("keeps the last duration while the next fetch runs", () => {
    expect(
      replay([
        { kind: "start", at: 0 },
        { kind: "success", at: 80 },
        { kind: "start", at: 1000 },
      ])
    ).toEqual({ startedAt: 1000, lastMs: 80, failed: false })
  })

  it("ignores an end whose start it never saw", () => {
    expect(replay([{ kind: "success", at: 50 }])).toEqual(noTiming)
  })

  it("measures from the latest start when a fetch restarts", () => {
    expect(
      replay([
        { kind: "start", at: 0 },
        { kind: "start", at: 200 },
        { kind: "success", at: 260 },
      ])
    ).toEqual({ startedAt: null, lastMs: 60, failed: false })
  })
})
