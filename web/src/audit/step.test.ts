import { describe, expect, it } from "vitest"

import { neighborsOf, stepFromKey } from "./step"

const neighbors = { previous: "a", next: "c" }

function inTable(): HTMLElement {
  const table = document.createElement("div")
  table.setAttribute("role", "region")
  table.setAttribute("aria-label", "Audit events")
  const row = document.createElement("button")
  table.append(row)
  return row
}

function key(init: Partial<Parameters<typeof stepFromKey>[0]>) {
  return {
    key: "ArrowDown",
    defaultPrevented: false,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    target: inTable(),
    ...init,
  }
}

describe("neighborsOf", () => {
  it("names the rows above and below the selected one", () => {
    expect(neighborsOf(["a", "b", "c"], "b")).toEqual(neighbors)
    expect(neighborsOf(["a", "b", "c"], "a")).toEqual({
      previous: null,
      next: "b",
    })
    expect(neighborsOf(["a", "b", "c"], "c").next).toBe(null)
  })

  it("has none for an event the list does not hold", () => {
    expect(neighborsOf(["a"], "z")).toEqual({ previous: null, next: null })
  })
})

describe("stepFromKey", () => {
  it("maps Up to the previous event and Down to the next", () => {
    expect(stepFromKey(key({ key: "ArrowUp" }), neighbors, null)).toBe("a")
    expect(stepFromKey(key({ key: "ArrowDown" }), neighbors, null)).toBe("c")
    expect(stepFromKey(key({ key: "Enter" }), neighbors, null)).toBe(null)
  })

  it.each([
    ["a modifier", { ctrlKey: true }],
    ["a handled key", { defaultPrevented: true }],
    ["an input", { target: document.createElement("input") }],
    ["a menu", { target: menuItem() }],
    ["the page body", { target: document.body }],
    ["a histogram bar", { target: document.createElement("button") }],
  ])("ignores %s", (_name, init) => {
    expect(stepFromKey(key(init), neighbors, null)).toBe(null)
  })
})

function menuItem(): HTMLElement {
  const menu = document.createElement("div")
  menu.setAttribute("role", "menu")
  const item = document.createElement("button")
  menu.append(item)
  return item
}

describe("stepFromKey in the detail panel", () => {
  it("steps while focus is inside the panel", () => {
    const panel = document.createElement("aside")
    const title = document.createElement("h2")
    panel.append(title)
    expect(stepFromKey(key({ target: title }), neighbors, panel)).toBe("c")
  })

  it("leaves focus outside both areas alone", () => {
    const panel = document.createElement("aside")
    expect(
      stepFromKey(
        key({ target: document.createElement("button") }),
        neighbors,
        panel
      )
    ).toBe(null)
  })
})
