export type CalendarDay = { year: number; month: number; day: number }

export type DaySelection = { start: string; end: string | null } | null

const monthNames = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
]

export const weekdayLetters = ["M", "T", "W", "T", "F", "S", "S"]

function pad(value: number): string {
  return String(value).padStart(2, "0")
}

export function dayText({ year, month, day }: CalendarDay): string {
  return `${String(year).padStart(4, "0")}-${pad(month + 1)}-${pad(day)}`
}

export function monthTitle(year: number, month: number): string {
  return `${monthNames[month]} ${year}`
}

export function dayLabel(day: CalendarDay): string {
  return `${day.day} ${monthNames[day.month]} ${day.year}`
}

export function shiftMonth(
  year: number,
  month: number,
  by: number
): { year: number; month: number } {
  const index = year * 12 + month + by
  return { year: Math.floor(index / 12), month: ((index % 12) + 12) % 12 }
}

export function monthWeeks(year: number, month: number): CalendarDay[][] {
  const first = new Date(Date.UTC(year, month, 1))
  const lead = (first.getUTCDay() + 6) % 7
  const length = new Date(Date.UTC(year, month + 1, 0)).getUTCDate()
  const rows = Math.ceil((lead + length) / 7)
  return Array.from({ length: rows }, (_, row) =>
    Array.from({ length: 7 }, (_, column) => {
      const at = new Date(Date.UTC(year, month, 1 - lead + row * 7 + column))
      return {
        year: at.getUTCFullYear(),
        month: at.getUTCMonth(),
        day: at.getUTCDate(),
      }
    })
  )
}

export function pickDay(selection: DaySelection, day: string): DaySelection {
  if (selection === null || selection.end !== null) {
    return { start: day, end: null }
  }
  return day < selection.start
    ? { start: day, end: selection.start }
    : { start: selection.start, end: day }
}

export function selectedSpan(selection: DaySelection): {
  first: string
  last: string
} | null {
  if (selection === null) return null
  return { first: selection.start, last: selection.end ?? selection.start }
}
