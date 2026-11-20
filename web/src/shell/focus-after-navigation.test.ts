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

const started: Array<() => void> = []

function start() {
  const stop = moveFocusToPage(document)
  started.push(stop)
  return stop
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  for (const stop of started.splice(0)) stop()
  vi.useRealTimers()
  document.body.replaceChildren()
})

describe("moveFocusToPage", () => {
  it("focuses the page heading when focus fell to the body", () => {
    page(["nav"])
    main(["h1", "Slow pods"])
    start()
    expect(document.activeElement?.textContent).toBe("Slow pods")
    expect(document.activeElement?.getAttribute("tabindex")).toBe("-1")
  })

  it("leaves focus where the person or the page put it", () => {
    page()
    main(["h1", "Slow pods"], ["input"])
    document.querySelector("input")?.focus()
    start()
    expect(document.activeElement?.tagName).toBe("INPUT")
  })

  it("moves focus once the focused link leaves with the old page", async () => {
    page()
    const link = document.createElement("a")
    link.href = "#open"
    document.body.append(link)
    link.focus()
    const target = main(["h1", "Trace"])
    start()
    expect(document.activeElement).toBe(link)
    link.remove()
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement).toBe(target.querySelector("h1"))
  })

  it("keeps waiting for the heading after the focused link left", async () => {
    page()
    const link = document.createElement("a")
    link.href = "#open"
    document.body.append(link)
    link.focus()
    const target = main()
    start()
    link.remove()
    await vi.advanceTimersByTimeAsync(0)
    const heading = document.createElement("h1")
    heading.textContent = "Trace"
    target.append(heading)
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement).toBe(heading)
  })

  it("waits for a heading that renders later", async () => {
    page()
    const target = main()
    start()
    const heading = document.createElement("h1")
    heading.textContent = "Loaded"
    target.append(heading)
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement?.textContent).toBe("Loaded")
  })

  it("falls back to main when no heading appears", () => {
    page()
    main(["p", "Empty"])
    start()
    expect(document.activeElement).toBe(document.body)
    vi.advanceTimersByTime(1500)
    expect(document.activeElement?.tagName).toBe("MAIN")
  })

  it("does not move focus once the person has moved it while waiting", () => {
    page()
    main(["p", "Empty"])
    document.body.append(document.createElement("button"))
    start()
    document.querySelector("button")?.focus()
    vi.advanceTimersByTime(1500)
    expect(document.activeElement?.tagName).toBe("BUTTON")
  })

  it("focuses the new heading when the focused one is replaced while the page loads", async () => {
    page()
    const target = main(["h1", "Trace"])
    start()
    expect(document.activeElement?.tagName).toBe("H1")
    const replacement = document.createElement("h1")
    replacement.textContent = "checkout GET /pay"
    target.replaceChildren(replacement)
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement).toBe(replacement)
  })

  it("keeps the replaced heading's focus only for a few seconds", async () => {
    page()
    const target = main(["h1", "Trace"])
    start()
    await vi.advanceTimersByTimeAsync(6000)
    target.replaceChildren(document.createElement("h1"))
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement).toBe(document.body)
  })

  it("leaves focus alone once the person clicked the page", async () => {
    page()
    const target = main(["h1", "Trace"])
    start()
    document.dispatchEvent(new Event("pointerdown"))
    target.replaceChildren(document.createElement("h1"))
    await vi.advanceTimersByTimeAsync(0)
    expect(document.activeElement).toBe(document.body)
  })

  it("stops waiting when cancelled", () => {
    page()
    main(["p", "Empty"])
    const stop = start()
    stop()
    vi.advanceTimersByTime(1500)
    expect(document.activeElement).toBe(document.body)
  })
})
