import { resolveTime, zoneOffsetMs, zonedDateTime } from "@/lib/time"

const absoluteTime = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?$/

const earliestMs = 0
const latestMs = Date.UTC(2100, 0, 1) - 1

export function zoneLabel(ms: number, zone: string | undefined): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: zone,
    timeZoneName: "short",
  }).formatToParts(new Date(ms))
  return parts.find((part) => part.type === "timeZoneName")?.value ?? ""
}

export function timeWithZone(
  value: string | number,
  zone: string | undefined
): string {
  const ms = new Date(value).getTime()
  if (Number.isNaN(ms)) return String(value)
  return `${zonedDateTime(ms, zone)} ${zoneLabel(ms, zone)}`.trim()
}

function inZone(fields: number[], zone: string | undefined): number | null {
  const [year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0] =
    fields
  const text = (ms: number) => zonedDateTime(ms, zone)
  const wanted = `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")} ${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}:${String(second).padStart(2, "0")}`
  let ms: number
  if (zone === undefined) {
    ms = new Date(year, month - 1, day, hour, minute, second).getTime()
  } else {
    const asUtc = Date.UTC(year, month - 1, day, hour, minute, second)
    ms = asUtc - zoneOffsetMs(asUtc, zone)
    ms = asUtc - zoneOffsetMs(ms, zone)
  }
  return Number.isNaN(ms) || text(ms) !== wanted ? null : ms
}

export function parseAnnotationTime(
  input: string,
  zone: string | undefined,
  now: number
): number | null {
  const value = input.trim()
  if (value === "") return null
  const match = absoluteTime.exec(value)
  const ms = match
    ? inZone(
        match.slice(1).map((part) => (part === undefined ? 0 : Number(part))),
        zone
      )
    : resolveTime(value, now, false, { zone })
  if (ms === null || !Number.isFinite(ms)) return null
  return ms >= earliestMs && ms <= latestMs ? ms : null
}
