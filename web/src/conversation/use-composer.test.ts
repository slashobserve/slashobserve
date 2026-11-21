import { describe, expect, it } from "vitest"

import { pinnedText, threadPlace, type Place } from "./delivery"
import { subjectOf } from "./send-plan"
import {
  chosenDelivery,
  oneAgentText,
  placeKey,
  recipientName,
  sendBlockOf,
} from "./use-composer"

const grantId = "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d"
const agents = [
  { kind: "observe" as const, id: "observe", name: "Observe Agent" },
  { kind: "grant" as const, id: grantId, name: "Claude Code" },
]
const chat: Place = { kind: "chat", chatId: null }
const newThread = threadPlace(null)

describe("recipientName", () => {
  it("names the mentioned agent first", () => {
    expect(
      recipientName(newThread, `@[Claude Code](grant:${grantId}) go`, agents)
    ).toBe("Claude Code")
  })

  it("names the thread's agent when nobody is mentioned", () => {
    const place = threadPlace("t1", {
      agent: { kind: "grant", id: grantId.toUpperCase() },
      agent_off: false,
    })
    expect(recipientName(place, "and now?", agents)).toBe("Claude Code")
    expect(
      recipientName(
        threadPlace("t1", {
          agent: { kind: "builtin", id: null },
          agent_off: false,
        }),
        "and now?",
        agents
      )
    ).toBe("Observe Agent")
  })

  it("falls back to the built-in agent", () => {
    expect(recipientName(chat, "why", agents)).toBe("Observe Agent")
  })
})

describe("chosenDelivery", () => {
  it("keeps a turned To chip in a thread", () => {
    expect(chosenDelivery(newThread, "note", { to: "agent" })).toEqual({
      to: "agent",
      visibleTo: "everyone",
    })
  })

  it("ignores a choice the place cannot turn", () => {
    expect(
      chosenDelivery(chat, "why", { to: "everyone", visibleTo: "everyone" })
    ).toEqual({ to: "agent", visibleTo: "me" })
    expect(chosenDelivery(newThread, "note", { visibleTo: "me" })).toEqual({
      to: "everyone",
      visibleTo: "everyone",
    })
  })

  it("follows the body until a chip is turned", () => {
    expect(chosenDelivery(newThread, "@observe go", {})).toMatchObject({
      to: "agent",
    })
  })
})

describe("sendBlockOf", () => {
  const blocked = { kind: "blocked" as const, reason: pinnedText }

  it("refuses two agents in one message", () => {
    expect(
      sendBlockOf(
        `@observe @[Claude Code](grant:${grantId}) go`,
        { to: "agent", visibleTo: "everyone" },
        { kind: "allowed" }
      )
    ).toBe(oneAgentText)
  })

  it("holds back only a message to the agent", () => {
    expect(
      sendBlockOf("go", { to: "agent", visibleTo: "everyone" }, blocked)
    ).toBe(pinnedText)
    expect(
      sendBlockOf("go", { to: "everyone", visibleTo: "everyone" }, blocked)
    ).toBeNull()
  })
})

describe("placeKey", () => {
  it("tells places apart so a turned chip resets", () => {
    expect(placeKey(threadPlace("t1"))).not.toBe(placeKey(threadPlace("t2")))
    expect(placeKey(chat)).toBe("chat:")
    expect(placeKey({ kind: "canvas", tool: "ask", chatId: "c1" })).toBe(
      "canvas:ask"
    )
  })
})

describe("subjectOf", () => {
  it("reads every thread target and refuses anything else", () => {
    expect(subjectOf("dashboard:d1")).toEqual({ kind: "dashboard", id: "d1" })
    expect(subjectOf("proposal:p1")).toEqual({ kind: "proposal", id: "p1" })
    expect(subjectOf("finding:f1")).toBeNull()
    expect(subjectOf("dashboard:")).toBeNull()
    expect(subjectOf("nothing")).toBeNull()
  })
})
