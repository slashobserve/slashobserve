import { beforeEach, describe, expect, it } from "vitest"

import { nextSelection, useSelectionStore } from "./selection-store"

const order = ["a", "b", "c"]

describe("nextSelection", () => {
  it.each([
    ["ArrowDown", "a", "b"],
    ["ArrowDown", "b", "c"],
    ["ArrowUp", "c", "b"],
    ["ArrowUp", "b", "a"],
    ["Home", "c", "a"],
    ["End", "a", "c"],
  ] as const)("%s from %s selects %s", (key, current, expected) => {
    expect(nextSelection(order, current, key)).toBe(expected)
  })

  it("does not wrap at either end", () => {
    expect(nextSelection(order, "c", "ArrowDown")).toBe("c")
    expect(nextSelection(order, "a", "ArrowUp")).toBe("a")
  })

  it.each(["ArrowDown", "ArrowUp", "Home", "End"] as const)(
    "%s with nothing selected picks the first panel",
    (key) => {
      expect(nextSelection(order, null, key)).toBe("a")
    }
  )

  it("picks the first panel when the current one is gone", () => {
    expect(nextSelection(order, "gone", "ArrowDown")).toBe("a")
  })

  it("selects nothing in an empty dashboard", () => {
    expect(nextSelection([], null, "Home")).toBeNull()
  })
})

describe("selection store", () => {
  beforeEach(() => useSelectionStore.getState().clear())

  it("remembers the dashboard of the selected panel and clears", () => {
    useSelectionStore.getState().select("d1", "a")
    expect(useSelectionStore.getState()).toMatchObject({
      dashboardId: "d1",
      panelId: "a",
    })
    useSelectionStore.getState().clear()
    expect(useSelectionStore.getState()).toMatchObject({
      dashboardId: null,
      panelId: null,
    })
  })
})
