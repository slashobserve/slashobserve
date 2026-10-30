import { timePickerDefaults, type Dashboard } from "@/spec"

export const quickRanges: QuickRange[] = [
  { label: "Last 5 minutes", from: "now-5m", to: "now" },
  { label: "Last 15 minutes", from: "now-15m", to: "now" },
  { label: "Last 30 minutes", from: "now-30m", to: "now" },
  { label: "Last 1 hour", from: "now-1h", to: "now" },
  { label: "Last 3 hours", from: "now-3h", to: "now" },
  { label: "Last 6 hours", from: "now-6h", to: "now" },
  { label: "Last 12 hours", from: "now-12h", to: "now" },
  { label: "Last 24 hours", from: "now-24h", to: "now" },
  { label: "Last 2 days", from: "now-2d", to: "now" },
  { label: "Last 7 days", from: "now-7d", to: "now" },
  { label: "Last 30 days", from: "now-30d", to: "now" },
  { label: "Today so far", from: "now/d", to: "now" },
  { label: "Yesterday", from: "now-1d/d", to: "now-1d/d" },
]

const timeExpression =
  /^(now([+-][1-9][0-9]*[smhdwMy])?(\/(fy|fQ|[smhdwMy]))?|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2}))$/
const epochMillis = /^[0-9]{10,15}$/
const relativeExpression =
  /^now(?:([+-])([0-9]+)([smhdwMy]))?(?:\/(fy|fQ|[smhdwMy]))?$/
const localDateTime =
  /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?$/

export function isTimeValue(value: string): boolean {
  return timeExpression.test(value) || epochMillis.test(value)
}

export function parseUserTime(input: string): string | null {
  const text = input.trim()
  if (isTimeValue(text)) return text
  const match = localDateTime.exec(text)
  if (!match) return null
  const [year, month, day, hour, minute, second] = match
    .slice(1)
    .map((part) => (part === undefined ? 0 : Number(part)))
  const date = new Date(year, month - 1, day, hour, minute, second)
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month - 1 ||
    date.getDate() !== day ||
    date.getHours() !== hour ||
    date.getMinutes() !== minute ||
    date.getSeconds() !== second
  ) {
    return null
  }
  return String(date.getTime())
}

function pad(value: number): string {
  return String(value).padStart(2, "0")
}

export function formatDateTime(value: string | number | Date): string {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export function formatDateTimeMs(value: string | number | Date): string {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return `${formatDateTime(date)}.${String(date.getMilliseconds()).padStart(3, "0")}`
}

export function formatTimeValue(value: string): string {
  return epochMillis.test(value) ? formatDateTime(Number(value)) : value
}

type TimeUnit = "s" | "m" | "h" | "d" | "w" | "M" | "y"

const unitNames: Record<TimeUnit, string> = {
  s: "second",
  m: "minute",
  h: "hour",
  d: "day",
  w: "week",
  M: "month",
  y: "year",
}

const maxRelativeCount = 2147483647

function isTimeUnit(value: string): value is TimeUnit {
  return Object.hasOwn(unitNames, value)
}

function describeRelativeRange(from: string, to: string): string | null {
  if (to !== "now" || !isTimeValue(from)) return null
  const match = relativeExpression.exec(from)
  if (!match || match[1] !== "-" || match[4]) return null
  const count = Number(match[2])
  if (!Number.isSafeInteger(count) || count > maxRelativeCount) return null
  if (!isTimeUnit(match[3])) return null
  return `Last ${durationText(count, match[3])}`
}

function durationText(count: number, unit: TimeUnit): string {
  return `${count} ${unitNames[unit]}${count === 1 ? "" : "s"}`
}

export function describeDuration(text: string): string | null {
  const match = /^([1-9][0-9]*)([smhdwMy])$/.exec(text)
  if (!match) return null
  const count = Number(match[1])
  if (!Number.isSafeInteger(count) || !isTimeUnit(match[2])) return null
  return durationText(count, match[2])
}

export type QuickRange = { label: string; from: string; to: string }

export function describeRange(
  from: string,
  to: string,
  ranges: QuickRange[] = quickRanges
): string {
  const quick = ranges.find((range) => range.from === from && range.to === to)
  return (
    quick?.label ??
    describeRelativeRange(from, to) ??
    `${formatTimeValue(from)} to ${formatTimeValue(to)}`
  )
}

const absoluteUnitMs = { s: 1000, m: 60_000, h: 3_600_000 }

const monthsPerUnit: Record<string, number> = { M: 1, y: 12, fQ: 3, fy: 12 }

const weekStartDay = { sunday: 0, monday: 1, saturday: 6 }

const nowDelayPattern = /^([1-9][0-9]{0,4})([smh])$/

const maxResolvedYear = 9999

export type TimeOptions = {
  zone?: string
  weekStart?: Dashboard["weekStart"]
  fiscalYearStartMonth?: number
  nowDelayMs?: number
}

type TimeSettings = Pick<
  Dashboard,
  "timezone" | "weekStart" | "fiscalYearStartMonth" | "nowDelay"
>

function browserZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

export function requestZone(timezone: string | undefined): string | undefined {
  return timezone === "browser" ? browserZone() : undefined
}

function knownZone(zone: string | undefined): string | undefined {
  if (zone === undefined) return undefined
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: zone })
    return zone
  } catch {
    return undefined
  }
}

