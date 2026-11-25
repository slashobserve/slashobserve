const focusable =
  "a[href], button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex='-1'])"

export function tabStaysInHeader(event: KeyboardEvent): boolean {
  if (event.key !== "Tab" || !(event.target instanceof HTMLElement)) {
    return false
  }
  const header = event.target.closest("[role=columnheader]")
  if (header === null) return false
  const inside = Array.from(header.querySelectorAll<HTMLElement>(focusable))
  if (inside.length === 0) return false
  const at = inside.indexOf(event.target)
  return event.shiftKey ? at > 0 : at < inside.length - 1
}
