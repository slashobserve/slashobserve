import type { DashboardSearch } from "./search"

export type SidePanelCard = "chat" | "activity" | "edit" | "share"

export type DockView = SidePanelCard | "inspect" | "spec" | null

export type DockState = {
  open: boolean
  layoutEditing: boolean
  inspectable: boolean
  specOpen: boolean
}

export function dockOf(search: DashboardSearch, state: DockState): DockView {
  const panelInUrl = search.edit !== undefined && search.panel !== undefined
  if (state.inspectable && !state.layoutEditing) return "inspect"
  if (search.kiosk === true) return null
  if (state.specOpen && !panelInUrl) return "spec"
  if (state.layoutEditing) return null
  if (panelInUrl) return "edit"
  if (search.share === true) return "share"
  if (
    search.tab === "chat" ||
    (search.tab === undefined && search.chat !== undefined)
  ) {
    return "chat"
  }
  if (state.open || search.tab === "activity" || search.thread !== undefined) {
    return "activity"
  }
  return null
}

export function isSidePanelCard(view: DockView): view is SidePanelCard {
  return (
    view === "chat" ||
    view === "activity" ||
    view === "edit" ||
    view === "share"
  )
}

export type SidePane = "activity" | "chat"

function sidePaneOf(view: DockView): SidePane | null {
  return view === "activity" || view === "chat" ? view : null
}

export function heldPaneOf(
  search: DashboardSearch,
  state: DockState
): SidePane | null {
  return sidePaneOf(dockOf(search, { ...state, layoutEditing: false }))
}

export function arrangingSetsPaneAside(
  shown: DockView,
  besideGrid: boolean
): boolean {
  return besideGrid && sidePaneOf(shown) !== null
}

export type ArrangingPane = "aside" | "kept" | null

export function arrangingPaneAfter(
  current: ArrangingPane,
  {
    arranging,
    setsAside,
    reopened,
  }: { arranging: boolean; setsAside: boolean; reopened: boolean }
): ArrangingPane {
  if (!arranging) return null
  if (reopened) return "kept"
  if (current === null) return setsAside ? "aside" : "kept"
  return current
}
