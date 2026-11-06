import { useEffect, useRef } from "react"

import { isEditableTarget } from "@/lib/shortcuts"

import { feedKey, idleChord, type ShortcutAction } from "./chords"
import { useHoveredPanel } from "./hovered-panel"

export type ShortcutHandlers = Partial<Record<ShortcutAction, () => void>>

const openPopup =
  '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]'

function popupOpen(): boolean {
  return document.querySelector(openPopup) !== null
}

export function shortcutPanelId(): string | null {
  const hovered = useHoveredPanel.getState().id
  if (hovered !== null) return hovered
  const focused =
    document.activeElement?.closest<HTMLElement>("[data-panel-id]")
  return focused?.dataset.panelId ?? null
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
