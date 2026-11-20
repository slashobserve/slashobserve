import { describe, expect, it } from "vitest"

import recipientCasesText from "../../../spec/examples/threads/recipient-cases.json?raw"
import {
  defaultDelivery,
  storeOf,
  type Delivery,
  type Place,
  type ThreadAgent,
} from "./delivery"

type RecipientCase = {
  name: string
  thread: { agent: ThreadAgent | null; off: boolean }
  body: string
  to: "" | "agent" | "everyone"
  builtinAgent: boolean
  want: {
    error: string | null
    agent: ThreadAgent | null
    sticky: "keep" | "off" | "follow"
    unavailable: boolean
  }
}

const recipientCases = JSON.parse(recipientCasesText) as RecipientCase[]

function threadPlace(thread: RecipientCase["thread"]): Place {
  return {
    kind: "thread",
    threadId: "t1",
    agent: thread.agent,
    agentOff: thread.off,
  }
}

function wantsAgent(want: RecipientCase["want"]): boolean {
  return want.error !== null || want.agent !== null || want.unavailable
}

describe("defaultDelivery in a thread, against the shared cases", () => {
  const unprompted = recipientCases.filter((c) => c.to === "")

  it("reads the cases the server's rule decides without a chosen recipient", () => {
    expect(unprompted.length).toBeGreaterThan(25)
  })

  it.each(unprompted.map((c) => [c.name, c] as const))("%s", (_name, c) => {
    expect(defaultDelivery(threadPlace(c.thread), c.body)).toEqual({
      to: wantsAgent(c.want) ? "agent" : "everyone",
      visibleTo: "everyone",
    })
  })
})

describe("defaultDelivery outside a thread", () => {
  it("always asks the agent privately in a chat", () => {
    expect(defaultDelivery({ kind: "chat", chatId: null }, "hi")).toEqual({
      to: "agent",
      visibleTo: "me",
    })
    expect(defaultDelivery({ kind: "chat", chatId: "c1" }, "@Ada hi")).toEqual({
      to: "agent",
      visibleTo: "me",
    })
  })

  it("comments to everyone on the canvas comment tool", () => {
    expect(
      defaultDelivery({ kind: "canvas", tool: "comment" }, "@observe hi")
    ).toEqual({ to: "everyone", visibleTo: "everyone" })
  })

  it("asks the agent privately on the canvas ask tool", () => {
    expect(defaultDelivery({ kind: "canvas", tool: "ask" }, "hi")).toEqual({
      to: "agent",
      visibleTo: "me",
    })
  })
})

describe("storeOf", () => {
  const cases: [Delivery, ReturnType<typeof storeOf>][] = [
    [{ to: "everyone", visibleTo: "everyone" }, "thread"],
    [{ to: "agent", visibleTo: "everyone" }, "thread"],
    [{ to: "agent", visibleTo: "me" }, "chat"],
    [{ to: "everyone", visibleTo: "me" }, null],
  ]

  it.each(cases)("%j is stored as %s", (delivery, store) => {
    expect(storeOf(delivery)).toBe(store)
  })
})