function nowDelayMs(value: string | undefined): number | undefined {
  const match = value === undefined ? null : nowDelayPattern.exec(value)
  if (!match) return undefined
  return Number(match[1]) * absoluteUnitMs[match[2] as "s" | "m" | "h"]
}

export function timeOptionsFor(settings: TimeSettings): TimeOptions {
  const { timezone } = settings
  const zone =
    timezone === "browser"
      ? browserZone()
      : timezone === "utc"
        ? undefined
        : timezone
  return {
    zone: knownZone(zone),
    weekStart: settings.weekStart,
    fiscalYearStartMonth: settings.fiscalYearStartMonth,
    nowDelayMs: nowDelayMs(settings.nowDelay),
  }
}

const zoneFormats = new Map<string, Intl.DateTimeFormat>()

function zoneFormat(zone: string): Intl.DateTimeFormat {
  let format = zoneFormats.get(zone)
  if (!format) {
    format = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      hourCycle: "h23",
      year: "numeric",
      month: "numeric",
      day: "numeric",
      hour: "numeric",
      minute: "numeric",
      second: "numeric",
    })
    zoneFormats.set(zone, format)
  }
  return format
}

function zoneOffsetMs(utcMs: number, zone: string): number {
  const parts = zoneFormat(zone).formatToParts(new Date(utcMs))
  const part = (type: string) =>
    Number(parts.find((candidate) => candidate.type === type)?.value)
  const localAsUtc = Date.UTC(
    part("year"),
    part("month") - 1,
    part("day"),
    part("hour"),
    part("minute"),
    part("second")
  )
  return localAsUtc - Math.floor(utcMs / 1000) * 1000
}

type LocalTime = {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
  millisecond: number
  weekday: number
}

function localTime(utcMs: number, zone: string): LocalTime {
  const shifted = new Date(utcMs + zoneOffsetMs(utcMs, zone))
  return {
    year: shifted.getUTCFullYear(),
    month: shifted.getUTCMonth(),
    day: shifted.getUTCDate(),
    hour: shifted.getUTCHours(),
    minute: shifted.getUTCMinutes(),
    second: shifted.getUTCSeconds(),
    millisecond: shifted.getUTCMilliseconds(),
    weekday: shifted.getUTCDay(),
  }
}

function fromLocal(
  zone: string,
  year: number,
  month: number,
  day: number,
  hour = 0,
  minute = 0,
  second = 0,
  millisecond = 0
): number {
  const guess = Date.UTC(year, month, day, hour, minute, second, millisecond)
  if (Number.isNaN(guess)) return NaN
  const guessOffset = zoneOffsetMs(guess, zone)
  const candidate = guess - guessOffset
  const candidateOffset = zoneOffsetMs(candidate, zone)
  return candidateOffset === guessOffset ? candidate : guess - candidateOffset
}

function addMonthsClamped(ms: number, months: number, zone: string): number {
  const local = localTime(ms, zone)
  const first = new Date(Date.UTC(local.year, local.month + months, 1))
  const year = first.getUTCFullYear()
  const month = first.getUTCMonth()
  const lastDay = new Date(Date.UTC(year, month + 1, 0)).getUTCDate()
  return fromLocal(
    zone,
    year,
    month,
    Math.min(local.day, lastDay),
    local.hour,
    local.minute,
    local.second,
    local.millisecond
  )
}

function advance(ms: number, count: number, unit: string, zone: string) {
  if (Object.hasOwn(monthsPerUnit, unit)) {
    return addMonthsClamped(ms, monthsPerUnit[unit] * count, zone)
  }
  if (unit === "d" || unit === "w") {
    const local = localTime(ms, zone)
    return fromLocal(
      zone,
      local.year,
      local.month,
      local.day + (unit === "w" ? 7 : 1) * count,
      local.hour,
      local.minute,
      local.second,
      local.millisecond
    )
  }
  return ms + count * absoluteUnitMs[unit as keyof typeof absoluteUnitMs]
}

function startOf(
  ms: number,
  unit: string,
  zone: string,
  options: TimeOptions
): number {
  const local = localTime(ms, zone)
  const { year, month, day } = local
  const monthsIntoFiscalYear =
    (month - (options.fiscalYearStartMonth ?? 0) + 12) % 12
  switch (unit) {
    case "s":
      return Math.floor(ms / 1000) * 1000
    case "m":
      return ms - local.second * 1000 - local.millisecond
    case "h":
      return (
        ms - local.minute * 60_000 - local.second * 1000 - local.millisecond
      )
    case "d":
      return fromLocal(zone, year, month, day)
    case "w": {
      const first = weekStartDay[options.weekStart ?? "monday"]
      return fromLocal(
        zone,
        year,
        month,
        day - ((local.weekday - first + 7) % 7)
      )
    }
    case "M":
      return fromLocal(zone, year, month, 1)
    case "fQ":
      return fromLocal(zone, year, month - (monthsIntoFiscalYear % 3), 1)
    case "fy":
      return fromLocal(zone, year, month - monthsIntoFiscalYear, 1)
    default:
      return fromLocal(zone, year, 0, 1)
  }
}

