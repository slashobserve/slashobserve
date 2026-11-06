export type DayBreak = { kind: "day"; key: string; label: string }

type Dated = { key: string; at: string }

function pad(value: number): string {
  return String(value).padStart(2, "0")
}

function dayKey(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function parsed(iso: string): Date | null {
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? null : date
}

export function dayLabel(iso: string, now: number): string {
  const date = parsed(iso)
  if (date === null) return ""
  const today = new Date(now)
  const yesterday = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate() - 1
  )
  const key = dayKey(date)
  if (key === dayKey(today)) return "Today"
  if (key === dayKey(yesterday)) return "Yesterday"
  if (date.getFullYear() === today.getFullYear()) {
    return date.toLocaleDateString("en-US", {
      weekday: "short",
      month: "short",
      day: "numeric",
    })
  }
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

export function withDayBreaks<T extends Dated>(
  entries: readonly T[],
  now: number
): (DayBreak | { kind: "entry"; entry: T })[] {
  const rows: (DayBreak | { kind: "entry"; entry: T })[] = []
  let previous: string | null = null
  for (const entry of entries) {
    const date = parsed(entry.at)
    const key = date === null ? null : dayKey(date)
    if (key !== null && key !== previous) {
      rows.push({
        kind: "day",
        key: `day:${key}`,
        label: dayLabel(entry.at, now),
      })
      previous = key
    }
    rows.push({ kind: "entry", entry })
  }
  return rows
}
