import { describe, expect, it } from "vitest"

import { sidebarShortcut } from "./sidebar-shortcut"

const wide = { phone: false, forced: false, collapsed: false, sheetOpen: false }

describe("sidebarShortcut", () => {
  it("collapses an expanded sidebar and expands a collapsed one on a wide screen", () => {
    expect(sidebarShortcut(wide)).toBe("collapse")
    expect(sidebarShortcut({ ...wide, collapsed: true })).toBe("expand")
  })

  it("opens the sidebar sheet when the rail is forced, and leaves an open sheet to its own Escape", () => {
    const forced = { ...wide, forced: true, collapsed: true }
    expect(sidebarShortcut(forced)).toBe("open-sheet")
    expect(sidebarShortcut({ ...forced, sheetOpen: true })).toBe("none")
  })

  it("ignores the stored collapsed choice while the rail is forced", () => {
    expect(sidebarShortcut({ ...wide, forced: true })).toBe("open-sheet")
  })

  it("does nothing on a phone, where the menu button opens the sheet", () => {
    expect(sidebarShortcut({ ...wide, phone: true, forced: true })).toBe("none")
    expect(sidebarShortcut({ ...wide, phone: true })).toBe("none")
  })
})
