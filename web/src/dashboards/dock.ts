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
