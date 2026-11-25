import { afterEach, beforeEach, describe, expect, it } from "vitest"

import {
  inboxAfterNavigation,
  returnFocusFromInbox,
  useInboxPanelStore,
} from "./inbox-panel-store"

const closed = {
  open: false,
  view: "for_you" as const,
  filter: "open" as const,
  item: null,
  opener: null,
  pending: null,
}

beforeEach(() => useInboxPanelStore.setState(closed))

afterEach(() => document.body.replaceChildren())

describe("inboxAfterNavigation", () => {
  it("opens the choice an old Inbox link left once the redirect lands", () => {
    useInboxPanelStore
      .getState()
      .showAfterNavigation({ view: "mentions", item: 7 })
    expect(useInboxPanelStore.getState().open).toBe(false)

    inboxAfterNavigation("/", true)

    expect(useInboxPanelStore.getState()).toMatchObject({
      open: true,
      view: "mentions",
      item: 7,
      pending: null,
    })
  })

  it("waits past the /inbox location itself", () => {
    useInboxPanelStore.getState().showAfterNavigation({ view: "proposals" })

    inboxAfterNavigation("/inbox", true)

    expect(useInboxPanelStore.getState().open).toBe(false)
    expect(useInboxPanelStore.getState().pending).toEqual({
      view: "proposals",
    })
  })

  it("closes the panel when the person moves to another page", () => {
    useInboxPanelStore.getState().show({ item: 3 })

    inboxAfterNavigation("/d/a1b2", true)

    expect(useInboxPanelStore.getState()).toMatchObject({
      open: false,
      item: null,
    })
  })

  it("keeps the panel open when only the search changes", () => {
    useInboxPanelStore.getState().show()

    inboxAfterNavigation("/d/a1b2", false)

    expect(useInboxPanelStore.getState().open).toBe(true)
  })
})

describe("returnFocusFromInbox", () => {
  function button(attributes: Record<string, string> = {}) {
    const element = document.createElement("button")
    for (const [name, value] of Object.entries(attributes)) {
      element.setAttribute(name, value)
    }
    document.body.append(element)
    return element
  }

  it("focuses the element that opened the panel", () => {
    button({ "data-inbox-bell": "" })
    const opener = button()

    returnFocusFromInbox(opener)

    expect(opener).toHaveFocus()
  })

  it("falls back to the bell when the opener has left the page", () => {
    const bell = button({ "data-inbox-bell": "" })
    const opener = button()
    opener.remove()

    returnFocusFromInbox(opener)

    expect(bell).toHaveFocus()
  })

  it("skips a bell that is not shown", () => {
    const hidden = button({ "data-inbox-bell": "", style: "display: none" })
    const shown = button({ "data-inbox-bell": "" })

    returnFocusFromInbox(null)

    expect(hidden).not.toHaveFocus()
    expect(shown).toHaveFocus()
  })
})
