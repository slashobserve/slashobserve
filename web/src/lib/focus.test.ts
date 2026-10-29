import { describe, expect, it } from "vitest"

import { focusMovedOutOfMenu } from "./focus"

type Fake = { closest: (selector: string) => unknown }

const body: Fake = { closest: () => null }
const insideMenu: Fake = {
  closest: (selector) => (selector === '[role="menu"]' ? {} : null),
}
const elsewhere: Fake = { closest: () => null }

describe("focusMovedOutOfMenu", () => {
  it.each([
    ["nothing is focused", null, false],
    ["the page body is focused", body, false],
    ["an item inside the menu is focused", insideMenu, false],
    ["another element is focused", elsewhere, true],
  ])("when %s returns %s", (_name, active, expected) => {
    expect(focusMovedOutOfMenu(active, body)).toBe(expected)
  })
})
