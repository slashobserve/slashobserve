import { describe, expect, it } from "vitest"

import {
  blockAskable,
  keyBlockOf,
  ownerGone,
  takesBlockComments,
} from "./observation-canvas"
import type { Block } from "./queries"

const panel = { id: "b1", kind: "panel", state: "live" } as const
const text = { id: "b2", kind: "text", state: "live" } as const
const run = { id: "b3", kind: "run", state: "live" } as const
const requested = { id: "b4", kind: "panel", state: "requested" } as const

describe("takesBlockComments", () => {
  it("puts every live block on the canvas for someone who can comment", () => {
    for (const block of [panel, text, run]) {
      expect(takesBlockComments(block, true)).toBe(true)
      expect(takesBlockComments(block, false)).toBe(false)
    }
    expect(takesBlockComments(requested, true)).toBe(false)
  })
})

describe("keyBlockOf", () => {
  const blocks = [panel, text, run, requested]
  const live = (block: { state: string }) => block.state === "live"

  it("takes the first candidate that the canvas takes, of any kind", () => {
    expect(keyBlockOf(blocks, ["b4", "b2"], live)).toBe(text)
    expect(keyBlockOf(blocks, [undefined, "b3"], live)).toBe(run)
  })

  it("names no block when none of them is on the canvas", () => {
    expect(keyBlockOf(blocks, ["b4", null, "gone"], live)).toBeUndefined()
  })
})

describe("ownerGone", () => {
  const blocks = [panel, requested]

  it("finds a draft's block gone once it is deleted or off the canvas", () => {
    expect(ownerGone(blocks, "b1", true)).toBe(false)
    expect(ownerGone(blocks, "deleted", true)).toBe(true)
    expect(ownerGone(blocks, "b4", true)).toBe(true)
    expect(ownerGone(blocks, "b1", false)).toBe(true)
  })
})

describe("blockAskable", () => {
  function panelBlock(type: string): Block {
    return {
      id: "b1",
      kind: "panel",
      state: "live",
      body: { panel: { type, title: "Errors" } },
    } as unknown as Block
  }

  it("asks about a data panel block only", () => {
    expect(blockAskable(panelBlock("timeseries"), true, true)).toBe(true)
    expect(blockAskable(panelBlock("text"), true, true)).toBe(false)
  })

  it("asks about a text, snapshot or run block whenever someone may ask", () => {
    const run = { id: "b3", kind: "run", state: "live", body: {} } as Block
    expect(blockAskable(run, true, true)).toBe(true)
    expect(blockAskable(run, true, false)).toBe(false)
  })

  it("asks nothing without Comment, a model or a panel", () => {
    expect(blockAskable(panelBlock("timeseries"), false, true)).toBe(false)
    expect(blockAskable(panelBlock("timeseries"), true, false)).toBe(false)
    expect(blockAskable(undefined, true, true)).toBe(false)
  })
})
