import { describe, expect, it } from "vitest"

import { kioskStep, nextFolderDashboard, nextRowIndex } from "./kiosk"

describe("nextRowIndex", () => {
  it.each([
    [3, null, 0],
    [3, 0, 1],
    [3, 1, 2],
    [3, 2, 0],
    [1, 0, 0],
    [2, 5, 0],
    [2, -1, 0],
  ])("with %d rows after %s goes to %s", (count, current, expected) => {
    expect(nextRowIndex(count, current)).toBe(expected)
  })

  it("answers null when there is no row", () => {
    expect(nextRowIndex(0, null)).toBeNull()
    expect(nextRowIndex(0, 2)).toBeNull()
  })
})

describe("nextFolderDashboard", () => {
  const folder = [
    { id: "c", title: "Payments" },
    { id: "a", title: "Checkout" },
    { id: "b", title: "Inventory" },
  ]

  it.each([
    ["a", "b"],
    ["b", "c"],
    ["c", "a"],
  ])("after %s opens %s in title order, wrapping", (current, expected) => {
    expect(nextFolderDashboard(folder, current)?.id).toBe(expected)
  })

  it("orders equal titles by id so the cycle is stable", () => {
    const twins = [
      { id: "y", title: "Same" },
      { id: "x", title: "Same" },
    ]
    expect(nextFolderDashboard(twins, "x")?.id).toBe("y")
    expect(nextFolderDashboard(twins, "y")?.id).toBe("x")
  })

  it("starts at the first title when the current dashboard is not listed", () => {
    expect(nextFolderDashboard(folder, "zzz")?.id).toBe("a")
  })

  it("answers null when the folder holds only this dashboard or nothing", () => {
    expect(
      nextFolderDashboard([{ id: "a", title: "Checkout" }], "a")
    ).toBeNull()
    expect(nextFolderDashboard([], "a")).toBeNull()
  })
})

describe("kioskStep", () => {
  it.each([
    ["fullscreen-changed", true, false, "leave-kiosk"],
    ["fullscreen-changed", true, true, null],
    ["fullscreen-changed", false, false, null],
    ["fullscreen-changed", false, true, null],
    ["kiosk-changed", false, true, "exit-fullscreen"],
    ["kiosk-changed", false, false, null],
    ["kiosk-changed", true, true, null],
    ["kiosk-changed", true, false, null],
    ["fullscreen-key", true, false, "enter-fullscreen"],
    ["fullscreen-key", true, true, null],
    ["fullscreen-key", false, false, null],
    ["fullscreen-key", false, true, null],
  ] as const)(
    "on %s with kiosk %s and fullscreen %s takes %s",
    (event, kiosk, fullscreen, expected) => {
      expect(kioskStep(event, { kiosk, fullscreen })).toBe(expected)
    }
  )
})
