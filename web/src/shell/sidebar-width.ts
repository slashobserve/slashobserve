export const sidebarMinWidth = 200
export const sidebarMaxWidth = 360
export const sidebarDefaultWidth = 240
export const sidebarCollapsedWidth = 44
const sidebarKeyStep = 8
const sidebarShiftKeyStep = 40

export function clampSidebarWidth(width: number): number {
  if (!Number.isFinite(width)) return sidebarDefaultWidth
  return Math.min(sidebarMaxWidth, Math.max(sidebarMinWidth, Math.round(width)))
}

export function parseStoredWidth(raw: string | null): number {
  if (raw === null || !/^\d+$/.test(raw)) return sidebarDefaultWidth
  return clampSidebarWidth(Number(raw))
}

export function keyboardWidth(
  width: number,
  key: string,
  shiftKey: boolean
): number | null {
  const step = shiftKey ? sidebarShiftKeyStep : sidebarKeyStep
  switch (key) {
    case "ArrowLeft":
      return clampSidebarWidth(width - step)
    case "ArrowRight":
      return clampSidebarWidth(width + step)
    case "Home":
      return sidebarMinWidth
    case "End":
      return sidebarMaxWidth
    case "Enter":
      return sidebarDefaultWidth
    default:
      return null
  }
}
