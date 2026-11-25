import { describe, expect, it } from "vitest"

import {
  cellKeyAction,
  handleCellKey,
  keepKeysOnFocusedControl,
} from "./cell-keys"

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

  it("tabs through the controls of a cell that holds several, and stops acting after the last", () => {
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

  it("tabs back through the controls to the cell, and stops acting there", () => {
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

describe("handleCellKey", () => {
  it("leaves Enter in a field inside a cell to the field, so its form submits", () => {
    const grid = document.createElement("div")
    const cell = document.createElement("div")
    cell.className = "ag-cell"
    const input = document.createElement("input")
    cell.append(input)
    grid.append(cell)
    document.body.append(grid)
    const event = new KeyboardEvent("keydown", {
      key: "Enter",
      bubbles: true,
      cancelable: true,
    })
    Object.defineProperty(event, "target", { value: input })
    handleCellKey(event, grid)
    expect(event.defaultPrevented).toBe(false)
    grid.remove()
  })
})

describe("keepKeysOnFocusedControl", () => {
  function keyOn(target: HTMLElement, key: string) {
    const event = new KeyboardEvent("keydown", { key })
    Object.defineProperty(event, "target", { value: target })
    return keepKeysOnFocusedControl({ event })
  }

  const cell = document.createElement("div")
  cell.className = "ag-cell"
  const button = document.createElement("button")
  cell.append(button)

  it("keeps navigation, paging, Space and Enter from the grid while a control in the cell has focus", () => {
    for (const key of [
      "Tab",
      "Enter",
      " ",
      "ArrowUp",
      "ArrowDown",
      "ArrowLeft",
      "ArrowRight",
      "Home",
      "End",
      "PageUp",
      "PageDown",
    ]) {
      expect(keyOn(button, key)).toBe(true)
    }
  })

  it("lets the grid handle keys on the cell itself and other keys on a control", () => {
    expect(keyOn(cell, "ArrowDown")).toBe(false)
    expect(keyOn(cell, "Tab")).toBe(false)
    expect(keyOn(button, "a")).toBe(false)
  })
})
