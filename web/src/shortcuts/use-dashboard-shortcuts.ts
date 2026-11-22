import { useEffect, useRef } from "react"

import { isEditableTarget } from "@/lib/shortcuts"

import { feedKey, idleChord, type ShortcutAction } from "./chords"
import { useHoveredPanel, type PanelTarget } from "./hovered-panel"

export type ShortcutHandlers = Partial<Record<ShortcutAction, () => void>>

const openPopup =
  '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]'

function popupOpen(): boolean {
  return document.querySelector(openPopup) !== null
}

export function shortcutTarget(): PanelTarget | null {
  const hovered = useHoveredPanel.getState().target
  if (hovered !== null) return hovered
  const focused =
    document.activeElement?.closest<HTMLElement>("[data-panel-id]")
  const id = focused?.dataset.panelId
  if (focused == null || id === undefined) return null
  return { id, copy: focused.dataset.repeatOf !== undefined }
}

export function shortcutPanelId(): string | null {
  return shortcutTarget()?.id ?? null
}

export function useDashboardShortcuts(handlers: ShortcutHandlers) {
  const latest = useRef(handlers)
  useEffect(() => {
    latest.current = handlers
  })
  useEffect(() => {
    let chord = idleChord
    function onKeyDown(event: KeyboardEvent) {
      if (
        event.defaultPrevented ||
        event.repeat ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        isEditableTarget(event.target) ||
        popupOpen()
      ) {
        chord = idleChord
        return
      }
      const result = feedKey(chord, event.key, event.timeStamp)
      chord = result.state
      const handler = result.action ? latest.current[result.action] : undefined
      if (!handler) return
      event.preventDefault()
      handler()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])
}
