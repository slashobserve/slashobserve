import { fireEvent, render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { TruncatedText } from "./truncated-text"

function widths(element: HTMLElement, scroll: number, client: number) {
  Object.defineProperty(element, "scrollWidth", {
    value: scroll,
    configurable: true,
  })
  Object.defineProperty(element, "clientWidth", {
    value: client,
    configurable: true,
  })
}

describe("TruncatedText", () => {
  it("carries no title while its text fits, so short values are read once", () => {
    render(<TruncatedText>admin</TruncatedText>)
    const text = screen.getByText("admin")
    expect(text).toHaveClass("truncate")
    widths(text, 40, 120)
    fireEvent.pointerEnter(text)
    expect(text).not.toHaveAttribute("title")
  })

  it("gets the full text as its title on hover while it is cut, and drops it when it fits again", () => {
    render(<TruncatedText>Observe Agent for admin</TruncatedText>)
    const text = screen.getByText("Observe Agent for admin")
    widths(text, 200, 80)
    fireEvent.pointerEnter(text)
    expect(text).toHaveAttribute("title", "Observe Agent for admin")
    widths(text, 80, 80)
    fireEvent.pointerEnter(text)
    expect(text).not.toHaveAttribute("title")
  })
})
