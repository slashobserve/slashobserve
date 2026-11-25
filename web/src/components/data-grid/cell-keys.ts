import type { SuppressKeyboardEventParams } from "ag-grid-community"

const controlSelector =
  "a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex='-1'])"
const keyStopSelector = ".ag-cell, .ag-full-width-row"
const controlKeys = new Set([
  "Tab",
  "Enter",
  " ",
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
  "PageUp",
  "PageDown",
])

export type CellKeyAction = { kind: "click" | "focus"; stop: number } | null

export function cellKeyAction(
  key: string,
  shift: boolean,
  stop: number,
  controlCount: number
): CellKeyAction {
  if (stop < 0) return null
  if (key === "Enter") {
    if (stop > 0) return { kind: "click", stop }
    return controlCount === 1 ? { kind: "click", stop: 1 } : null
  }
  if (key !== "Tab" || controlCount < 2) return null
  const next = stop + (shift ? -1 : 1)
  return next >= 0 && next <= controlCount
    ? { kind: "focus", stop: next }
    : null
}

export function keepKeysOnFocusedControl({
  event,
}: Pick<SuppressKeyboardEventParams, "event">): boolean {
  const target = event.target
  if (!(target instanceof HTMLElement) || !controlKeys.has(event.key)) {
    return false
  }
  const cell = target.closest(keyStopSelector)
  return cell !== null && cell !== target
}

export function handleCellKey(event: KeyboardEvent, grid: HTMLElement): void {
  const target = event.target
  if (!(target instanceof HTMLElement)) return
  if (event.key === "Enter" && target.matches("input, textarea, select")) return
  const cell = target.closest<HTMLElement>(keyStopSelector)
  if (cell === null || !grid.contains(cell)) return
  const stops = [cell, ...cell.querySelectorAll<HTMLElement>(controlSelector)]
  const action = cellKeyAction(
    event.key,
    event.shiftKey,
    stops.indexOf(target),
    stops.length - 1
  )
  if (action === null) return
  event.preventDefault()
  if (action.kind === "click") stops[action.stop]?.click()
  else stops[action.stop]?.focus()
}
