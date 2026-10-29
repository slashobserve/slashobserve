import { useEffect, useRef } from "react"

import type { CursorSync } from "@/panels/shared-cursor"
import { isEditableTarget, isModChord, type ChordEvent } from "@/lib/shortcuts"
import { dashboardDefaults } from "@/spec/defaults"
import type { Dashboard } from "@/spec"

export const cursorSyncOptions: readonly {
  value: CursorSync
  label: string
}[] = [
  { value: "off", label: "This chart only" },
  { value: "crosshair", label: "Shared crosshair" },
  { value: "tooltip", label: "Shared tooltip" },
]

export function isCursorSync(value: unknown): value is CursorSync {
  return cursorSyncOptions.some((option) => option.value === value)
}

export function cursorSyncLabel(mode: CursorSync): string {
  return cursorSyncOptions.find((option) => option.value === mode)!.label
}

export function savedCursorSync(
  spec: Pick<Dashboard, "cursorSync">
): CursorSync {
  return spec.cursorSync ?? dashboardDefaults.cursorSync
}

export function nextCursorSync(mode: CursorSync): CursorSync {
  const index = cursorSyncOptions.findIndex((option) => option.value === mode)
  return cursorSyncOptions[(index + 1) % cursorSyncOptions.length].value
}

export function cursorOverride(
  chosen: CursorSync,
  saved: CursorSync
): CursorSync | undefined {
  return chosen === saved ? undefined : chosen
}

export function isCursorSyncShortcut(
  event: ChordEvent & {
    target: EventTarget | null
    defaultPrevented: boolean
    repeat: boolean
  }
): boolean {
  return (
    !event.defaultPrevented &&
    !event.repeat &&
    isModChord(event, "o") &&
    !isEditableTarget(event.target)
  )
}

export function useCursorSyncShortcut(onCycle: () => void) {
  const latest = useRef(onCycle)
  useEffect(() => {
    latest.current = onCycle
  })
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (!isCursorSyncShortcut(event)) return
      event.preventDefault()
      latest.current()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])
}
