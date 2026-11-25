const focusable =
  "a[href], button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex='-1'])"

export function tabStaysInCell(event: KeyboardEvent): boolean {
  if (event.key !== "Tab" || !(event.target instanceof HTMLElement)) {
    return false
  }
  const cell = event.target.closest("[role=gridcell], [role=columnheader]")
  if (cell === null) return false
  const inside = Array.from(cell.querySelectorAll<HTMLElement>(focusable))
  if (inside.length === 0) return false
  const at = inside.indexOf(event.target)
  return event.shiftKey ? at > 0 : at < inside.length - 1
}

export function clickCellControl(event: KeyboardEvent): boolean {
  if (event.type !== "keydown" || event.key !== "Enter") return false
  const cell = event.target
  if (
    !(cell instanceof HTMLElement) ||
    cell.getAttribute("role") !== "gridcell"
  )
    return false
  const control = cell.querySelector<HTMLElement>("a[href], button")
  if (control === null) return false
  event.preventDefault()
  control.click()
  return true
}
