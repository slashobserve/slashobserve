import { describe, expect, it, vi } from "vitest"

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

  it("keeps arrows, Space and Enter from the grid while a button in the cell has focus", () => {
    for (const key of [
      "Enter",
      " ",
      "ArrowUp",
      "ArrowDown",
      "ArrowLeft",
      "ArrowRight",
    ]) {
      expect(keyOn(button, key)).toBe(true)
    }
  })

  it("lets the grid move on arrows from a link, and keeps Enter and Space on it", () => {
    const link = document.createElement("a")
    link.href = "/somewhere"
    cell.append(link)
    for (const key of ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"]) {
      expect(keyOn(link, key), key).toBe(false)
    }
    for (const key of ["Enter", " "]) {
      expect(keyOn(link, key), key).toBe(true)
    }
    link.remove()
  })

  it("lets the grid handle keys on the cell itself and other keys on a control", () => {
    expect(keyOn(cell, "ArrowDown")).toBe(false)
    expect(keyOn(cell, "Tab")).toBe(false)
    expect(keyOn(button, "PageDown")).toBe(false)
  })

  it("lets the grid take a Tab the cell does not use, so its focused cell follows", () => {
    expect(keyOn(button, "Tab")).toBe(false)
  })
})

describe("fields inside a cell", () => {
  function fieldInCell(tag: "input" | "textarea" | "select" | "div") {
    const grid = document.createElement("div")
    const cell = document.createElement("div")
    cell.className = "ag-cell"
    const field = document.createElement(tag)
    if (tag === "div") field.setAttribute("contenteditable", "true")
    cell.append(field)
    grid.append(cell)
    return { grid, field }
  }

  function keydown(target: HTMLElement, key: string) {
    const event = new KeyboardEvent("keydown", { key, cancelable: true })
    Object.defineProperty(event, "target", { value: target })
    return event
  }

  it("leaves Enter in a field to the field, so its form submits", () => {
    for (const tag of ["input", "textarea", "select", "div"] as const) {
      const { grid, field } = fieldInCell(tag)
      const clicked = vi.fn()
      field.addEventListener("click", clicked)
      const event = keydown(field, "Enter")
      handleCellKey(event, grid)
      expect(event.defaultPrevented).toBe(false)
      expect(clicked).not.toHaveBeenCalled()
    }
  })

  it("keeps Home, End, PageUp and PageDown from the grid while a field has focus", () => {
    const { field } = fieldInCell("input")
    const { field: editable } = fieldInCell("div")
    for (const key of ["Home", "End", "PageUp", "PageDown"]) {
      expect(keepKeysOnFocusedControl({ event: keydown(field, key) })).toBe(
        true
      )
      expect(keepKeysOnFocusedControl({ event: keydown(editable, key) })).toBe(
        true
      )
    }
  })

  it("leaves Home and End to the grid on a button", () => {
    const cell = document.createElement("div")
    cell.className = "ag-cell"
    const button = document.createElement("button")
    cell.append(button)
    expect(keepKeysOnFocusedControl({ event: keydown(button, "Home") })).toBe(
      false
    )
  })
})
