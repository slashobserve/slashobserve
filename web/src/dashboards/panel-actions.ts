import { useActivityStore } from "@/activity/activity-store"

import type { useSetDashboardSearch } from "./search"

type SetSearch = ReturnType<typeof useSetDashboardSearch>

export function commentOnPanel(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: undefined }))
}

export function askAboutPanel(setSearch: SetSearch, panelId: string) {
  const activity = useActivityStore.getState()
  activity.setOpen(true)
  activity.setAgentKind("investigate")
  setSearch((state) => ({ ...state, panel: panelId, edit: undefined }))
}

export function editPanelQuery(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: "query" }))
}

export function askAboutDashboard(setSearch: SetSearch) {
  const activity = useActivityStore.getState()
  activity.setOpen(true)
  activity.setAgentKind("investigate")
  setSearch((state) => ({ ...state, panel: undefined, edit: undefined }))
}
