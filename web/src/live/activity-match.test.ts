import { describe, expect, it } from "vitest"

import {
  matchesActivityFilter,
  touchesObservation,
  type ActivityEvent,
} from "./activity-match"

const observation = {
  id: "o1",
  thread_id: "t1",
  blocks: [{ run_id: null }, { run_id: "r1" }],
}

function event(
  kind: ActivityEvent["target"]["kind"],
  id: string,
  extra: Partial<Pick<ActivityEvent, "actor" | "data">> = {}
) {
  return {
    seq: 1,
    type: "observation.updated",
    target: { kind, id },
    dashboard_id: null,
    folder_id: null,
    actor: { run_id: null } as ActivityEvent["actor"],
    data: {},
    ...extra,
    created_at: "2026-11-12T10:00:00Z",
  } satisfies ActivityEvent
}

function namesObservation(id: string) {
  return {
    object: { kind: "observation", id },
  } as unknown as ActivityEvent["data"]
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

  it("matches a thread event by the observation it names when the page has no thread yet", () => {
    const named = event("thread", "t9", {
      data: namesObservation("o1"),
    })
    expect(
      touchesObservation(named, { id: "o1", thread_id: null, blocks: [] })
    ).toBe("discussion")
  })

  it("ignores a thread event that names another observation", () => {
    const named = event("thread", "t9", {
      data: namesObservation("o2"),
    })
    expect(
      touchesObservation(named, { id: "o1", thread_id: null, blocks: [] })
    ).toBeNull()
  })

  it("refreshes the page too when a run replies in the discussion, because the run just finished", () => {
    const reply = event("thread", "t1", {
      actor: { run_id: "r9" } as ActivityEvent["actor"],
    })
    expect(touchesObservation(reply, observation)).toBe("both")
  })
})

describe("matchesActivityFilter", () => {
  const inFolder = { ...event("dashboard", "d1"), folder_id: "f1" }

  it("passes every event with no filter", () => {
    expect(matchesActivityFilter(inFolder, {})).toBe(true)
    expect(matchesActivityFilter(inFolder, { folderId: null, types: [] })).toBe(
      true
    )
  })

  it("keeps the events of one folder, as the stream's folder_id does", () => {
    expect(matchesActivityFilter(inFolder, { folderId: "f1" })).toBe(true)
    expect(matchesActivityFilter(inFolder, { folderId: "f2" })).toBe(false)
    expect(
      matchesActivityFilter(event("dashboard", "d1"), { folderId: "f1" })
    ).toBe(false)
  })

  it("keeps the events of the listed types, as the stream's type does", () => {
    expect(
      matchesActivityFilter(inFolder, {
        types: ["dashboard.created", "observation.updated"],
      })
    ).toBe(true)
    expect(
      matchesActivityFilter(inFolder, { types: ["dashboard.created"] })
    ).toBe(false)
  })

  it("needs both the folder and the type to match", () => {
    expect(
      matchesActivityFilter(inFolder, {
        folderId: "f2",
        types: ["observation.updated"],
      })
    ).toBe(false)
  })
})
