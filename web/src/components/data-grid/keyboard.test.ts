import { afterEach, describe, expect, it, vi } from "vitest"

import { clickCellControl, tabStaysInCell } from "./keyboard"

type Control = "link" | "button" | "disabled" | "text"

function cellWith(controls: Control[], role = "gridcell"): HTMLElement {
  const cell = document.createElement("div")
  cell.setAttribute("role", role)
  cell.tabIndex = -1
  for (const control of controls) {
    if (control === "text") {
      cell.append("text")
    } else if (control === "link") {
      const link = document.createElement("a")
      link.href = "#x"
      link.textContent = "Open"
      cell.append(link)
    } else {
      const button = document.createElement("button")
      button.textContent = control
      button.disabled = control === "disabled"
      cell.append(button)
    }
  }
  document.body.append(cell)
  return cell
}

function pressOn(
  target: HTMLElement,
  init: KeyboardEventInit,
  handle: (event: KeyboardEvent) => boolean
): boolean {
  let result = false
  target.addEventListener("keydown", (event) => (result = handle(event)), {
    once: true,
  })
  target.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, ...init }))
  return result
}

afterEach(() => {
  document.body.replaceChildren()
})

describe("tabStaysInCell", () => {
  it("lets Tab move from the cell into its controls and between them", () => {
    const cell = cellWith(["button", "button"])
    const [sort, filter] = cell.querySelectorAll("button")
    expect(pressOn(cell, { key: "Tab" }, tabStaysInCell)).toBe(true)
    expect(pressOn(sort, { key: "Tab" }, tabStaysInCell)).toBe(true)
    expect(pressOn(filter, { key: "Tab" }, tabStaysInCell)).toBe(false)
    expect(
      pressOn(filter, { key: "Tab", shiftKey: true }, tabStaysInCell)
    ).toBe(true)
    expect(pressOn(sort, { key: "Tab", shiftKey: true }, tabStaysInCell)).toBe(
      false
    )
  })

  it("leaves Tab to the grid in a cell without controls, and in a header", () => {
    expect(pressOn(cellWith(["text"]), { key: "Tab" }, tabStaysInCell)).toBe(
      false
    )
    const header = cellWith(["disabled"], "columnheader")
    expect(pressOn(header, { key: "Tab" }, tabStaysInCell)).toBe(false)
    const sorting = cellWith(["button"], "columnheader")
    expect(pressOn(sorting, { key: "Tab" }, tabStaysInCell)).toBe(true)
  })

  it("ignores other keys", () => {
    const cell = cellWith(["button"])
    expect(pressOn(cell, { key: "ArrowRight" }, tabStaysInCell)).toBe(false)
  })
})

describe("clickCellControl", () => {
  it("clicks the first link or button of a cell on Enter", () => {
    const cell = cellWith(["link", "button"])
    const clicked = vi.fn((event: Event) => event.preventDefault())
    cell.querySelector("a")?.addEventListener("click", clicked)
    expect(pressOn(cell, { key: "Enter" }, clickCellControl)).toBe(true)
    expect(clicked).toHaveBeenCalledTimes(1)
  })

  it("does nothing for a cell without a control or for a control itself", () => {
    expect(
      pressOn(cellWith(["text"]), { key: "Enter" }, clickCellControl)
    ).toBe(false)
    const cell = cellWith(["button"])
    const button = cell.querySelector("button") as HTMLElement
    expect(pressOn(button, { key: "Enter" }, clickCellControl)).toBe(false)
  })
})
