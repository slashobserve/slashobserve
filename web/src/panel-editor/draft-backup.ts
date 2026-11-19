import { useEffect, useState } from "react"

import { sessionEnded } from "@/lib/auth"

import { panelChanges, rebasePanel } from "./changes"
import type { EditablePanel, EditorDraft } from "./draft"

type Backup = { base: EditablePanel; panel: EditablePanel }

function keyOf(dashboardId: string, panelId: string): string {
  return `slashobserve-panel-draft:${dashboardId}:${panelId}`
}

function isPanelWithId(value: unknown, id: string): value is EditablePanel {
  return (
    typeof value === "object" &&
    value !== null &&
    (value as { id?: unknown }).id === id
  )
}

export function readBackup(
  storage: Pick<Storage, "getItem">,
  dashboardId: string,
  panelId: string
): Backup | null {
  try {
    const parsed: unknown = JSON.parse(
      storage.getItem(keyOf(dashboardId, panelId)) ?? ""
    )
    if (typeof parsed !== "object" || parsed === null) return null
    const { base, panel } = parsed as { base?: unknown; panel?: unknown }
    if (!isPanelWithId(base, panelId) || !isPanelWithId(panel, panelId)) {
      return null
    }
    return panelChanges(base, panel).length > 0 ? { base, panel } : null
  } catch {
    return null
  }
}

function sessionStore(): Storage | null {
  try {
    return sessionStorage
  } catch {
    return null
  }
}

export function restoreBackup(
  backup: Backup,
  current: EditablePanel
): EditablePanel {
  return rebasePanel(backup.base, backup.panel, current)
}

export function useDraftBackup(
  dashboardId: string,
  draft: EditorDraft,
  dirty: boolean
) {
  const panelId = draft.base.id
  const [kept, setKept] = useState<Backup | null>(() => {
    const store = sessionStore()
    return store ? readBackup(store, dashboardId, panelId) : null
  })
  useEffect(() => {
    if (!dirty) return
    try {
      sessionStore()?.setItem(
        keyOf(dashboardId, panelId),
        JSON.stringify({ base: draft.base, panel: draft.panel })
      )
    } catch {
      return
    }
  }, [dashboardId, panelId, dirty, draft.base, draft.panel])
  useEffect(
    () => () => {
      if (sessionEnded()) return
      try {
        sessionStore()?.removeItem(keyOf(dashboardId, panelId))
      } catch {
        return
      }
    },
    [dashboardId, panelId]
  )
  return { kept: dirty ? null : kept, drop: () => setKept(null) }
}
