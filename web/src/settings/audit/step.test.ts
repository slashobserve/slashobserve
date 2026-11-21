import { describe, expect, it } from "vitest"

import { neighborsOf, stepFromKey } from "./step"

const neighbors = { previous: "a", next: "c" }

function key(init: Partial<Parameters<typeof stepFromKey>[0]>) {
  return {
    key: "ArrowDown",
    defaultPrevented: false,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    target: document.body,
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
    expect(stepFromKey(key({ key: "ArrowUp" }), neighbors)).toBe("a")
    expect(stepFromKey(key({ key: "ArrowDown" }), neighbors)).toBe("c")
    expect(stepFromKey(key({ key: "Enter" }), neighbors)).toBe(null)
  })

  it.each([
    ["a modifier", { ctrlKey: true }],
    ["a handled key", { defaultPrevented: true }],
    ["an input", { target: document.createElement("input") }],
    ["a menu", { target: menuItem() }],
  ])("ignores %s", (_name, init) => {
    expect(stepFromKey(key(init), neighbors)).toBe(null)
  })
})

function menuItem(): HTMLElement {
  const menu = document.createElement("div")
  menu.setAttribute("role", "menu")
  const item = document.createElement("button")
  menu.append(item)
  return item
}
