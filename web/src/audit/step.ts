import { isEditableTarget } from "@/lib/shortcuts"

export type Neighbors = { previous: string | null; next: string | null }

export function neighborsOf(
  ids: readonly string[],
  selected: string
): Neighbors {
  const index = ids.indexOf(selected)
  if (index < 0) return { previous: null, next: null }
  return {
    previous: ids[index - 1] ?? null,
    next: ids[index + 1] ?? null,
  }
}

type StepEvent = {
  key: string
  defaultPrevented: boolean
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
  target: EventTarget | null
}

const ownsArrowKeys =
  '[role="menu"], [role="listbox"], [role="combobox"], [role="slider"], [role="tablist"], [role="dialog"], [role="alertdialog"]'

function inWidgetThatOwnsArrows(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(ownsArrowKeys) !== null
}

const tableScope = '[role="region"][aria-label="Audit events"]'

function inStepScope(target: EventTarget | null, panel: Element | null) {
  if (!(target instanceof Element)) return false
  return target.closest(tableScope) !== null || panel?.contains(target) === true
}

export function stepFromKey(
  event: StepEvent,
  neighbors: Neighbors,
  panel: Element | null
): string | null {
  if (event.defaultPrevented) return null
  if (!inStepScope(event.target, panel)) return null
  if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) {
    return null
  }
  if (isEditableTarget(event.target) || inWidgetThatOwnsArrows(event.target)) {
    return null
  }
  if (event.key === "ArrowUp") return neighbors.previous
  if (event.key === "ArrowDown") return neighbors.next
  return null
}
