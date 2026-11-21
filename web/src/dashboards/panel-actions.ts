import type { RefObject } from "react"

import { useActivityStore } from "@/activity/activity-store"

import type { DashboardSearch, useSetDashboardSearch } from "./search"

type SetSearch = ReturnType<typeof useSetDashboardSearch>

const focusFrames = 10

function onActivityTab(state: DashboardSearch): DashboardSearch {
  return state.tab === "activity" ? state : { ...state, tab: "activity" }
}

export function focusComment(
  setSearch: SetSearch,
  field: RefObject<HTMLTextAreaElement | null>
) {
  useActivityStore.getState().setOpen(true)
  setSearch(onActivityTab)
  function attempt(framesLeft: number) {
    if (field.current !== null) {
      field.current.focus()
      return
    }
    if (framesLeft > 0) requestAnimationFrame(() => attempt(framesLeft - 1))
  }
  requestAnimationFrame(() => attempt(focusFrames))
}

export function commentOnPanel(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({
    ...onActivityTab(state),
    panel: panelId,
    edit: undefined,
  }))
}

export function askAboutPanel(setSearch: SetSearch, panelId: string) {
  const activity = useActivityStore.getState()
  activity.setOpen(true)
  activity.setAgentKind("investigate")
  setSearch((state) => ({
    ...onActivityTab(state),
    panel: panelId,
    edit: undefined,
  }))
}

export function editPanelQuery(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: "query" }))
}

export function askAboutDashboard(setSearch: SetSearch) {
  const activity = useActivityStore.getState()
  activity.setOpen(true)
  activity.setAgentKind("investigate")
  setSearch((state) => ({
    ...onActivityTab(state),
    panel: undefined,
    edit: undefined,
  }))
}
