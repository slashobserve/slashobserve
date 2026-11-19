import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ShortcutsDialog } from "./shortcuts-dialog"

describe("ShortcutsDialog", () => {
  it("lists the sidebar chord the docs describe", () => {
    render(<ShortcutsDialog open onOpenChange={() => undefined} />)
    const term = screen.getByText("Collapse or expand the sidebar")
    expect(term.previousElementSibling?.textContent).toMatch(/[⌘Ctrl] ?\\/)
  })
})
