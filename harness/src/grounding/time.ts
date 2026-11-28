const rfc3339 =
  /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(?:[.,](\d+))?(?:(Z)|([+-])(\d\d):(\d\d))$/

function daysIn(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

export function parseTime(text: string): number | undefined {
  const m = rfc3339.exec(text)
  if (!m) {
    return undefined
  }
  const [, y, mo, d, h, mi, s, frac, , sign, oh, om] = m
  const year = Number(y)
  const month = Number(mo)
  const day = Number(d)
  const hour = Number(h)
  const minute = Number(mi)
  const second = Number(s)
  if (
    month < 1 ||
    month > 12 ||
    day < 1 ||
    day > daysIn(year, month) ||
    hour > 23 ||
    minute > 59 ||
    second > 59 ||
    (sign !== undefined && (Number(oh) > 23 || Number(om) > 59))
  ) {
    return undefined
  }
  const date = new Date(0)
  date.setUTCFullYear(year, month - 1, day)
  date.setUTCHours(hour, minute, second, 0)
  let ms = date.getTime() + Number(((frac ?? "") + "000").slice(0, 3))
  if (sign !== undefined) {
    const offset = (Number(oh) * 60 + Number(om)) * 60000
    ms += sign === "-" ? offset : -offset
  }
  return ms
}

function pad(n: number, width: number): string {
  return String(n).padStart(width, "0")
}

export function formatTime(ms: number): string {
  const d = new Date(Math.floor(ms / 1000) * 1000)
  return (
    `${pad(d.getUTCFullYear(), 4)}-${pad(d.getUTCMonth() + 1, 2)}-${pad(d.getUTCDate(), 2)}` +
    `T${pad(d.getUTCHours(), 2)}:${pad(d.getUTCMinutes(), 2)}:${pad(d.getUTCSeconds(), 2)}Z`
  )
}

export const minute = 60000
