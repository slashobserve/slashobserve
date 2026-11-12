import { describe, expect, it } from "vitest"

import { touchesObservation, type ActivityEvent } from "./activity-match"

const observation = {
  id: "o1",
  thread_id: "t1",
  blocks: [{ run_id: null }, { run_id: "r1" }],
}

function event(kind: ActivityEvent["target"]["kind"], id: string) {
  return {
    seq: 1,
    type: "observation.updated",
    target: { kind, id },
    dashboard_id: null,
    folder_id: null,
    actor: {} as ActivityEvent["actor"],
    data: {},
    created_at: "2026-11-12T10:00:00Z",
  } satisfies ActivityEvent
}

describe("touchesObservation", () => {
  it.each([
    ["the observation itself", event("observation", "o1"), "observation"],
    ["a run that is a block of the page", event("run", "r1"), "observation"],
    ["the observation's thread", event("thread", "t1"), "discussion"],
    ["another observation", event("observation", "o2"), null],
    ["an unrelated thread", event("thread", "t2"), null],
    ["a run that is not on the page", event("run", "r2"), null],
    ["a dashboard with the same id", event("dashboard", "o1"), null],
  ])("matches %s", (_, activity, expected) => {
    expect(touchesObservation(activity, observation)).toBe(expected)
  })

  it("ignores threads while the observation has no discussion yet", () => {
    expect(
      touchesObservation(event("thread", "t1"), {
        id: "o1",
        thread_id: null,
        blocks: undefined,
      })
    ).toBeNull()
  })

  it("ignores runs when the blocks are not loaded", () => {
    expect(
      touchesObservation(event("run", "r1"), {
        id: "o1",
        thread_id: null,
        blocks: undefined,
      })
    ).toBeNull()
  })
})
