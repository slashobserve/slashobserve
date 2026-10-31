import { describeDuration, describeRange } from "@/lib/time"

export type TimeOverride = { text: string; title: string }

export function timeOverrideLabel(panel: {
  timeFrom?: string
  timeShift?: string
  hideTimeOverride?: boolean
}): TimeOverride | null {
  const { timeFrom, timeShift } = panel
  if (panel.hideTimeOverride) return null
  const parts: string[] = []
  if (timeFrom) parts.push(describeRange(timeFrom, "now"))
  if (timeShift) {
    const shifted = `shifted ${describeDuration(timeShift) ?? timeShift}`
    parts.push(timeFrom ? shifted : capitalize(shifted))
  }
  if (parts.length === 0) return null
  const exact = [
    timeFrom ? `timeFrom ${timeFrom}` : "",
    timeShift ? `timeShift ${timeShift}` : "",
  ].filter(Boolean)
  return { text: parts.join(", "), title: exact.join(", ") }
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}
