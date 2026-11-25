import { describe, expect, test } from "vitest"

import { inboxChoice } from "./inbox-views"

describe("inboxChoice", () => {
  test("keeps a known view and filter from an old Inbox address", () => {
    expect(inboxChoice({ view: "mentions", filter: "done" })).toEqual({
      view: "mentions",
      filter: "done",
    })
  })

  test("falls back to For you and Open for anything else", () => {
    expect(inboxChoice({})).toEqual({ view: "for_you", filter: "open" })
    expect(inboxChoice({ view: "spam", filter: 3, item: "12" })).toEqual({
      view: "for_you",
      filter: "open",
    })
  })
})
