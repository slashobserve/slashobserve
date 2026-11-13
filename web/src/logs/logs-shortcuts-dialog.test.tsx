import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { LogsShortcutsDialog, logsShortcuts } from "./logs-shortcuts-dialog"

describe("logsShortcuts", () => {
  it.each([
    ["Macintosh", "⌘Enter", "⌘I"],
    ["Windows", "Ctrl Enter", "Ctrl I"],
  ])("writes the modifier chords for %s", (userAgent, run, mode) => {
    const keys = logsShortcuts(`Mozilla/5.0 (${userAgent})`).map(
      (row) => row.keys
    )
    expect(keys).toContain(run)
    expect(keys).toContain(mode)
  })
})

describe("LogsShortcutsDialog", () => {
  it("lists every logs key in a dialog named Keyboard shortcuts", async () => {
    renderWithProviders(<LogsShortcutsDialog open onOpenChange={() => {}} />)
    const dialog = await screen.findByRole("dialog", {
      name: "Keyboard shortcuts",
    })
    const terms = within(dialog)
      .getAllByRole("term")
      .map((term) => term.textContent)
    expect(terms).toEqual(
      expect.arrayContaining(["/", "j / k", "Enter", "Esc", "?", "Ctrl Space"])
    )
    expect(within(dialog).getAllByRole("definition")).toHaveLength(8)
  })

  it("renders nothing while closed", () => {
    renderWithProviders(
      <LogsShortcutsDialog open={false} onOpenChange={() => {}} />
    )
    expect(screen.queryByRole("dialog")).toBeNull()
  })
})
