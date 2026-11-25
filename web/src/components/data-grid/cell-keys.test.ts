import { afterEach, describe, expect, it, vi } from "vitest"

import type { FocusGridInnerElementParams } from "ag-grid-community"

import {
  cellKeyAction,
  focusEntryCell,
  handleCellKey,
  keepKeysOnFocusedControl,
  leaveGridOnTab,
} from "./cell-keys"

describe("cellKeyAction", () => {
  it("presses the only control of a focused cell on Enter", () => {
    expect(cellKeyAction("Enter", false, 0, 1)).toEqual({
      kind: "click",
      stop: 1,
    })
  })

  it("leaves Enter to the grid on a cell with no control", () => {
    expect(cellKeyAction("Enter", false, 0, 0)).toBeNull()
  })

  it("enters a cell that holds several controls on Enter, at the first", () => {
    expect(cellKeyAction("Enter", false, 0, 3)).toEqual({
      kind: "focus",
      stop: 1,
    })
  })

  it("returns from a control to its cell on Escape, and ignores Escape on the cell", () => {
    expect(cellKeyAction("Escape", false, 2, 3)).toEqual({
      kind: "focus",
      stop: 0,
    })
    expect(cellKeyAction("Escape", false, 0, 3)).toBeNull()
    expect(cellKeyAction("Escape", false, -1, 3)).toBeNull()
  })

  it("presses the focused control on Enter", () => {
    expect(cellKeyAction("Enter", false, 2, 3)).toEqual({
      kind: "click",
      stop: 2,
    })
  })

  it("leaves the grid on Tab from the cell itself, however many controls it holds", () => {
    for (const count of [0, 1, 2, 5]) {
      expect(cellKeyAction("Tab", false, 0, count)).toEqual({ kind: "leave" })
      expect(cellKeyAction("Tab", true, 0, count)).toEqual({ kind: "leave" })
    }
    expect(cellKeyAction("Tab", false, -1, 2)).toEqual({ kind: "leave" })
  })

  it("tabs between the controls once one has focus, and leaves after the last or before the first", () => {
    expect(cellKeyAction("Tab", false, 1, 2)).toEqual({
      kind: "focus",
      stop: 2,
    })
    expect(cellKeyAction("Tab", false, 2, 2)).toEqual({ kind: "leave" })
    expect(cellKeyAction("Tab", true, 2, 2)).toEqual({ kind: "focus", stop: 1 })
    expect(cellKeyAction("Tab", true, 1, 2)).toEqual({ kind: "leave" })
    expect(cellKeyAction("Tab", false, 1, 1)).toEqual({ kind: "leave" })
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

describe("leaving the grid on Tab", () => {
  function page() {
    const before = document.createElement("button")
    const after = document.createElement("button")
    const grid = document.createElement("div")
    const header = document.createElement("div")
    header.setAttribute("role", "columnheader")
    const rows = [0, 1].map(() => {
      const cell = document.createElement("div")
      cell.className = "ag-cell"
      cell.tabIndex = -1
      const link = document.createElement("a")
      link.href = "#row"
      cell.append(link)
      return { cell, link }
    })
    grid.append(header, ...rows.map((row) => row.cell))
    document.body.append(before, grid, after)
    return { before, after, grid, header, rows }
  }

  function tab(target: HTMLElement, grid: HTMLElement, shiftKey = false) {
    const event = new KeyboardEvent("keydown", {
      key: "Tab",
      shiftKey,
      cancelable: true,
    })
    Object.defineProperty(event, "target", { value: target })
    handleCellKey(event, grid)
    return event
  }

  afterEach(() => {
    document.body.replaceChildren()
  })

  it("moves focus to the next control after the grid, past every row's own controls", () => {
    const { after, grid, rows } = page()
    const event = tab(rows[0]!.cell, grid)
    expect(event.defaultPrevented).toBe(true)
    expect(after).toHaveFocus()
  })

  it("moves focus to the control before the grid on Shift+Tab", () => {
    const { before, grid, rows } = page()
    const event = tab(rows[1]!.cell, grid, true)
    expect(event.defaultPrevented).toBe(true)
    expect(before).toHaveFocus()
  })

  it("leaves from a header cell too", () => {
    const { after, grid, header } = page()
    expect(tab(header, grid).defaultPrevented).toBe(true)
    expect(after).toHaveFocus()
  })

  it("leaves from a cell holding a checkbox and a link on one Tab, either way", () => {
    const { after, before, grid, rows } = page()
    const cell = rows[0]!.cell
    const checkbox = document.createElement("input")
    checkbox.type = "checkbox"
    cell.prepend(checkbox)
    expect(tab(cell, grid).defaultPrevented).toBe(true)
    expect(after).toHaveFocus()
    expect(tab(cell, grid, true).defaultPrevented).toBe(true)
    expect(before).toHaveFocus()
  })

  it("enters such a cell on Enter and steps between its controls", () => {
    const { after, grid, rows } = page()
    const cell = rows[0]!.cell
    const checkbox = document.createElement("input")
    checkbox.type = "checkbox"
    cell.prepend(checkbox)
    const enter = new KeyboardEvent("keydown", {
      key: "Enter",
      cancelable: true,
    })
    Object.defineProperty(enter, "target", { value: cell })
    handleCellKey(enter, grid)
    expect(checkbox).toHaveFocus()
    expect(tab(checkbox, grid).defaultPrevented).toBe(true)
    expect(rows[0]!.link).toHaveFocus()
    expect(tab(rows[0]!.link, grid).defaultPrevented).toBe(true)
    expect(after).toHaveFocus()
  })

  it("skips controls that are out of the tab order or hidden", () => {
    const { before, grid, rows } = page()
    const removed = document.createElement("button")
    removed.tabIndex = -1
    const hidden = document.createElement("button")
    hidden.setAttribute("aria-hidden", "true")
    before.after(removed, hidden)
    const event = tab(rows[0]!.cell, grid, true)
    expect(event.defaultPrevented).toBe(true)
    expect(before).toHaveFocus()
  })

  it("tabs from a textarea to the control after it in the same row before leaving", () => {
    const { after, grid, rows } = page()
    const cell = rows[0]!.cell
    const field = document.createElement("textarea")
    const save = document.createElement("button")
    cell.replaceChildren(field, save)
    field.focus()
    expect(tab(field, grid).defaultPrevented).toBe(true)
    expect(save).toHaveFocus()
    expect(tab(save, grid).defaultPrevented).toBe(true)
    expect(after).toHaveFocus()
  })

  it("treats a child of an editable region as that region", () => {
    const { grid, rows } = page()
    const cell = rows[0]!.cell
    const editor = document.createElement("div")
    editor.setAttribute("contenteditable", "true")
    const word = document.createElement("b")
    editor.append(word)
    const save = document.createElement("button")
    cell.replaceChildren(editor, save)
    tab(word, grid)
    expect(save).toHaveFocus()
  })

  it("leaves a nested grid's keys to the nested grid", () => {
    const { after, grid, rows } = page()
    const outer = document.createElement("div")
    outer.setAttribute("role", "grid")
    grid.replaceChildren(outer)
    const inner = document.createElement("div")
    inner.setAttribute("role", "grid")
    const cell = document.createElement("div")
    cell.className = "ag-cell"
    inner.append(cell)
    outer.append(rows[0]!.cell, inner)
    const event = tab(cell, grid)
    expect(event.defaultPrevented).toBe(false)
    expect(after).not.toHaveFocus()
  })

  it("leaves the event alone when nothing follows the grid", () => {
    const { after, grid, rows } = page()
    after.remove()
    expect(tab(rows[0]!.cell, grid).defaultPrevented).toBe(false)
  })

  it("never lets the grid's own tab navigation pick the next cell", () => {
    expect(leaveGridOnTab()).toBe(false)
  })
})

describe("focusEntryCell", () => {
  const columns = ["name", "owner"].map((id) => ({ getColId: () => id }))

  function api(
    focused: { rowIndex: number; column: string; rowPinned?: string } | null,
    rowCount: number,
    displayed = columns,
    pinnedBottom = 0
  ) {
    const setFocusedCell = vi.fn()
    return {
      setFocusedCell,
      params: {
        api: {
          getAllDisplayedColumns: () => displayed,
          getDisplayedRowCount: () => rowCount,
          getPinnedBottomRowCount: () => pinnedBottom,
          getFocusedCell: () =>
            focused === null
              ? null
              : {
                  rowPinned: null,
                  ...focused,
                  column: { getColId: () => focused.column },
                },
          setFocusedCell,
        },
      } as unknown as FocusGridInnerElementParams,
    }
  }

  it("returns to the last focused cell", () => {
    const { params, setFocusedCell } = api({ rowIndex: 4, column: "owner" }, 9)
    expect(focusEntryCell(params)).toBe(true)
    expect(setFocusedCell).toHaveBeenCalledWith(4, columns[1], null)
  })

  it("starts at the first displayed cell when none was focused or its row is gone", () => {
    for (const focused of [null, { rowIndex: 9, column: "owner" }]) {
      const { params, setFocusedCell } = api(focused, 3)
      expect(focusEntryCell(params)).toBe(true)
      expect(setFocusedCell).toHaveBeenCalledWith(0, columns[0])
    }
  })

  it("falls back to the first displayed column when the focused one is hidden", () => {
    const { params, setFocusedCell } = api(
      { rowIndex: 1, column: "owner" },
      3,
      columns.slice(0, 1)
    )
    expect(focusEntryCell(params)).toBe(true)
    expect(setFocusedCell).toHaveBeenCalledWith(1, columns[0], null)
  })

  it("checks a pinned footer row against the footer count", () => {
    const footer = { rowIndex: 1, column: "name", rowPinned: "bottom" }
    const kept = api(footer, 5, columns, 2)
    expect(focusEntryCell(kept.params)).toBe(true)
    expect(kept.setFocusedCell).toHaveBeenCalledWith(1, columns[0], "bottom")
    const gone = api(footer, 5, columns, 1)
    expect(focusEntryCell(gone.params)).toBe(true)
    expect(gone.setFocusedCell).toHaveBeenCalledWith(0, columns[0])
  })

  it("leaves the entry to the grid without rows or columns", () => {
    expect(focusEntryCell(api(null, 0).params)).toBe(false)
    expect(focusEntryCell(api(null, 3, []).params)).toBe(false)
  })
})
