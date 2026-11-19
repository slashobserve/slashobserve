import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { moveFocusToPage } from "./focus-after-navigation"

type Spec = [tag: string, text?: string]

function page(...specs: Spec[]) {
  document.body.replaceChildren(
    ...specs.map(([tag, text]) => {
      const element = document.createElement(tag)
      if (text) element.textContent = text
      return element
    })
  )
}

function main(...children: Spec[]) {
  const element = document.createElement("main")
  element.append(
    ...children.map(([tag, text]) => {
      const child = document.createElement(tag)
      if (text) child.textContent = text
      return child
    })
  )
  document.body.append(element)
  return element
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  document.body.replaceChildren()
})

describe("moveFocusToPage", () => {
  it("focuses the page heading when focus fell to the body", () => {
    page(["nav"])
    main(["h1", "Slow pods"])
    moveFocusToPage(document)
    expect(document.activeElement?.textContent).toBe("Slow pods")
    expect(document.activeElement?.getAttribute("tabindex")).toBe("-1")
  })

  it("leaves focus where the person or the page put it", () => {
    page()
    main(["h1", "Slow pods"], ["input"])
    document.querySelector("input")?.focus()
    moveFocusToPage(document)
    expect(document.activeElement?.tagName).toBe("INPUT")
  })

  it("waits for a heading that renders later", async () => {
    page()
    const target = main()
    moveFocusToPage(document)
    const heading = document.createElement("h1")
    heading.textContent = "Loaded"
    target.append(heading)
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement?.textContent).toBe("Loaded")
  })

  it("falls back to main when no heading appears", () => {
    page()
    main(["p", "Empty"])
    moveFocusToPage(document)
    expect(document.activeElement).toBe(document.body)
    vi.advanceTimersByTime(1500)
    expect(document.activeElement?.tagName).toBe("MAIN")
  })

  it("does not move focus once the person has moved it while waiting", () => {
    page()
    main(["p", "Empty"])
    document.body.append(document.createElement("button"))
    moveFocusToPage(document)
    document.querySelector("button")?.focus()
    vi.advanceTimersByTime(1500)
    expect(document.activeElement?.tagName).toBe("BUTTON")
  })

  it("stops waiting when cancelled", () => {
    page()
    main(["p", "Empty"])
    const stop = moveFocusToPage(document)
    stop()
    vi.advanceTimersByTime(1500)
    expect(document.activeElement).toBe(document.body)
  })
})
