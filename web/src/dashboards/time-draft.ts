import {
  isAbsoluteOrdered,
  isRangeTooLong,
  maxRangeDays,
  parseUserTime,
  resolveTime,
  userTimeProblem,
  zonedDateTime,
} from "@/lib/time"

export type DraftField = "from" | "to"

export type DraftProblem = { field: DraftField; message: string }

export type DraftRead =
  | { range: { from: string; to: string }; problem?: undefined }
  | { range?: undefined; problem: DraftProblem }

const fallback = "Enter a time like now-6h, now/d or 2026-10-17 09:00."

export function readDraft(
  draftFrom: string,
  draftTo: string,
  zone: string | undefined
): DraftRead {
  const from = parseUserTime(draftFrom, zone)
  if (from === null) {
    return {
      problem: {
        field: "from",
        message: userTimeProblem(draftFrom, zone) ?? fallback,
      },
    }
  }
  const to = parseUserTime(draftTo, zone)
  if (to === null) {
    return {
      problem: {
        field: "to",
        message: userTimeProblem(draftTo, zone) ?? fallback,
      },
    }
  }
  if (!isAbsoluteOrdered(from, to)) {
    return {
      problem: { field: "from", message: "From must be earlier than To." },
    }
  }
  if (isRangeTooLong(from, to)) {
    return {
      problem: {
        field: "from",
        message: `The range is longer than ${maxRangeDays} days. Choose a From closer to To.`,
      },
    }
  }
  return { range: { from, to } }
}

export type CalendarMonth = { year: number; month: number }

export function calendarMonthOf(
  draft: string,
  zone: string | undefined,
  now = Date.now()
): CalendarMonth {
  const value = parseUserTime(draft, zone)
  const ms =
    value === null ? now : resolveTime(value, now, false, { zone: zone })
  const at = zonedDateTime(Number.isNaN(ms) ? now : ms, zone)
  return { year: Number(at.slice(0, 4)), month: Number(at.slice(5, 7)) - 1 }
}
