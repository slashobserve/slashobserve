import { act, render } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ScrollWithMoreCue, hasMoreBelow } from "./scroll-with-more-cue"

describe("hasMoreBelow", () => {
  it("is true while content is hidden under the bottom edge", () => {
    expect(
      hasMoreBelow({ scrollTop: 0, clientHeight: 536, scrollHeight: 736 })
    ).toBe(true)
  })

  it("is false once scrolled to the end", () => {
    expect(
      hasMoreBelow({ scrollTop: 200, clientHeight: 536, scrollHeight: 736 })
    ).toBe(false)
  })

  it("ignores a sub-pixel remainder", () => {
    expect(
      hasMoreBelow({ scrollTop: 199.5, clientHeight: 536, scrollHeight: 736 })
    ).toBe(false)
  })

  it("is false when everything fits", () => {
    expect(
      hasMoreBelow({ scrollTop: 0, clientHeight: 536, scrollHeight: 536 })
    ).toBe(false)
  })
})

describe("ScrollWithMoreCue", () => {
  it("marks itself while rows are hidden below the bottom edge", () => {
    const { container } = render(<ScrollWithMoreCue>rows</ScrollWithMoreCue>)
    const element = container.firstElementChild as HTMLDivElement
    expect(element.hasAttribute("data-more-below")).toBe(false)
    Object.defineProperty(element, "clientHeight", { value: 100 })
    Object.defineProperty(element, "scrollHeight", { value: 300 })
    act(() => {
      element.dispatchEvent(new Event("scroll"))
    })
    expect(element.hasAttribute("data-more-below")).toBe(true)
    expect(container.querySelector("[data-more-cue]")).not.toBeNull()
    Object.defineProperty(element, "scrollTop", { value: 200 })
    act(() => {
      element.dispatchEvent(new Event("scroll"))
    })
    expect(element.hasAttribute("data-more-below")).toBe(false)
    expect(container.querySelector("[data-more-cue]")).toBeNull()
  })

  it("recomputes when rows are added", async () => {
    const { container } = render(<ScrollWithMoreCue>rows</ScrollWithMoreCue>)
    const element = container.firstElementChild as HTMLDivElement
    Object.defineProperty(element, "clientHeight", { value: 100 })
    Object.defineProperty(element, "scrollHeight", { value: 300 })
    await act(async () => {
      element.append(document.createElement("a"))
    })
    expect(element.hasAttribute("data-more-below")).toBe(true)
  })
})