function inResolvedYears(ms: number): boolean {
  if (Number.isNaN(ms)) return false
  const year = new Date(ms).getUTCFullYear()
  return year >= 1 && year <= maxResolvedYear
}

export function resolveTime(
  value: string,
  now: number,
  roundUp: boolean,
  options: TimeOptions = {}
): number {
  const match = relativeExpression.exec(value)
  if (!match) {
    return epochMillis.test(value) ? Number(value) : Date.parse(value)
  }
  const zone = options.zone ?? "UTC"
  let time = now - (options.nowDelayMs ?? 0)
  if (match[2]) {
    const count = Number(match[2]) * (match[1] === "-" ? -1 : 1)
    time = advance(time, count, match[3], zone)
  }
  if (match[4] && inResolvedYears(time)) {
    const start = startOf(time, match[4], zone, options)
    time = roundUp ? advance(start, 1, match[4], zone) - 1 : start
  }
  return inResolvedYears(time) ? time : NaN
}

export function isAbsoluteOrdered(
  from: string,
  to: string,
  now = Date.now()
): boolean {
  const start = resolveTime(from, now, false)
  const end = resolveTime(to, now, true)
  if (Number.isNaN(start) || Number.isNaN(end)) return true
  return start < end
}

const relativeUnits: { unit: Intl.RelativeTimeFormatUnit; seconds: number }[] =
  [
    { unit: "year", seconds: 365 * 86400 },
    { unit: "month", seconds: 30 * 86400 },
    { unit: "day", seconds: 86400 },
    { unit: "hour", seconds: 3600 },
    { unit: "minute", seconds: 60 },
  ]

const relativeFormat = new Intl.RelativeTimeFormat("en", { numeric: "auto" })

export function formatRelative(iso: string, now = Date.now()): string {
  const seconds = Math.round((new Date(iso).getTime() - now) / 1000)
  const abs = Math.abs(seconds)
  for (const { unit, seconds: size } of relativeUnits) {
    if (abs >= size) {
      return relativeFormat.format(Math.round(seconds / size), unit)
    }
  }
  return relativeFormat.format(seconds, "second")
}

const slowRelativeTickMs = 30_000
const fastRelativeTickMs = 1000

export function isSubMinuteOffset(iso: string, now: number): boolean {
  const target = Date.parse(iso)
  return !Number.isNaN(target) && Math.abs(target - now) < 60_000
}

export function relativeTickMs(fastCount: number): number {
  return fastCount > 0 ? fastRelativeTickMs : slowRelativeTickMs
}

export function formatShortRelative(iso: string, now = Date.now()): string {
  const target = Date.parse(iso)
  if (Number.isNaN(target)) return iso
  const diff = target - now
  const abs = Math.abs(diff)
  const s = Math.floor(abs / 1000)
  const m = Math.floor(abs / 60_000)
  const h = Math.floor(abs / 3_600_000)
  const d = Math.floor(abs / 86_400_000)
  if (diff > 0) {
    if (s < 60) return "Just now"
    if (m < 60) return `In ${m} min`
    if (h < 24) return `In ${h} h`
    if (d === 1) return "Tomorrow"
    if (d < 730) return `In ${d} days`
    return `In ${Math.floor(d / 365)} years`
  }
  if (s < 5) return "Just now"
  if (s < 60) return `${s} s ago`
  if (m < 60) return `${m} min ago`
  if (h < 24) return `${h} h ago`
  if (d === 1) return "Yesterday"
  if (d < 7) return `${d} days ago`
  if (d < 14) return "Last week"
  if (d < 30) return `${Math.floor(d / 7)} weeks ago`
  if (d < 365) {
    const months = Math.floor(d / 30)
    return months === 1 ? "Last month" : `${months} months ago`
  }
  const years = Math.floor(d / 365)
  return years === 1 ? "Last year" : `${years} years ago`
}

export const refreshOptions = ["off", ...timePickerDefaults.refreshIntervals]

export const refreshPattern = /^[1-9][0-9]{0,4}[smhd]$/

export const liveNowRefresh = "5s"

const refreshUnitMs = { s: 1000, m: 60_000, h: 3_600_000, d: 86_400_000 }
const minRefreshMs = 5000
const maxRefreshMs = 2_147_483_647

export function refreshToMs(value: string | undefined): number | null {
  if (value === undefined || !refreshPattern.test(value)) return null
  const unit = value[value.length - 1] as keyof typeof refreshUnitMs
  const ms = Number(value.slice(0, -1)) * refreshUnitMs[unit]
  return Math.min(Math.max(ms, minRefreshMs), maxRefreshMs)
}
