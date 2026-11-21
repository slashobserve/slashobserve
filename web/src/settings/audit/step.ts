import { isEditableTarget } from "@/lib/shortcuts"

export type Neighbors = { previous: string | null; next: string | null }

export function neighborsOf(
  ids: readonly string[],
  selected: string
): Neighbors {
  const index = ids.indexOf(selected)
  if (index < 0) return { previous: null, next: null }
  return {
    previous: index > 0 ? ids[index - 1] : null,
    next: index < ids.length - 1 ? ids[index + 1] : null,
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

export function stepFromKey(
  event: StepEvent,
  neighbors: Neighbors
): string | null {
  if (event.defaultPrevented) return null
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
