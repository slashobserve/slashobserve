export const fieldsMinWidth = 200
export const fieldsMaxWidth = 560
export const fieldsDefaultWidth = 248
const fieldsKeyStep = 8
const fieldsShiftKeyStep = 40

export const fieldsWidthKey = "slashobserve-logs-fields-width"

export function clampFieldsWidth(width: number): number {
  if (!Number.isFinite(width)) return fieldsDefaultWidth
  return Math.min(fieldsMaxWidth, Math.max(fieldsMinWidth, Math.round(width)))
}

export function parseStoredFieldsWidth(raw: string | null): number {
  if (raw === null || !/^\d+$/.test(raw)) return fieldsDefaultWidth
  return clampFieldsWidth(Number(raw))
}

export function storedFieldsWidth(): number {
  try {
    return parseStoredFieldsWidth(localStorage.getItem(fieldsWidthKey))
  } catch {
    return fieldsDefaultWidth
  }
}

export function storeFieldsWidth(width: number): void {
  try {
    localStorage.setItem(fieldsWidthKey, String(clampFieldsWidth(width)))
  } catch {
    return
  }
}

export function keyboardFieldsWidth(
  width: number,
  key: string,
  shiftKey: boolean
): number | null {
  const step = shiftKey ? fieldsShiftKeyStep : fieldsKeyStep
  switch (key) {
    case "ArrowLeft":
      return clampFieldsWidth(width - step)
    case "ArrowRight":
      return clampFieldsWidth(width + step)
    case "Home":
      return fieldsMinWidth
    case "End":
      return fieldsMaxWidth
    case "Enter":
      return fieldsDefaultWidth
    default:
      return null
  }
}
