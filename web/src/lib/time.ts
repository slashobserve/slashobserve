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
const epochMillis = /^[0-9]{1,15}$/
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

export function describeRange(from: string, to: string): string {
  const quick = quickRanges.find(
    (range) => range.from === from && range.to === to
  )
  return quick
    ? quick.label
    : `${formatTimeValue(from)} to ${formatTimeValue(to)}`
}

export function isAbsoluteOrdered(from: string, to: string): boolean {
  if (epochMillis.test(from) && epochMillis.test(to)) {
    return Number(from) < Number(to)
  }
  return true
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

export function refreshToMs(value: string | undefined): number | null {
  if (value === undefined || !refreshPattern.test(value)) return null
  const unit = value[value.length - 1] as keyof typeof refreshUnitMs
  return Math.max(Number(value.slice(0, -1)) * refreshUnitMs[unit], 5000)
}
