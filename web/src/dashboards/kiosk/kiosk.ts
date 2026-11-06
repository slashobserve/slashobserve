type Listed = { id: string; title: string }

export function nextRowIndex(
  rowCount: number,
  current: number | null
): number | null {
  if (rowCount <= 0) return null
  if (current === null || current < 0 || current >= rowCount - 1) return 0
  return current + 1
}

export function nextFolderDashboard<T extends Listed>(
  dashboards: readonly T[],
  currentId: string
): T | null {
  const ordered = dashboards.toSorted(
    (a, b) => a.title.localeCompare(b.title) || a.id.localeCompare(b.id)
  )
  const index = ordered.findIndex((dashboard) => dashboard.id === currentId)
  const next = ordered[(index + 1) % ordered.length]
  return next === undefined || next.id === currentId ? null : next
}

export type KioskScreen = { kiosk: boolean; fullscreen: boolean }

export type KioskEvent =
  "fullscreen-changed" | "kiosk-changed" | "fullscreen-key"

export type KioskStep = "leave-kiosk" | "exit-fullscreen" | "enter-fullscreen"

export function kioskStep(
  event: KioskEvent,
  { kiosk, fullscreen }: KioskScreen
): KioskStep | null {
  switch (event) {
    case "fullscreen-changed":
      return kiosk && !fullscreen ? "leave-kiosk" : null
    case "kiosk-changed":
      return !kiosk && fullscreen ? "exit-fullscreen" : null
    case "fullscreen-key":
      return kiosk && !fullscreen ? "enter-fullscreen" : null
  }
}
