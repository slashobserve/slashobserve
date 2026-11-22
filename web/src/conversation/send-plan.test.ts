import { describe, expect, it } from "vitest"

import type { Delivery, Place } from "./delivery"
import {
  noSubjectText,
  noteToSelfText,
  planRevise,
  planSend,
  type Subject,
} from "./send-plan"

const dashboard: Subject = { kind: "dashboard", id: "d1" }
const observation: Subject = { kind: "observation", id: "o1" }
const anchor = { kind: "title" as const, pos: { x: 0.1, y: 0.2 } }

const newChat: Place = { kind: "chat", chatId: null }
const openChat: Place = { kind: "chat", chatId: "c1" }
const newThread: Place = {
  kind: "thread",
  threadId: null,
  agent: null,
  agentOff: false,
}
const openThread: Place = { ...newThread, threadId: "t1" }

const privately: Delivery = { to: "agent", visibleTo: "me" }
const toAgent: Delivery = { to: "agent", visibleTo: "everyone" }
const toEveryone: Delivery = { to: "everyone", visibleTo: "everyone" }

const draft = { body: "why", clientId: "k1" }

describe("planSend into a chat", () => {
  it("builds or answers from Home and opens the chat", () => {
    expect(planSend(newChat, null, privately, draft)).toEqual({
      via: "chat.turn",
      input: { prompt: "why", clientId: "k1", kind: "ask" },
      opens: "chat",
    })
  })

  it("continues an open chat in place", () => {
    expect(planSend(openChat, null, privately, draft)).toEqual({
      via: "chat.turn",
      input: { prompt: "why", clientId: "k1", chatId: "c1", kind: "ask" },
      opens: "same",
    })
  })

  it("asks about a dashboard panel and its spot", () => {
    expect(
      planSend(openChat, dashboard, privately, {
        ...draft,
        target: { panelId: "p1", anchor, blockId: "b1" },
      })
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        chatId: "c1",
        kind: "ask",
        dashboardId: "d1",
        panelId: "p1",
        anchor,
      },
      opens: "same",
    })
  })

  it("asks about an observation block", () => {
    expect(
      planSend(newChat, observation, privately, {
        ...draft,
        target: { anchor, blockId: "b1" },
      })
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        kind: "ask",
        observationId: "o1",
        anchor,
        blockId: "b1",
      },
      opens: "same",
    })
  })

  it("imports into a new chat even from an open one", () => {
    expect(
      planSend(openChat, dashboard, privately, { ...draft, command: "import" })
    ).toEqual({
      via: "chat.turn",
      input: { prompt: "why", clientId: "k1", kind: "ask" },
      opens: "chat",
    })
  })

  it("investigates a dashboard panel over a window", () => {
    expect(
      planSend(openChat, dashboard, privately, {
        ...draft,
        command: "investigate",
        target: { panelId: "p1" },
        range: { from: "now-6h", to: "now" },
      })
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        chatId: "c1",
        kind: "investigate",
        dashboardId: "d1",
        panelId: "p1",
        from: "now-6h",
        to: "now",
      },
      opens: "same",
    })
  })
})

describe("planSend investigating the observation a chat is about", () => {
  it("names the observation with the chat", () => {
    expect(
      planSend(openChat, observation, privately, {
        ...draft,
        command: "investigate",
      })
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        chatId: "c1",
        kind: "investigate",
        observationId: "o1",
      },
      opens: "same",
    })
  })
})

