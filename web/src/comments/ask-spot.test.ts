import { describe, expect, it } from "vitest"

import type { Anchor } from "@/panels/anchors"

import {
  askChips,
  askWorkingText,
  pinName,
  privatePins,
  spotKey,
  spotLabel,
  spotTurnIds,
  type PinTurn,
} from "./ask-spot"

const row: Anchor = {
  kind: "row",
  pos: { x: 0.4, y: 0.6 },
  row: { service: "payments", span: "authorize_card" },
  shown: "440",
  window: { from: "now-1h", to: "now" },
}

const sameRowOtherOrder = {
  window: { to: "now", from: "now-1h" },
  shown: "440",
  row: { span: "authorize_card", service: "payments" },
  pos: { y: 0.6, x: 0.4 },
  kind: "row",
} as Anchor

const point: Anchor = {
  kind: "point",
  pos: { x: 0.45, y: 0.1 },
  series: { service: "payments" },
  from: "2026-11-21T14:02:00Z",
}

function turn(
  fields: Partial<PinTurn & { retried_by: string | null }> & { id: string }
): PinTurn & { retried_by: string | null } {
  return {
    retried_by: null,
    prompt: "Why is payments slow?",
    chat_id: "c1",
    panel_id: "p1",
    anchor: row,
    requested_by: { id: "me", kind: "user", name: "Me" },
    created_at: "2026-11-21T14:31:00Z",
    ...fields,
  }
}

describe("spotKey", () => {
  it("is the same for one anchor whatever the key order", () => {
    expect(spotKey(sameRowOtherOrder)).toBe(spotKey(row))
    expect(spotKey(point)).not.toBe(spotKey(row))
  })
})

describe("privatePins", () => {
  it("draws one pin per spot of the viewer's own chat turns on the panel", () => {
    const pins = privatePins(
      [
        turn({ id: "r2", created_at: "2026-11-21T14:33:00Z" }),
        turn({ id: "r1" }),
        turn({ id: "r3", anchor: point }),
      ],
      "p1",
      "me"
    )
    expect(pins).toEqual([
      expect.objectContaining({ requestId: "r1", chatId: "c1", turns: 2 }),
      expect.objectContaining({ requestId: "r3", turns: 1 }),
    ])
  })

  it("draws nothing for someone else's turns, which an Admin may read", () => {
    expect(
      privatePins(
        [
          turn({
            id: "r1",
            requested_by: { id: "other", kind: "user", name: "Other" },
          }),
        ],
        "p1",
        "me"
      )
    ).toEqual([])
  })

  it("draws nothing for a turn with no spot, on another panel or outside a chat", () => {
    expect(
      privatePins(
        [
          turn({ id: "r1", anchor: null }),
          turn({ id: "r3", panel_id: "p2" }),
          turn({ id: "r4", chat_id: null }),
        ],
        "p1",
        "me"
      )
    ).toEqual([])
  })

  it("keeps the same spot in two chats as two pins", () => {
    expect(
      privatePins(
        [turn({ id: "r1" }), turn({ id: "r2", chat_id: "c2" })],
        "p1",
        "me"
      )
    ).toHaveLength(2)
  })
})

describe("spotTurnIds", () => {
  it("lists the chat's turns on the opened spot in order, then those asked here", () => {
    expect(
      spotTurnIds(
        [
          turn({ id: "r3", created_at: "2026-11-21T14:40:00Z" }),
          turn({ id: "r1" }),
          turn({ id: "r2", anchor: point }),
        ],
        { requestId: "r1", panelId: "p1", anchor: sameRowOtherOrder },
        ["r4", "r3"]
      )
    ).toEqual(["r1", "r3", "r4"])
  })

  it("shows the opened turn before the chat has loaded it", () => {
    expect(
      spotTurnIds([], { requestId: "r9", panelId: "p1", anchor: row }, [])
    ).toEqual(["r9"])
  })

  it("leaves out a turn at the same spot of another panel", () => {
    expect(
      spotTurnIds(
        [turn({ id: "r1" }), turn({ id: "r2", panel_id: "p2" })],
        { requestId: "r1", panelId: "p1", anchor: row },
        []
      )
    ).toEqual(["r1"])
  })

  it("keeps a retry with the spot of the turn it retried, whatever its own anchor", () => {
    expect(
      spotTurnIds(
        [
          turn({ id: "r1", retried_by: "r2" }),
          turn({
            id: "r2",
            anchor: null,
            retried_by: "r3",
            created_at: "2026-11-21T14:32:00Z",
          }),
          turn({ id: "r3", anchor: null, created_at: "2026-11-21T14:33:00Z" }),
          turn({ id: "r4", anchor: null, created_at: "2026-11-21T14:34:00Z" }),
        ],
        { requestId: "r1", panelId: "p1", anchor: row },
        []
      )
    ).toEqual(["r1", "r2", "r3"])
  })
})

describe("askChips", () => {
  it("names the panel, the target, the value under the pointer and the window", () => {
    expect(askChips("Span errors", row).map((chip) => chip.label)).toEqual([
      "Span errors",
      "The row payments, authorize_card",
      "440",
      "last hour",
    ])
  })

  it("names the whole panel when asked by key", () => {
    expect(askChips("", null)).toEqual([
      { id: "panel", label: "Untitled panel, whole panel" },
    ])
  })

  it("joins the chips into the popover's heading and the pin's name", () => {
    expect(spotLabel("Span errors", row)).toBe(
      "Span errors · The row payments, authorize_card · 440 · last hour"
    )
    expect(pinName("Span errors", row)).toBe(
      "Your question on Span errors, The row payments, authorize_card, 440, last hour"
    )
  })
})

describe("askWorkingText", () => {
  it("says what the agent is doing", () => {
    expect(askWorkingText({ status: "queued", intent: "ask" })).toBe(
      "Waiting for a free agent"
    )
    expect(askWorkingText({ status: "running", intent: "ask" })).toBe(
      "Looking into it"
    )
    expect(askWorkingText({ status: "running", intent: "apply" })).toBe(
      "Revising the dashboard"
    )
  })
})
