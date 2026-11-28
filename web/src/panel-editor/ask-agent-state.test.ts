import { describe, expect, it } from "vitest"

import {
  askIdle,
  reduceAsk,
  suggestBusy,
  type AskEvent,
  type AskPhase,
} from "./ask-agent-state"
import type { EditablePanel } from "./draft"

const panel = { id: "p1", type: "timeseries" } as EditablePanel
const startCreate: AskEvent = {
  type: "create_started",
  panel,
  prompt: "only 5xx",
}
const started = { suggestionId: "s1", panel, prompt: "only 5xx" }
const tracking: AskPhase = { kind: "tracking", started }

function walk(...events: AskEvent[]): AskPhase {
  return events.reduce(reduceAsk, askIdle)
}

describe("reduceAsk", () => {
  it("creates once, then tracks what was created", () => {
    expect(walk(startCreate)).toEqual({
      kind: "creating",
      panel,
      prompt: "only 5xx",
    })
    expect(
      walk(startCreate, { ...startCreate, prompt: "again" })
    ).toMatchObject({ prompt: "only 5xx" })
    expect(walk(startCreate, { type: "created", suggestionId: "s1" })).toEqual(
      tracking
    )
  })

  it("fails a refused prompt or a failed create, and typing clears it", () => {
    expect(walk({ type: "refused", error: "Describe it" })).toEqual({
      kind: "failed",
      error: "Describe it",
      started: null,
    })
    const failed = walk(startCreate, { type: "create_failed", error: "busy" })
    expect(failed).toEqual({ kind: "failed", error: "busy", started: null })
    expect(reduceAsk(failed, { type: "typed" })).toEqual(askIdle)
  })

  it("ignores a refusal while a create is under way", () => {
    expect(
      walk(startCreate, { type: "refused", error: "Describe it" }).kind
    ).toBe("creating")
  })

  it("fails a tracked suggestion whose poll cannot recover", () => {
    expect(
      reduceAsk(tracking, {
        type: "poll_failed",
        suggestionId: "s1",
        error: "gone",
      })
    ).toEqual({ kind: "failed", error: "gone", started })
    expect(
      reduceAsk(tracking, {
        type: "poll_failed",
        suggestionId: "s0",
        error: "gone",
      })
    ).toBe(tracking)
  })

  it("is applied after an accept and idle after a discard", () => {
    expect(reduceAsk(tracking, { type: "accepted" })).toEqual({
      kind: "applied",
    })
    expect(reduceAsk(tracking, { type: "discarded" })).toEqual(askIdle)
    expect(reduceAsk({ kind: "applied" }, { type: "typed" })).toEqual({
      kind: "applied",
    })
  })

  it("ignores a result no create is waiting for", () => {
    expect(walk({ type: "created", suggestionId: "s1" })).toEqual(askIdle)
    expect(walk({ type: "create_failed", error: "x" })).toEqual(askIdle)
    expect(walk({ type: "accepted" })).toEqual(askIdle)
  })
})

describe("suggestBusy", () => {
  it("is busy while creating and while a tracked suggestion runs", () => {
    expect(suggestBusy(walk(startCreate), undefined)).toBe(true)
    expect(suggestBusy(tracking, undefined)).toBe(true)
    expect(suggestBusy(tracking, "running")).toBe(true)
  })

  it("is free once the suggestion settles or nothing runs", () => {
    expect(suggestBusy(tracking, "succeeded")).toBe(false)
    expect(suggestBusy(tracking, "failed")).toBe(false)
    expect(suggestBusy(askIdle, undefined)).toBe(false)
    expect(
      suggestBusy({ kind: "failed", error: "x", started }, "running")
    ).toBe(false)
  })
})
