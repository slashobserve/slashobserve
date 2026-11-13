import { renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useLogsShortcuts } from "./use-logs-shortcuts"

function press(target: Element | Window, init: KeyboardEventInit) {
  const event = new KeyboardEvent("keydown", {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  target.dispatchEvent(event)
  return event
}

function setup() {
  const focusQuery = vi.fn()
  const openHelp = vi.fn()
  const rendered = renderHook(() => useLogsShortcuts({ focusQuery, openHelp }))
  return { focusQuery, openHelp, ...rendered }
}

afterEach(() => {
  document.body.replaceChildren()
})

describe("useLogsShortcuts", () => {
  it("focuses the query on a slash outside an input and keeps the slash out of the page", () => {
    const { focusQuery, openHelp } = setup()
    const event = press(document.body, { key: "/" })
    expect(focusQuery).toHaveBeenCalledOnce()
    expect(openHelp).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
  })

  it("opens the help on a question mark", () => {
    const { focusQuery, openHelp } = setup()
    press(document.body, { key: "?", shiftKey: true })
    expect(openHelp).toHaveBeenCalledOnce()
    expect(focusQuery).not.toHaveBeenCalled()
  })

  it.each([
    ["an input", () => document.createElement("input")],
    ["a text area", () => document.createElement("textarea")],
    [
      "the query editor",
      () => {
        const editor = document.createElement("div")
        editor.contentEditable = "true"
        Object.defineProperty(editor, "isContentEditable", { value: true })
        return editor
      },
    ],
  ])("ignores the keys typed into %s", (_label, create) => {
    const { focusQuery, openHelp } = setup()
    const field = create()
    document.body.append(field)
    press(field, { key: "/" })
    press(field, { key: "?", shiftKey: true })
    expect(focusQuery).not.toHaveBeenCalled()
    expect(openHelp).not.toHaveBeenCalled()
  })

  it.each([
    ["Control", { ctrlKey: true }],
    ["Meta", { metaKey: true }],
    ["Alt", { altKey: true }],
    ["a repeat", { repeat: true }],
  ])("ignores a slash with %s", (_label, init) => {
    const { focusQuery } = setup()
    press(document.body, { key: "/", ...init })
    expect(focusQuery).not.toHaveBeenCalled()
  })

  it("ignores the keys while a dialog is open", () => {
    const { focusQuery, openHelp } = setup()
    const dialog = document.createElement("div")
    dialog.setAttribute("role", "dialog")
    document.body.append(dialog)
    press(document.body, { key: "/" })
    press(document.body, { key: "?", shiftKey: true })
    expect(focusQuery).not.toHaveBeenCalled()
    expect(openHelp).not.toHaveBeenCalled()
  })

  it("stops listening when the page unmounts", () => {
    const { focusQuery, unmount } = setup()
    unmount()
    press(document.body, { key: "/" })
    expect(focusQuery).not.toHaveBeenCalled()
  })

  it("uses the handlers of the latest render", () => {
    const first = vi.fn()
    const second = vi.fn()
    const { rerender } = renderHook(
      ({ focusQuery }) => useLogsShortcuts({ focusQuery, openHelp: vi.fn() }),
      { initialProps: { focusQuery: first } }
    )
    rerender({ focusQuery: second })
    press(document.body, { key: "/" })
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledOnce()
  })
})
