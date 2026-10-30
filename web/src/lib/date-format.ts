const tokens = /YYYY|YY|SSS|MM|M|DD|D|HH|H|hh|h|mm|ss|A|a/g

function pad(value: number, width = 2): string {
  return String(value).padStart(width, "0")
}

function hour12(hours: number): number {
  return hours % 12 === 0 ? 12 : hours % 12
}

type Parts = {
  year: number
  month: number
  day: number
  hours: number
  minutes: number
  seconds: number
  millis: number
}

function partsOf(date: Date, utc: boolean): Parts {
  return utc
    ? {
        year: date.getUTCFullYear(),
        month: date.getUTCMonth() + 1,
        day: date.getUTCDate(),
        hours: date.getUTCHours(),
        minutes: date.getUTCMinutes(),
        seconds: date.getUTCSeconds(),
        millis: date.getUTCMilliseconds(),
      }
    : {
        year: date.getFullYear(),
        month: date.getMonth() + 1,
        day: date.getDate(),
        hours: date.getHours(),
        minutes: date.getMinutes(),
        seconds: date.getSeconds(),
        millis: date.getMilliseconds(),
      }
}

function render(token: string, parts: Parts): string {
  switch (token) {
    case "YYYY":
      return pad(parts.year, 4)
    case "YY":
      return pad(parts.year % 100)
    case "MM":
      return pad(parts.month)
    case "M":
      return String(parts.month)
    case "DD":
      return pad(parts.day)
    case "D":
      return String(parts.day)
    case "HH":
      return pad(parts.hours)
    case "H":
      return String(parts.hours)
    case "hh":
      return pad(hour12(parts.hours))
    case "h":
      return String(hour12(parts.hours))
    case "mm":
      return pad(parts.minutes)
    case "ss":
      return pad(parts.seconds)
    case "SSS":
      return pad(parts.millis, 3)
    case "A":
      return parts.hours < 12 ? "AM" : "PM"
    default:
      return parts.hours < 12 ? "am" : "pm"
  }
}

export function formatDate(ms: number, format: string, utc = false): string {
  const date = new Date(ms)
  if (Number.isNaN(date.getTime())) return String(ms)
  const parts = partsOf(date, utc)
  return format.replace(tokens, (token) => render(token, parts))
}

const escaped = /[.*+?^${}()|[\]\\]/g

function digits(token: string): string {
  switch (token) {
    case "YYYY":
      return "(\\d{4})"
    case "SSS":
      return "(\\d{3})"
    case "M":
    case "D":
    case "H":
    case "h":
      return "(\\d{1,2})"
    case "A":
    case "a":
      return "([AaPp][Mm])"
    default:
      return "(\\d{2})"
  }
}

export function parseDate(
  text: string,
  format: string,
  utc = false
): number | null {
  const order: string[] = []
  let source = ""
  let last = 0
  for (const match of format.matchAll(tokens)) {
    source += format.slice(last, match.index).replace(escaped, "\\$&")
    source += digits(match[0])
    order.push(match[0])
    last = match.index + match[0].length
  }
  source += format.slice(last).replace(escaped, "\\$&")
  const found = new RegExp(`^${source}$`).exec(text.trim())
  if (!found) return null
  const parts: Parts = {
    year: 1970,
    month: 1,
    day: 1,
    hours: 0,
    minutes: 0,
    seconds: 0,
    millis: 0,
  }
  let pm: boolean | null = null
  order.forEach((token, i) => {
    const raw = found[i + 1]
    const value = Number(raw)
    switch (token) {
      case "YYYY":
        parts.year = value
        break
      case "YY":
        parts.year = 2000 + value
        break
      case "MM":
      case "M":
        parts.month = value
        break
      case "DD":
      case "D":
        parts.day = value
        break
      case "HH":
      case "H":
      case "hh":
      case "h":
        parts.hours = value
        break
      case "mm":
        parts.minutes = value
        break
      case "ss":
        parts.seconds = value
        break
      case "SSS":
        parts.millis = value
        break
      default:
        pm = raw.toLowerCase() === "pm"
    }
  })
  if (pm !== null) parts.hours = (parts.hours % 12) + (pm ? 12 : 0)
  const date = utc
    ? new Date(
        Date.UTC(
          parts.year,
          parts.month - 1,
          parts.day,
          parts.hours,
          parts.minutes,
          parts.seconds,
          parts.millis
        )
      )
    : new Date(
        parts.year,
        parts.month - 1,
        parts.day,
        parts.hours,
        parts.minutes,
        parts.seconds,
        parts.millis
      )
  const back = partsOf(date, utc)
  const same =
    back.year === parts.year &&
    back.month === parts.month &&
    back.day === parts.day
  return same ? date.getTime() : null
}
