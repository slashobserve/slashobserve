import type {
  FocusGridInnerElementParams,
  GridApi,
  RowPinnedType,
  SuppressKeyboardEventParams,
} from "ag-grid-community"

import { tabStaysInHeader } from "./header-keys"

const notRemoved = ":not([tabindex='-1'])"
const controlSelector = [
  "a[href]",
  "button:not(:disabled)",
  "input:not(:disabled):not([type=hidden])",
  "select:not(:disabled)",
  "textarea:not(:disabled)",
  "[contenteditable]:not([contenteditable='false'])",
  "[tabindex]",
]
  .map((alternative) => alternative + notRemoved)
  .join(", ")
const gridSelector = "[role=grid], [role=treegrid]"
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

function rowCountAt(api: GridApi, pinned: RowPinnedType): number {
  if (pinned === "bottom") return api.getPinnedBottomRowCount()
  if (pinned === "top") return api.getPinnedTopRowCount()
  return api.getDisplayedRowCount()
}

export function focusEntryCell({
  api,
}: Pick<FocusGridInnerElementParams, "api">): boolean {
  const displayed = api.getAllDisplayedColumns()
  const first = displayed[0]
  if (first === undefined) return false
  const focused = api.getFocusedCell()
  if (
    focused !== null &&
    focused.rowIndex < rowCountAt(api, focused.rowPinned)
  ) {
    const column =
      displayed.find((c) => c.getColId() === focused.column.getColId()) ?? first
    api.setFocusedCell(focused.rowIndex, column, focused.rowPinned)
    return true
  }
  if (api.getDisplayedRowCount() === 0) return false
  api.setFocusedCell(0, first)
  return true
}

function focusBeyond(grid: HTMLElement, backwards: boolean): boolean {
  const wanted = backwards
    ? Node.DOCUMENT_POSITION_PRECEDING
    : Node.DOCUMENT_POSITION_FOLLOWING
  const candidates = Array.from(
    grid.ownerDocument.querySelectorAll<HTMLElement>(controlSelector)
  ).filter(
    (element) =>
      !grid.contains(element) &&
      !element.contains(grid) &&
      (grid.compareDocumentPosition(element) & wanted) !== 0 &&
      element.closest("[inert], [aria-hidden='true']") === null &&
      element.checkVisibility?.({ visibilityProperty: true }) !== false
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

function stopOf(
  stops: readonly HTMLElement[],
  target: HTMLElement,
  withinStop: boolean
): number {
  const exact = stops.indexOf(target)
  if (exact >= 0 || !withinStop) return exact
  return stops.findLastIndex((stop) => stop.contains(target))
}

function ownsCell(grid: HTMLElement, cell: HTMLElement): boolean {
  const inner = cell.closest(gridSelector)
  return inner === null || inner === grid.querySelector(gridSelector)
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
  if (cell === null || !grid.contains(cell) || !ownsCell(grid, cell)) return
  const stops = [cell, ...cell.querySelectorAll<HTMLElement>(controlSelector)]
  const action = cellKeyAction(
    event.key,
    event.shiftKey,
    stopOf(stops, target, event.key === "Tab"),
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
