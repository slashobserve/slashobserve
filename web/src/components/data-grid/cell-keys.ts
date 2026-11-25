import type {
  FocusGridInnerElementParams,
  SuppressKeyboardEventParams,
} from "ag-grid-community"

import { tabStaysInHeader } from "./header-keys"

const controlSelector =
  "a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex='-1'])"
const keyStopSelector = ".ag-cell, .ag-full-width-row"
const controlKeys = new Set(["Enter", " "])
const arrowKeys = new Set(["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"])

const fieldKeys = new Set(["Home", "End", "PageUp", "PageDown"])

function isField(target: HTMLElement): boolean {
  return (
    target.isContentEditable ||
    target.matches("input, textarea, select, [contenteditable]")
  )
}

function leavesArrowsToGrid(target: HTMLElement): boolean {
  return target.matches("a[href]")
}

const tabbableSelector =
  "a[href], button:not(:disabled), input:not(:disabled):not([type=hidden]), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex='-1'])"

export type CellKeyAction =
  { kind: "click" | "focus"; stop: number } | { kind: "leave" } | null

export function cellKeyAction(
  key: string,
  shift: boolean,
  stop: number,
  controlCount: number
): CellKeyAction {
  if (key === "Tab") {
    const next = stop + (shift ? -1 : 1)
    return stop >= 0 && controlCount >= 2 && next >= 0 && next <= controlCount
      ? { kind: "focus", stop: next }
      : { kind: "leave" }
  }
  if (stop < 0 || key !== "Enter") return null
  if (stop > 0) return { kind: "click", stop }
  return controlCount === 1 ? { kind: "click", stop: 1 } : null
}

export function leaveGridOnTab(): boolean {
  return false
}

export function focusEntryCell(firstColumnId: string | undefined) {
  return ({ api }: FocusGridInnerElementParams): boolean => {
    const rowCount = api.getDisplayedRowCount()
    if (rowCount === 0) return false
    const focused = api.getFocusedCell()
    if (focused !== null && focused.rowIndex < rowCount) {
      api.setFocusedCell(focused.rowIndex, focused.column, focused.rowPinned)
      return true
    }
    if (firstColumnId === undefined) return false
    api.setFocusedCell(0, firstColumnId)
    return true
  }
}

function focusBeyond(grid: HTMLElement, backwards: boolean): boolean {
  const wanted = backwards
    ? Node.DOCUMENT_POSITION_PRECEDING
    : Node.DOCUMENT_POSITION_FOLLOWING
  const candidates = Array.from(
    grid.ownerDocument.querySelectorAll<HTMLElement>(tabbableSelector)
  ).filter(
    (element) =>
      !grid.contains(element) &&
      !element.contains(grid) &&
      (grid.compareDocumentPosition(element) & wanted) !== 0 &&
      element.closest("[inert]") === null &&
      element.checkVisibility?.() !== false
  )
  const target = backwards ? candidates.at(-1) : candidates[0]
  target?.focus()
  return target !== undefined
}

export function keepKeysOnFocusedControl({
  event,
}: Pick<SuppressKeyboardEventParams, "event">): boolean {
  const target = event.target
  if (!(target instanceof HTMLElement)) return false
  if (arrowKeys.has(event.key)) {
    if (leavesArrowsToGrid(target)) return false
  } else if (!controlKeys.has(event.key)) {
    if (!fieldKeys.has(event.key) || !isField(target)) return false
  }
  const cell = target.closest(keyStopSelector)
  return cell !== null && cell !== target
}

export function handleCellKey(event: KeyboardEvent, grid: HTMLElement): void {
  const target = event.target
  if (!(target instanceof HTMLElement)) return
  if (event.key === "Enter" && isField(target)) return
  if (
    event.key === "Tab" &&
    target.closest("[role=columnheader]") !== null &&
    grid.contains(target)
  ) {
    if (!tabStaysInHeader(event) && focusBeyond(grid, event.shiftKey)) {
      event.preventDefault()
    }
    return
  }
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
  if (action.kind === "leave") {
    if (focusBeyond(grid, event.shiftKey)) event.preventDefault()
    return
  }
  event.preventDefault()
  if (action.kind === "click") stops[action.stop]?.click()
  else stops[action.stop]?.focus()
}
