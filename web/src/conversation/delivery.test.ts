import { describe, expect, it } from "vitest"

import recipientCasesText from "../../../spec/examples/threads/recipient-cases.json?raw"
import {
  defaultDelivery,
  deliveryGate,
  flips,
  layoutEditingText,
  noModelReviseText,
  pinnedText,
  reviseBlock,
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

  it("comments to everyone on the canvas comment tool without an agent mention", () => {
    expect(
      defaultDelivery({ kind: "canvas", tool: "comment" }, "looks odd")
    ).toEqual({ to: "everyone", visibleTo: "everyone" })
    expect(
      defaultDelivery({ kind: "canvas", tool: "comment" }, "@Ada looks odd")
    ).toEqual({ to: "everyone", visibleTo: "everyone" })
  })

  it("asks the agent in view of everyone when a canvas comment mentions it", () => {
    expect(
      defaultDelivery({ kind: "canvas", tool: "comment" }, "@observe hi")
    ).toEqual({ to: "agent", visibleTo: "everyone" })
  })

  it("asks the agent privately on the canvas ask tool", () => {
    expect(
      defaultDelivery({ kind: "canvas", tool: "ask", chatId: null }, "hi")
    ).toEqual({
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

describe("flips", () => {
  it("turns nothing in a chat", () => {
    expect(flips({ kind: "chat", chatId: null })).toEqual({
      to: false,
      visibleTo: false,
    })
  })

  it("turns only the recipient in a thread", () => {
    expect(
      flips({ kind: "thread", threadId: null, agent: null, agentOff: false })
    ).toEqual({ to: true, visibleTo: false })
  })

  it("turns nothing on a canvas ask, which stays private to the asker, and only the recipient of a canvas comment", () => {
    expect(flips({ kind: "canvas", tool: "ask", chatId: null })).toEqual({
      to: false,
      visibleTo: false,
    })
    expect(flips({ kind: "canvas", tool: "comment" })).toEqual({
      to: true,
      visibleTo: false,
    })
  })
})

describe("deliveryGate", () => {
  const open = {
    canComment: true,
    agentAvailable: true,
    pinned: false,
    layoutEditing: false,
  }
  const toAgent: Delivery = { to: "agent", visibleTo: "everyone" }
  const toEveryone: Delivery = { to: "everyone", visibleTo: "everyone" }

  it("hides the agent from someone who cannot comment", () => {
    expect(deliveryGate({ ...open, canComment: false }, toAgent)).toEqual({
      kind: "hidden",
    })
  })

  it("allows a message to the agent", () => {
    expect(deliveryGate(open, toAgent)).toEqual({ kind: "allowed" })
    expect(
      deliveryGate({ ...open, agentAvailable: undefined }, toAgent)
    ).toEqual({ kind: "allowed" })
  })

  it("reports no model only for a message to the agent", () => {
    const off = { ...open, agentAvailable: false }
    expect(deliveryGate(off, toAgent)).toEqual({ kind: "no-model" })
    expect(deliveryGate(off, toEveryone)).toEqual({ kind: "allowed" })
  })

  it.each([
    ["a pinned version", { pinned: true }, pinnedText],
    ["a layout being edited", { layoutEditing: true }, layoutEditingText],
  ])(
    "blocks the agent on %s and lets a comment through",
    (_name, patch, reason) => {
      const input = { ...open, ...patch }
      expect(deliveryGate(input, toAgent)).toEqual({ kind: "blocked", reason })
      expect(deliveryGate(input, toEveryone)).toEqual({ kind: "allowed" })
    }
  )
})

describe("reviseBlock", () => {
  it("passes the composer's blocking reason through", () => {
    expect(reviseBlock({ kind: "blocked", reason: pinnedText }, true)).toBe(
      pinnedText
    )
  })

  it("blocks without a model only when the built-in agent would make the change", () => {
    expect(reviseBlock({ kind: "no-model" }, true)).toBe(noModelReviseText)
    expect(reviseBlock({ kind: "no-model" }, false)).toBeNull()
  })

  it("lets an allowed revision through", () => {
    expect(reviseBlock({ kind: "allowed" }, true)).toBeNull()
  })
})
