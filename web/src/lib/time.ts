export const quickRanges: { label: string; from: string; to: string }[] = [
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
  /^(now([+-][1-9][0-9]*[smhdwMy])?(\/[smhdwMy])?|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2}))$/
const epochMillis = /^[0-9]{10,15}$/
const relativeExpression =
  /^now(?:([+-])([0-9]+)([smhdwMy]))?(?:\/([smhdwMy]))?$/
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
  return `Last ${count} ${unitNames[match[3]]}${count === 1 ? "" : "s"}`
}

export function describeRange(from: string, to: string): string {
  const quick = quickRanges.find(
    (range) => range.from === from && range.to === to
  )
  return (
    quick?.label ??
    describeRelativeRange(from, to) ??
    `${formatTimeValue(from)} to ${formatTimeValue(to)}`
  )
}

const fixedUnitMs = {
  s: 1000,
  m: 60_000,
  h: 3_600_000,
  d: 86_400_000,
  w: 604_800_000,
}

function addMonthsClamped(time: Date, months: number): Date {
  const first = new Date(
    Date.UTC(time.getUTCFullYear(), time.getUTCMonth() + months, 1)
  )
  const lastDay = new Date(
    Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)
  ).getUTCDate()
  return new Date(
    Date.UTC(
      first.getUTCFullYear(),
      first.getUTCMonth(),
      Math.min(time.getUTCDate(), lastDay),
      time.getUTCHours(),
      time.getUTCMinutes(),
      time.getUTCSeconds(),
      time.getUTCMilliseconds()
    )
  )
}

function advance(time: Date, count: number, unit: string): Date {
  if (unit === "M") return addMonthsClamped(time, count)
  if (unit === "y") return addMonthsClamped(time, 12 * count)
  return new Date(
    time.getTime() + count * fixedUnitMs[unit as keyof typeof fixedUnitMs]
  )
}

function startOf(time: Date, unit: string): Date {
  const year = time.getUTCFullYear()
  const month = time.getUTCMonth()
  const day = time.getUTCDate()
  switch (unit) {
    case "s":
    case "m":
    case "h": {
      const size = fixedUnitMs[unit]
      return new Date(Math.floor(time.getTime() / size) * size)
    }
    case "d":
      return new Date(Date.UTC(year, month, day))
    case "w":
      return new Date(Date.UTC(year, month, day - ((time.getUTCDay() + 6) % 7)))
    case "M":
      return new Date(Date.UTC(year, month, 1))
    default:
      return new Date(Date.UTC(year, 0, 1))
  }
}

function resolveTime(value: string, now: number, roundUp: boolean): number {
  const match = relativeExpression.exec(value)
  if (!match) {
    return epochMillis.test(value) ? Number(value) : Date.parse(value)
  }
  let time = new Date(now)
  if (match[2]) {
    const count = Number(match[2]) * (match[1] === "-" ? -1 : 1)
    time = advance(time, count, match[3])
  }
  if (match[4]) {
    const start = startOf(time, match[4])
    time = roundUp ? new Date(advance(start, 1, match[4]).getTime() - 1) : start
  }
  return time.getTime()
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

export function formatShortRelative(iso: string, now = Date.now()): string {
  const target = Date.parse(iso)
  if (Number.isNaN(target)) return iso
  const diff = target - now
  const abs = Math.abs(diff)
  const s = Math.round(abs / 1000)
  const m = Math.floor(abs / 60_000)
  const h = Math.floor(abs / 3_600_000)
  const d = Math.floor(abs / 86_400_000)
  if (diff > 0) {
    if (s < 60) return "In under a minute"
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

export const refreshOptions = [
  "off",
  "5s",
  "10s",
  "30s",
  "1m",
  "5m",
  "15m",
  "30m",
  "1h",
]

export const refreshPattern = /^[1-9][0-9]{0,4}[smh]$/

const refreshUnitMs = { s: 1000, m: 60_000, h: 3_600_000 }
const minRefreshMs = 5000
const maxRefreshMs = 86_400_000

export function refreshToMs(value: string | undefined): number | null {
  if (value === undefined || !refreshPattern.test(value)) return null
  const unit = value[value.length - 1] as keyof typeof refreshUnitMs
  const ms = Number(value.slice(0, -1)) * refreshUnitMs[unit]
  return Math.min(Math.max(ms, minRefreshMs), maxRefreshMs)
}
