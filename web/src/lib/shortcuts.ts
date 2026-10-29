export type ChordEvent = {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
}

export function isEditableTarget(target: EventTarget | null): boolean {
  if (target === null || !("tagName" in target)) return false
  const element = target as { tagName: string; isContentEditable?: boolean }
  return (
    element.isContentEditable === true ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName)
  )
}

export function isModChord(event: ChordEvent, key: string): boolean {
  return (
    (event.metaKey || event.ctrlKey) &&
    !event.altKey &&
    !event.shiftKey &&
    event.key.toLowerCase() === key.toLowerCase()
  )
}

export function isModEnter(
  event: Pick<ChordEvent, "key" | "metaKey" | "ctrlKey">
): boolean {
  return event.key === "Enter" && (event.metaKey || event.ctrlKey)
}

export function modChordLabel(userAgent: string, key: string): string {
  const letter = key.toUpperCase()
  return /Mac|iPhone|iPad|iPod/.test(userAgent)
    ? `⌘${letter}`
    : `Ctrl ${letter}`
}
