import { describe, expect, it } from "vitest"

import type { SentMessage } from "@/comments/thread-queries"

import {
  delivering,
  reduceSend,
  sendAt,
  sendIn,
  sentOtherBody,
  type PlacedSend,
  type SendEvent,
  type Started,
} from "./send-state"

const here = "chat:a"
const there = "chat:b"
const failure = { text: "Nothing was sent.", invalid: false }
const started: Started = {
  sent: { id: "m1" } as Partial<SentMessage> as SentMessage,
  agentName: "Observe",
}

function walk(...events: SendEvent[]): PlacedSend {
  return events.reduce(reduceSend, sendAt(here, "id-1"))
}

function at(place: string, ...events: SendEvent[]) {
  return sendIn(walk(...events), place)
}

const held: SendEvent = { type: "hold_started", place: here }
const sendStarted: SendEvent = { type: "send_started", place: here, body: "a" }
const failed: SendEvent = { type: "failed", place: here, failure }
const moved: SendEvent = { type: "placed", place: there, clientId: "id-2" }

describe("reduceSend", () => {
  it("starts idle", () => {
    expect(at(here)).toEqual({ status: "idle" })
  })

  it("holds once, however often a hold is asked", () => {
    expect(at(here, held)).toEqual({ status: "holding", hold: 1 })
    expect(at(here, held, held)).toEqual({ status: "holding", hold: 1 })
  })

  it("ends only the hold it is asked about", () => {
    const again = walk(held, { type: "hold_cancelled", place: here }, held)
    expect(again.send).toEqual({ status: "holding", hold: 2 })
    expect(
      reduceSend(again, { type: "hold_ended", place: here, hold: 1 }).send
    ).toEqual({ status: "holding", hold: 2 })
    expect(
      reduceSend(again, { type: "hold_ended", place: here, hold: 2 }).send
    ).toEqual({ status: "idle" })
  })

  it("fails from a hold, and an edit clears the failure", () => {
    expect(at(here, held, failed)).toEqual({ status: "failed", failure })
    expect(at(here, failed, { type: "edited", place: here })).toEqual({
      status: "idle",
    })
    expect(at(here, held, { type: "edited", place: here })).toEqual({
      status: "holding",
      hold: 1,
    })
  })

  it("is idle after a failure that was reported elsewhere", () => {
    expect(
      at(here, sendStarted, { type: "failed", place: here, failure: null })
    ).toEqual({ status: "idle" })
  })

  it("sends once and refuses a hold or a second send meanwhile", () => {
    expect(at(here, failed, sendStarted)).toEqual({ status: "sending" })
    expect(at(here, sendStarted, sendStarted, held)).toEqual({
      status: "sending",
    })
    expect(delivering(at(here, sendStarted))).toBe(true)
  })

  it("opens what was delivered, then shows what the agent started", () => {
    const delivered: SendEvent = {
      type: "delivered",
      place: here,
      started,
      clientId: "id-2",
    }
    const opened: SendEvent = { type: "opened", place: here }
    expect(at(here, sendStarted, delivered)).toEqual({
      status: "opening",
      started,
    })
    expect(delivering(at(here, sendStarted, delivered))).toBe(true)
    expect(at(here, sendStarted, delivered, sendStarted)).toEqual({
      status: "opening",
      started,
    })
    expect(at(here, sendStarted, delivered, opened)).toEqual({
      status: "started",
      started,
    })
    expect(
      at(
        here,
        sendStarted,
        { type: "delivered", place: here, started: null, clientId: "id-2" },
        opened
      )
    ).toEqual({ status: "idle" })
  })

  it("ignores a delivery or an opening that no send is waiting for", () => {
    expect(
      at(here, {
        type: "delivered",
        place: here,
        started,
        clientId: "id-2",
      })
    ).toEqual({
      status: "idle",
    })
    expect(at(here, failed, { type: "opened", place: here })).toEqual({
      status: "failed",
      failure,
    })
  })

  it("starts from idle in another place", () => {
    expect(at(there, held, moved)).toEqual({ status: "idle" })
    expect(at(there, failed, moved)).toEqual({ status: "idle" })
    expect(at(there, sendStarted, moved)).toEqual({ status: "idle" })
  })

  it("shows another place as idle before the move is recorded", () => {
    expect(at(there, failed)).toEqual({ status: "idle" })
  })

  it("drops what the place left behind reports later", () => {
    expect(
      at(there, held, moved, { type: "hold_ended", place: here, hold: 1 })
    ).toEqual({ status: "idle" })
    expect(at(there, sendStarted, moved, failed)).toEqual({ status: "idle" })
    expect(
      at(there, sendStarted, moved, {
        type: "delivered",
        place: here,
        started,
        clientId: "id-3",
      })
    ).toEqual({ status: "idle" })
  })

  it("is idle on coming back, with a hold number the old hold cannot match", () => {
    const back = walk(held, moved, {
      type: "placed",
      place: here,
      clientId: "id-3",
    })
    expect(back.send).toEqual({ status: "idle" })
    expect(reduceSend(back, held).send).toEqual({ status: "holding", hold: 2 })
  })
})

describe("client ids", () => {
  const edited: SendEvent = { type: "send_started", place: here, body: "b" }
  const superseded: SendEvent = {
    type: "superseded",
    place: here,
    failure,
    clientId: "id-2",
  }

  it("keeps the id and records each body across failed attempts", () => {
    const state = walk(sendStarted, failed, edited, failed)
    expect(state.clientId).toBe("id-1")
    expect(state.sentBodies).toEqual(["a", "b"])
    expect(sentOtherBody(state, "b")).toBe(true)
    expect(sentOtherBody(walk(sendStarted, failed, sendStarted), "a")).toBe(
      false
    )
  })

  it("keeps the id after an edit", () => {
    const state = walk(sendStarted, failed, { type: "edited", place: here })
    expect(state.clientId).toBe("id-1")
    expect(state.sentBodies).toEqual(["a"])
  })

  it("records no body for a send that was already under way", () => {
    expect(walk(sendStarted, edited).sentBodies).toEqual(["a"])
  })

  it("takes a fresh id once a send is delivered", () => {
    const state = walk(sendStarted, {
      type: "delivered",
      place: here,
      started: null,
      clientId: "id-2",
    })
    expect(state.clientId).toBe("id-2")
    expect(state.sentBodies).toEqual([])
  })

  it("takes a fresh id and says so when the first message was sent", () => {
    const state = walk(sendStarted, failed, edited, superseded)
    expect(state.send).toEqual({ status: "failed", failure })
    expect(state.clientId).toBe("id-2")
    expect(state.sentBodies).toEqual([])
  })

  it("ignores a supersession that no send is waiting for", () => {
    const state = walk(sendStarted, failed, superseded)
    expect(state.clientId).toBe("id-1")
  })

  it("takes a fresh id in another place, and keeps it on coming back to the same one", () => {
    const state = walk(sendStarted, failed, moved)
    expect(state.clientId).toBe("id-2")
    expect(state.sentBodies).toEqual([])
    expect(
      reduceSend(state, { type: "placed", place: there, clientId: "id-9" })
        .clientId
    ).toBe("id-2")
  })
})
