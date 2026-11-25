import { describe, expect, it } from "vitest"

import { cellKeyAction } from "./cell-keys"

describe("cellKeyAction", () => {
  it("presses the only control of a focused cell on Enter", () => {
    expect(cellKeyAction("Enter", false, 0, 1)).toEqual({
      kind: "click",
      stop: 1,
    })
  })

  it("leaves Enter to the grid on a cell with no control or several", () => {
    expect(cellKeyAction("Enter", false, 0, 0)).toBeNull()
    expect(cellKeyAction("Enter", false, 0, 3)).toBeNull()
  })

  it("presses the focused control on Enter", () => {
    expect(cellKeyAction("Enter", false, 2, 3)).toEqual({
      kind: "click",
      stop: 2,
    })
  })

  it("tabs through the controls of a cell that holds several, then lets the grid move on", () => {
    expect(cellKeyAction("Tab", false, 0, 2)).toEqual({
      kind: "focus",
      stop: 1,
    })
    expect(cellKeyAction("Tab", false, 1, 2)).toEqual({
      kind: "focus",
      stop: 2,
    })
    expect(cellKeyAction("Tab", false, 2, 2)).toBeNull()
  })

  it("tabs back through the controls to the cell, then lets the grid move on", () => {
    expect(cellKeyAction("Tab", true, 2, 2)).toEqual({ kind: "focus", stop: 1 })
    expect(cellKeyAction("Tab", true, 1, 2)).toEqual({ kind: "focus", stop: 0 })
    expect(cellKeyAction("Tab", true, 0, 2)).toBeNull()
  })

  it("leaves Tab to the grid on a cell with one control", () => {
    expect(cellKeyAction("Tab", false, 0, 1)).toBeNull()
  })

  it("ignores other keys and targets outside the cell's stops", () => {
    expect(cellKeyAction("ArrowDown", false, 1, 2)).toBeNull()
    expect(cellKeyAction("Enter", false, -1, 1)).toBeNull()
  })
})
