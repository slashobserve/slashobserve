import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { shortcutTarget } from "./use-dashboard-shortcuts"
import { useHoveredPanel } from "./hovered-panel"

afterEach(() => {
  useHoveredPanel.setState({ target: null })
  cleanup()
})

function focusPanel(repeatOf: string | undefined) {
  render(
    <article data-panel-id="p1" data-repeat-of={repeatOf}>
      <button type="button">Title</button>
    </article>
  )
  screen.getByRole("button", { name: "Title" }).focus()
}

describe("shortcutTarget", () => {
  it("says whether the hovered panel is a repeated panel's copy", () => {
    useHoveredPanel.getState().enter("p1", true)
    expect(shortcutTarget()).toEqual({ id: "p1", copy: true })
    useHoveredPanel.getState().leave("p1")
    expect(shortcutTarget()).toBeNull()
  })

  it("reads a copy from the focused panel when none is hovered", () => {
    focusPanel("p1")
    expect(shortcutTarget()).toEqual({ id: "p1", copy: true })
  })

  it("reads the original from the focused panel", () => {
    focusPanel(undefined)
    expect(shortcutTarget()).toEqual({ id: "p1", copy: false })
  })
})
