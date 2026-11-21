import { act, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AuditFilterBar } from "./audit-filter-bar"

function press(content: HTMLElement, init: KeyboardEventInit) {
  act(() => {
    content.dispatchEvent(
      new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init })
    )
  })
}

function setup() {
  const onRun = vi.fn()
  const onDraft = vi.fn()
  renderWithProviders(
    <AuditFilterBar
      draft="actor:admin"
      onDraft={onDraft}
      onRun={onRun}
      problem={null}
    />
  )
  const content = screen.getByRole("textbox", { name: /Audit query/ })
  content.focus()
  return { content, onRun, onDraft }
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ fields: [] }), {
          headers: { "content-type": "application/json" },
        })
      )
    )
  )
  return () => vi.unstubAllGlobals()
})

describe("AuditFilterBar keys", () => {
  it("runs the search on Enter without adding a line", () => {
    const { content, onRun, onDraft } = setup()
    press(content, { key: "Enter", code: "Enter" })
    expect(onRun).toHaveBeenCalledTimes(1)
    expect(onDraft).not.toHaveBeenCalled()
  })

  it.each([
    ["Meta", { metaKey: true }],
    ["Control", { ctrlKey: true }],
  ])("keeps %s+Enter running the search", (_name, modifier) => {
    const { content, onRun } = setup()
    press(content, { key: "Enter", code: "Enter", ...modifier })
    expect(onRun).toHaveBeenCalledTimes(1)
  })

  it.each([
    ["isComposing", { isComposing: true }],
    ["keyCode 229", { keyCode: 229 }],
  ])(
    "does not search on an Enter that confirms IME input (%s)",
    (_name, init) => {
      const { content, onRun } = setup()
      press(content, { key: "Enter", code: "Enter", ...init })
      expect(onRun).not.toHaveBeenCalled()
    }
  )

  it("adds a line on Shift+Enter instead of searching", () => {
    const { content, onRun } = setup()
    press(content, { key: "Enter", code: "Enter", shiftKey: true })
    expect(onRun).not.toHaveBeenCalled()
  })
})
