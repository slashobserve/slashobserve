import { describe, expect, it } from "vitest"

import {
  asksToDiscard,
  hasUnsavedEdits,
  noUnsavedEdits,
  reduceLeave,
  type LeaveEvent,
  type LeaveState,
} from "./leave-state"

type Kind = "close" | "back"

function walk(...events: LeaveEvent<Kind>[]): LeaveState<Kind> {
  return events.reduce<LeaveState<Kind>>(reduceLeave, noUnsavedEdits)
}

const edited: LeaveEvent<Kind> = { type: "edited", dirty: true }
const saved: LeaveEvent<Kind> = { type: "edited", dirty: false }
const askedClose: LeaveEvent<Kind> = { type: "asked", kind: "close" }
const held: LeaveEvent<Kind> = { type: "navigation_held" }
const kept: LeaveEvent<Kind> = { type: "kept" }
const left: LeaveEvent<Kind> = { type: "left" }

describe("reduceLeave", () => {
  it("moves between clean and dirty with the draft", () => {
    expect(walk(edited)).toEqual({ status: "dirty" })
    expect(walk(edited, saved)).toEqual({ status: "clean" })
    expect(walk(saved)).toEqual({ status: "clean" })
  })

  it("asks before a leave only while there are unsaved edits", () => {
    expect(walk(askedClose)).toEqual({ status: "clean" })
    expect(walk(edited, askedClose)).toEqual({
      status: "confirming",
      kind: "close",
    })
  })

  it("holds a navigation only while there are unsaved edits", () => {
    expect(walk(held)).toEqual({ status: "clean" })
    expect(walk(edited, held)).toEqual({ status: "blocked" })
  })

  it("keeps one prompt when a navigation arrives during a confirm", () => {
    const state = walk(edited, askedClose, held)
    expect(state).toEqual({ status: "blocked" })
    expect(asksToDiscard(state)).toBe(true)
    expect(walk(edited, held, askedClose)).toEqual({ status: "blocked" })
  })

  it("keeps the first leave when a second one is asked", () => {
    expect(walk(edited, askedClose, { type: "asked", kind: "back" })).toEqual({
      status: "confirming",
      kind: "close",
    })
  })

  it("returns to dirty, with the edits, when the person keeps editing", () => {
    expect(walk(edited, askedClose, kept)).toEqual({ status: "dirty" })
    expect(walk(edited, held, kept)).toEqual({ status: "dirty" })
    expect(walk(kept)).toEqual({ status: "clean" })
    expect(walk(edited, kept)).toEqual({ status: "dirty" })
  })

  it("keeps a prompt open until it is answered", () => {
    expect(walk(edited, askedClose, saved)).toEqual({
      status: "confirming",
      kind: "close",
    })
    expect(walk(edited, held, saved)).toEqual({ status: "blocked" })
  })

  it("is clean after a leave from any state", () => {
    expect(walk(left)).toEqual({ status: "clean" })
    expect(walk(edited, left)).toEqual({ status: "clean" })
    expect(walk(edited, askedClose, left)).toEqual({ status: "clean" })
    expect(walk(edited, held, left)).toEqual({ status: "clean" })
  })

  it("has unsaved edits in every state but clean", () => {
    expect(hasUnsavedEdits(walk())).toBe(false)
    expect(hasUnsavedEdits(walk(edited))).toBe(true)
    expect(hasUnsavedEdits(walk(edited, askedClose))).toBe(true)
    expect(hasUnsavedEdits(walk(edited, held))).toBe(true)
    expect(asksToDiscard(walk(edited))).toBe(false)
  })
})
