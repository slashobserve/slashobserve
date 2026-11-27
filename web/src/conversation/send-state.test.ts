import { describe, expect, it } from "vitest"

import type { SentMessage } from "@/comments/thread-queries"

import {
  delivering,
  reduceSend,
  sendAt,
  sendIn,
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
  return events.reduce(reduceSend, sendAt(here))
}

function at(place: string, ...events: SendEvent[]) {
  return sendIn(walk(...events), place)
}

const held: SendEvent = { type: "hold_started", place: here }
const sendStarted: SendEvent = { type: "send_started", place: here }
const failed: SendEvent = { type: "failed", place: here, failure }
const moved: SendEvent = { type: "placed", place: there }

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
    const delivered: SendEvent = { type: "delivered", place: here, started }
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
        { type: "delivered", place: here, started: null },
        opened
      )
    ).toEqual({ status: "idle" })
  })

  it("ignores a delivery or an opening that no send is waiting for", () => {
    expect(at(here, { type: "delivered", place: here, started })).toEqual({
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
      })
    ).toEqual({ status: "idle" })
  })

  it("is idle on coming back, with a hold number the old hold cannot match", () => {
    const back = walk(held, moved, { type: "placed", place: here })
    expect(back.send).toEqual({ status: "idle" })
    expect(reduceSend(back, held).send).toEqual({ status: "holding", hold: 2 })
  })
})
