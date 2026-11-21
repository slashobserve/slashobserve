import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ScrollPane } from "./scroll-pane"

describe("ScrollPane", () => {
  it("scrolls vertically and contains its absolutely positioned descendants", () => {
    render(
      <ScrollPane data-testid="pane" className="flex-1">
        <span className="sr-only">Copied</span>
      </ScrollPane>
    )
    const pane = screen.getByTestId("pane")
    expect(pane).toHaveClass("overflow-y-auto", "relative", "flex-1")
  })

  it("passes the attributes it is given to the element", () => {
    render(
      <ScrollPane role="region" aria-label="Conversation" tabIndex={-1}>
        text
      </ScrollPane>
    )
    expect(
      screen.getByRole("region", { name: "Conversation" })
    ).toHaveAttribute("tabindex", "-1")
  })
})