describe("planSend into a thread", () => {
  it("replies in the open thread with the recipient the person chose", () => {
    expect(
      planSend(openThread, dashboard, toEveryone, {
        ...draft,
        toChosen: true,
        replyToId: "m1",
        target: { blockId: "b1" },
      })
    ).toEqual({
      via: "thread.reply",
      threadId: "t1",
      body: "why",
      to: "everyone",
      clientId: "k1",
      blockId: "b1",
      replyToId: "m1",
    })
  })

  it("leaves the recipient of a thread message to the server unless the person chose it", () => {
    expect(planSend(openThread, dashboard, toEveryone, draft)).toEqual({
      via: "thread.reply",
      threadId: "t1",
      body: "why",
      clientId: "k1",
    })
    expect(planSend(newThread, dashboard, toAgent, draft)).not.toHaveProperty(
      "to"
    )
  })

  it("names the recipient of a canvas message, which has no thread rule", () => {
    expect(
      planSend(
        { kind: "canvas", tool: "comment" },
        dashboard,
        toEveryone,
        draft
      )
    ).toMatchObject({ via: "thread.open", to: "everyone" })
  })

  it("opens a dashboard thread on a panel and spot", () => {
    expect(
      planSend(newThread, dashboard, toAgent, {
        ...draft,
        toChosen: true,
        target: { panelId: "p1", anchor, blockId: "b1" },
      })
    ).toEqual({
      via: "thread.open",
      target: "dashboard:d1",
      body: "why",
      to: "agent",
      clientId: "k1",
      anchor,
      panelId: "p1",
    })
  })

  it("opens an observation thread on a block", () => {
    expect(
      planSend(newThread, observation, toEveryone, {
        ...draft,
        target: { blockId: "b1" },
      })
    ).toEqual({
      via: "thread.open",
      target: "observation:o1",
      body: "why",
      clientId: "k1",
      blockId: "b1",
    })
  })

  it("posts a canvas comment as a new thread", () => {
    expect(
      planSend(
        { kind: "canvas", tool: "comment" },
        dashboard,
        toEveryone,
        draft
      )
    ).toMatchObject({ via: "thread.open", target: "dashboard:d1" })
  })

  it("needs a subject to open a thread", () => {
    expect(planSend(newThread, null, toEveryone, draft)).toEqual({
      error: noSubjectText,
    })
  })

  it("refuses a note only the sender sees", () => {
    expect(
      planSend(
        openThread,
        dashboard,
        { to: "everyone", visibleTo: "me" },
        draft
      )
    ).toEqual({ error: noteToSelfText })
  })
})

describe("planSend for a canvas ask", () => {
  const rowAnchor = {
    kind: "row" as const,
    pos: { x: 0.4, y: 0.6 },
    row: { service: "payments", span: "authorize_card" },
    shown: "440",
    window: { from: "now-1h", to: "now" },
  }

  it("sends the spot, the value under the pointer and the window as a turn of the open chat", () => {
    expect(
      planSend(
        { kind: "canvas", tool: "ask", chatId: "c1" },
        dashboard,
        privately,
        { ...draft, target: { panelId: "p1", anchor: rowAnchor } }
      )
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        chatId: "c1",
        kind: "ask",
        dashboardId: "d1",
        panelId: "p1",
        anchor: rowAnchor,
      },
      opens: "same",
    })
  })

  it("starts a new chat about the dashboard when none is open, and stays on the canvas", () => {
    expect(
      planSend(
        { kind: "canvas", tool: "ask", chatId: null },
        dashboard,
        privately,
        { ...draft, target: { panelId: "p1" } }
      )
    ).toEqual({
      via: "chat.turn",
      input: {
        prompt: "why",
        clientId: "k1",
        kind: "ask",
        dashboardId: "d1",
        panelId: "p1",
      },
      opens: "same",
    })
  })
})

describe("planRevise", () => {
  it("applies the answer's change in the thread the answer is in", () => {
    expect(planRevise(openThread, dashboard, "f1", "k2")).toEqual({
      via: "thread.apply",
      threadId: "t1",
      findingId: "f1",
    })
  })

  it("asks for the answer's change by its finding as the next turn of the chat", () => {
    expect(planRevise(openChat, dashboard, "f1", "k2")).toEqual({
      via: "chat.turn",
      input: {
        appliesFindingId: "f1",
        clientId: "k2",
        kind: "revise",
        dashboardId: "d1",
        chatId: "c1",
      },
      opens: "same",
    })
  })

  it("revises only a dashboard", () => {
    expect(planRevise(openChat, observation, "f1", "k2")).toEqual({
      error: "Only a dashboard can be revised.",
    })
  })
})
