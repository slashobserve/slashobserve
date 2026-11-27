const htmlUnsafe = /[<>&\u2028\u2029]/g

function hex(code: number, width: number): string {
  return code.toString(16).padStart(width, "0")
}

export function goJSON(value: unknown): string {
  return (JSON.stringify(value) ?? "null").replace(
    htmlUnsafe,
    (c) => "\\u" + hex(c.charCodeAt(0), 4)
  )
}

const printable = /^[\p{L}\p{M}\p{N}\p{P}\p{S} ]$/u

const quoteEscapes: Record<number, string> = {
  7: "\\a",
  8: "\\b",
  9: "\\t",
  10: "\\n",
  11: "\\v",
  12: "\\f",
  13: "\\r",
}

export function goQuote(s: string): string {
  let out = '"'
  for (const ch of s) {
    const r = ch.codePointAt(0) ?? 0
    if (ch === '"' || ch === "\\") {
      out += "\\" + ch
    } else if (r >= 0xd800 && r <= 0xdfff) {
      out += "\ufffd"
    } else if (printable.test(ch)) {
      out += ch
    } else if (quoteEscapes[r] !== undefined) {
      out += quoteEscapes[r]
    } else if (r < 0x20 || r === 0x7f) {
      out += "\\x" + hex(r, 2)
    } else if (r < 0x10000) {
      out += "\\u" + hex(r, 4)
    } else {
      out += "\\U" + hex(r, 8)
    }
  }
  return out + '"'
}

const goSpace =
  /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/

export function fields(s: string): string[] {
  return s.split(goSpace).filter((f) => f !== "")
}

export function byteLength(s: string): number {
  return Buffer.byteLength(s, "utf8")
}

const rfc3339 =
  /^(\d{4})-(\d\d)-(\d\d)[Tt](\d\d):(\d\d):(\d\d)(?:\.\d+)?(?:([Zz])|([+-])(\d\d):(\d\d))$/

export function unixSeconds(text: string): number {
  const m = rfc3339.exec(text)
  if (!m) {
    throw new Error("not an RFC 3339 time")
  }
  const [, y, mo, d, h, mi, s, , sign, oh, om] = m
  const local =
    Date.UTC(
      Number(y),
      Number(mo) - 1,
      Number(d),
      Number(h),
      Number(mi),
      Number(s)
    ) / 1000
  const offset = sign
    ? (Number(oh) * 3600 + Number(om) * 60) * (sign === "-" ? -1 : 1)
    : 0
  return local - offset
}

type Parts = {
  year: string
  month: string
  day: string
  hour: string
  minute: string
  second: string
}

function partsOf(seconds: number): Parts {
  const iso = new Date(seconds * 1000).toISOString()
  return {
    year: iso.slice(0, 4),
    month: iso.slice(5, 7),
    day: iso.slice(8, 10),
    hour: iso.slice(11, 13),
    minute: iso.slice(14, 16),
    second: iso.slice(17, 19),
  }
}

export function formatRFC3339(seconds: number): string {
  const p = partsOf(seconds)
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}:${p.second}Z`
}

export function formatDate(seconds: number): string {
  const p = partsOf(seconds)
  return `${p.year}-${p.month}-${p.day}`
}

export function formatClock(seconds: number): string {
  const p = partsOf(seconds)
  return `${p.hour}:${p.minute}`
}

export function formatMinute(seconds: number): string {
  return `${formatDate(seconds)} ${formatClock(seconds)}`
}

export function formatSecond(seconds: number): string {
  return `${formatMinute(seconds)}:${partsOf(seconds).second}`
}

export function hasSeconds(seconds: number): boolean {
  return partsOf(seconds).second !== "00"
}
