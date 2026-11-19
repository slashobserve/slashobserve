import { beforeEach, describe, expect, it } from "vitest"

import { useDraftTitles } from "./draft-title"

const store = useDraftTitles

beforeEach(() => {
  store.setState({ titles: {} })
})

describe("draft titles", () => {
  it("shows a typed title and drops it when the editor clears it", () => {
    store.getState().show("p1", "Requests")
    expect(store.getState().titles).toEqual({ p1: "Requests" })
    store.getState().show("p1", undefined)
    expect(store.getState().titles).toEqual({})
  })

  it("falls back to the saved title while the field is blank", () => {
    store.getState().show("p1", "   ")
    expect(store.getState().titles).toEqual({})
  })

  it("keeps the same state when nothing changes", () => {
    store.getState().show("p1", "Requests")
    const before = store.getState()
    store.getState().show("p1", "Requests")
    expect(store.getState()).toBe(before)
  })
})
