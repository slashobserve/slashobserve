import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { SkipLink } from "./skip-link"

describe("SkipLink", () => {
  it("is the first tab stop and moves focus to the page heading", async () => {
    const user = userEvent.setup()
    render(
      <>
        <SkipLink />
        <nav>
          <a href="/inbox">Inbox</a>
        </nav>
        <main>
          <h1>Dashboards</h1>
        </main>
      </>
    )
    await user.tab()
    expect(screen.getByRole("link", { name: "Skip to the page" })).toHaveFocus()
    await user.keyboard("{Enter}")
    expect(screen.getByRole("heading", { name: "Dashboards" })).toHaveFocus()
  })
})
