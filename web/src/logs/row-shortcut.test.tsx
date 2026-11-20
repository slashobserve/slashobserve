import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { useFirstRowShortcut } from "./row-shortcut"

function Page() {
  useFirstRowShortcut()
  return (
    <>
      <input aria-label="Query" />
      <ul data-logs-count="2">
        <li>
          <button type="button">first</button>
        </li>
        <li>
          <button type="button">second</button>
        </li>
      </ul>
    </>
  )
}

describe("useFirstRowShortcut", () => {
  it("moves focus to the first row when j is pressed on the page body", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.keyboard("j")
    expect(screen.getByRole("button", { name: "first" })).toHaveFocus()
  })

  it("leaves j alone while a field has focus", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.click(screen.getByRole("textbox", { name: "Query" }))
    await user.keyboard("j")
    expect(screen.getByRole("textbox", { name: "Query" })).toHaveValue("j")
  })
})
