import type { RefObject } from "react"

import { useActivityStore } from "@/activity/activity-store"
import { useAgentInfo } from "@/observe/queries"
import { isDataPanel, type LayoutPanel } from "@/spec"

import { openKeyDraft } from "./canvas-tools"

import type { DashboardSearch, useSetDashboardSearch } from "./search"

type SetSearch = ReturnType<typeof useSetDashboardSearch>

const focusFrames = 10

function onActivityTab(state: DashboardSearch): DashboardSearch {
  return state.tab === "activity" ? state : { ...state, tab: "activity" }
}

type Composer = RefObject<HTMLTextAreaElement | null>

export type ConversationActions = { comment: boolean; ask: boolean }

export function conversationActionsFor(
  panel: LayoutPanel,
  canComment: boolean,
  agentAvailable: boolean
): ConversationActions {
  return {
    comment: canComment,
    ask: canComment && isDataPanel(panel) && agentAvailable,
  }
}

export function useConversationActions(
  panel: LayoutPanel,
  canComment: boolean
): ConversationActions {
  const agentAvailable = useAgentInfo().data?.available === true
  return conversationActionsFor(panel, canComment, agentAvailable)
}

function focusWhenMounted(field: Composer) {
  function attempt(framesLeft: number) {
    if (field.current !== null) {
      field.current.focus()
      return
    }
    if (framesLeft > 0) requestAnimationFrame(() => attempt(framesLeft - 1))
  }
  requestAnimationFrame(() => attempt(focusFrames))
}

export function focusComment(setSearch: SetSearch, field: Composer) {
  useActivityStore.getState().setOpen(true)
  setSearch(onActivityTab)
  focusWhenMounted(field)
}

function commentOnPanel(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({
    ...onActivityTab(state),
    panel: panelId,
    edit: undefined,
  }))
}

export type CommentPlace = "canvas" | "pane"

export function commentOnThisPanel(
  setSearch: SetSearch,
  panelId: string,
  onCanvas: boolean,
  paneField?: Composer
): CommentPlace {
  if (onCanvas) {
    openKeyDraft(panelId, "comment")
    return "canvas"
  }
  commentOnPanel(setSearch, panelId)
  if (paneField) focusWhenMounted(paneField)
  return "pane"
}

export function askAboutThisPanel(
  setSearch: SetSearch,
  panelId: string,
  onCanvas: boolean
): CommentPlace {
  if (onCanvas) {
    openKeyDraft(panelId, "ask")
    return "canvas"
  }
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({
    ...state,
    tab: "chat",
    panel: panelId,
    edit: undefined,
  }))
  return "pane"
}

export function editPanelQuery(setSearch: SetSearch, panelId: string) {
  useActivityStore.getState().setOpen(true)
  setSearch((state) => ({ ...state, panel: panelId, edit: "query" }))
}
