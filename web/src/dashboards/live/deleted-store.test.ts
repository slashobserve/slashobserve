import { beforeEach, describe, expect, it } from "vitest"

import { deletedText, useDeletedDashboards } from "./deleted-store"

const store = useDeletedDashboards

beforeEach(() => {
  store.setState({ deletedBy: {}, heldDrafts: {} })
})

describe("deleted dashboards", () => {
  it("keeps the name of whoever deleted it when a later check knows none", () => {
    store.getState().markDeleted("d1", "Ana")
    store.getState().markDeleted("d1", "")
    expect(store.getState().deletedBy).toEqual({ d1: "Ana" })
  })

  it("records a deletion found by a check alone", () => {
    store.getState().markDeleted("d1", "")
    expect(store.getState().deletedBy).toEqual({ d1: "" })
  })

  it("holds and releases a draft per dashboard", () => {
    store.getState().holdDraft("d1", true)
    store.getState().holdDraft("d2", true)
    store.getState().holdDraft("d1", false)
    expect(store.getState().heldDrafts).toEqual({ d2: true })
  })

  it("keeps the same state when nothing changes, so subscribers do not rerender", () => {
    const before = store.getState()
    store.getState().holdDraft("d1", false)
    expect(store.getState()).toBe(before)
  })
})

describe("deletedText", () => {
  it("names who deleted the dashboard when the event did", () => {
    expect(deletedText("Ana")).toBe("Ana deleted this dashboard.")
    expect(deletedText("")).toBe("This dashboard was deleted.")
  })
})
