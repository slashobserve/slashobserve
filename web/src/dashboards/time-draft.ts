import {
  isAbsoluteOrdered,
  isRangeTooLong,
  maxRangeDays,
  parseUserTime,
  resolveTime,
  timeFormatHint,
  userTimeProblem,
  zonedDateTime,
} from "@/lib/time"

export type DraftField = "from" | "to"

export type DraftProblem = { field: DraftField; message: string }

export type DraftRead =
  | { range: { from: string; to: string }; problem?: undefined }
  | { range?: undefined; problem: DraftProblem }

function unreadableTimeText(zone: string | undefined): string {
  return `Enter a time like ${timeFormatHint(zone)}.`
}

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
        message: userTimeProblem(draftFrom, zone) ?? unreadableTimeText(zone),
      },
    }
  }
  const to = parseUserTime(draftTo, zone)
  if (to === null) {
    return {
      problem: {
        field: "to",
        message: userTimeProblem(draftTo, zone) ?? unreadableTimeText(zone),
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
