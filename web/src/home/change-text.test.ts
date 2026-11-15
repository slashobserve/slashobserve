import { describe, expect, test } from "vitest"

import type { components } from "@/api/schema.gen"

import { changeText } from "./change-text"

type ActivityEvent = components["schemas"]["ActivityEvent"]

const now = new Date(2026, 10, 15, 15, 30).getTime()
const yesterday = new Date(2026, 10, 14, 9, 0).toISOString()
const today = new Date(2026, 10, 15, 8, 0).toISOString()
const earlier = new Date(2026, 10, 4, 12, 0).toISOString()

function event(
  type: ActivityEvent["type"],
  createdAt: string,
  actorId = "me",
  label = "admin"
): ActivityEvent {
  return {
    type,
    created_at: createdAt,
    actor: { id: actorId, label },
  } as ActivityEvent
}

describe("changeText", () => {
  test.each([
    ["observation.block_added", "you added a block yesterday"],
    ["observation.block_removed", "you removed a block yesterday"],
    ["observation.block_updated", "you changed a block yesterday"],
    ["observation.updated", "you renamed it yesterday"],
    ["observation.resolved", "you resolved it yesterday"],
    ["observation.reopened", "you reopened it yesterday"],
    ["thread.opened", "you wrote in the discussion yesterday"],
    ["thread.replied", "you wrote in the discussion yesterday"],
  ] as const)("%s reads %s", (type, text) => {
    expect(changeText(event(type, yesterday), "me", now)).toBe(text)
  })

  test("names someone else by their label", () => {
    expect(
      changeText(
        event("observation.block_added", today, "other", "Sam Rivera"),
        "me",
        now
      )
    ).toBe("Sam Rivera added a block today")
  })

  test("names the date of an older change", () => {
    expect(changeText(event("observation.resolved", earlier), "me", now)).toBe(
      "you resolved it on Wed, Nov 4"
    )
  })

  test("leaves out the day it cannot read", () => {
    expect(
      changeText(event("observation.resolved", "not a date"), "me", now)
    ).toBe("you resolved it")
  })

  test("gives nothing for an event type it does not describe", () => {
    expect(
      changeText(event("observation.published", yesterday), "me", now)
    ).toBeNull()
  })
})
