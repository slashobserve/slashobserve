import { afterEach, describe, expect, it } from "vitest"

import { tabStaysInHeader } from "./header-keys"

type Control = "button" | "disabled" | "text"

function headerWith(controls: Control[], role = "columnheader"): HTMLElement {
  const header = document.createElement("div")
  header.setAttribute("role", role)
  header.tabIndex = -1
  for (const control of controls) {
    if (control === "text") {
      header.append("text")
    } else {
      const button = document.createElement("button")
      button.textContent = control
      button.disabled = control === "disabled"
      header.append(button)
    }
  }
  document.body.append(header)
  return header
}

function tabFrom(target: HTMLElement, shiftKey = false): boolean {
  let result = false
  target.addEventListener(
    "keydown",
    (event) => (result = tabStaysInHeader(event)),
    { once: true }
  )
  target.dispatchEvent(
    new KeyboardEvent("keydown", { bubbles: true, key: "Tab", shiftKey })
  )
  return result
}

afterEach(() => {
  document.body.replaceChildren()
})

describe("tabStaysInHeader", () => {
  it("lets Tab move from the header into its controls and between them", () => {
    const header = headerWith(["button", "button"])
    const [sort, filter] = header.querySelectorAll("button")
    expect(tabFrom(header)).toBe(true)
    expect(tabFrom(sort)).toBe(true)
    expect(tabFrom(filter)).toBe(false)
    expect(tabFrom(filter, true)).toBe(true)
    expect(tabFrom(sort, true)).toBe(false)
  })

  it("leaves Tab to the grid without enabled controls or outside a header", () => {
    expect(tabFrom(headerWith(["text"]))).toBe(false)
    expect(tabFrom(headerWith(["disabled"]))).toBe(false)
    expect(tabFrom(headerWith(["button"], "gridcell"))).toBe(false)
  })

  it("ignores other keys", () => {
    const header = headerWith(["button"])
    let result = true
    header.addEventListener("keydown", (event) => {
      result = tabStaysInHeader(event)
    })
    header.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight" }))
    expect(result).toBe(false)
  })
})
