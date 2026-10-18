import { describe, expect, it } from "vitest"

import {
  commitKeys,
  emptySeriesState,
  toggleKey,
  visibleSeries,
} from "./series-state"

describe("visibleSeries", () => {
  const hidden = new Set(["checkout-1"])

  it("shows every series while the legend is not shown", () => {
    expect(visibleSeries(hidden, ["checkout-1"], false).size).toBe(0)
  })

  it("drops hidden entries for series that are gone", () => {
    expect(visibleSeries(hidden, ["checkout-0", "checkout-2"], true).size).toBe(
      0
    )
  })

  it("keeps hidden entries for series that are still present", () => {
    const kept = visibleSeries(hidden, ["checkout-0", "checkout-1"], true)
    expect(kept).toBe(hidden)
  })
})

describe("series state", () => {
  it("keeps colors and hidden series across the panel's lifetime", () => {
    let state = commitKeys(emptySeriesState, ["a", "b"])
    state = toggleKey(state, "b")
    state = commitKeys(state, ["b"])
    expect(state.slots.get("b")).toBe(1)
    expect([...state.hidden]).toEqual(["b"])
    state = commitKeys(state, ["a", "b"])
    expect(state.slots.get("a")).toBe(0)
    expect(state.slots.get("b")).toBe(1)
  })

  it("forgets hidden series that left the data", () => {
    let state = commitKeys(emptySeriesState, ["a", "b"])
    state = toggleKey(state, "b")
    state = commitKeys(state, ["a"])
    expect(state.hidden.size).toBe(0)
  })

  it("ignores empty data so a blank refresh keeps its state", () => {
    let state = commitKeys(emptySeriesState, ["a", "b"])
    state = toggleKey(state, "b")
    expect(commitKeys(state, [])).toBe(state)
  })

  it("returns the same state when nothing changed", () => {
    const state = commitKeys(emptySeriesState, ["a", "b"])
    expect(commitKeys(state, ["a", "b"])).toBe(state)
  })
})
