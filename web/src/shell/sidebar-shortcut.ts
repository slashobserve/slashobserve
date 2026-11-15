export const sidebarChordKey = "\\"

export const sidebarKeyShortcuts = "Meta+Backslash Control+Backslash"

export type SidebarShortcut = "collapse" | "expand" | "open-sheet" | "none"

export function sidebarShortcut(state: {
  phone: boolean
  forced: boolean
  collapsed: boolean
  sheetOpen: boolean
}): SidebarShortcut {
  if (state.phone) return "none"
  if (state.forced) return state.sheetOpen ? "none" : "open-sheet"
  return state.collapsed ? "expand" : "collapse"
}
