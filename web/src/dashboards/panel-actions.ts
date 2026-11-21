import type { RefObject } from "react"

import { useActivityStore } from "@/activity/activity-store"

import type { useSetDashboardSearch } from "./search"

type SetSearch = ReturnType<typeof useSetDashboardSearch>

const focusFrames = 10

export function focusComment(field: RefObject<HTMLTextAreaElement | null>) {
  useActivityStore.getState().setOpen(true)
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
  setSearch((state) => ({ ...state, panel: panelId, edit: undefined }))
}

export function askAboutPanel(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: undefined }))
}

export function editPanelQuery(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: "query" }))
}

export function askAboutDashboard(setSearch: SetSearch) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: undefined, edit: undefined }))
}
