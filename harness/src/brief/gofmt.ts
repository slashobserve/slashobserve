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

const leadingSpace = new RegExp("^(?:" + goSpace.source + ")")
const trailingSpace = new RegExp("(?:" + goSpace.source + ")$")
const oneSpace = new RegExp("^(?:" + goSpace.source + ")$")

export function trimSpace(s: string): string {
  return s.replace(leadingSpace, "").replace(trailingSpace, "")
}

export function isSpace(ch: string): boolean {
  return oneSpace.test(ch)
}

export function toLower(s: string): string {
  let out = ""
  for (const ch of s) {
    const lower = ch.toLowerCase()
    out +=
      [...lower].length === 1
        ? lower
        : String.fromCodePoint(lower.codePointAt(0) ?? 0)
  }
  return out
}

export function compareBytes(a: string, b: string): number {
  return Buffer.compare(Buffer.from(a, "utf8"), Buffer.from(b, "utf8"))
}

export function clipBytes(s: string, n: number): string {
  const raw = Buffer.from(s, "utf8")
  if (raw.length <= n) {
    return s
  }
  let cut = n
  while (cut > 0 && ((raw[cut] ?? 0) & 0xc0) === 0x80) {
    cut--
  }
  return raw.subarray(0, cut).toString("utf8")
}

type Decimal = { digits: string; point: number }

function exactDecimal(v: number): Decimal {
  const view = new DataView(new ArrayBuffer(8))
  view.setFloat64(0, v)
  const bits = view.getBigUint64(0)
  const biased = Number((bits >> 52n) & 0x7ffn)
  let mant = bits & 0xfffffffffffffn
  let exp = biased - 1075
  if (biased === 0) {
    exp = -1074
  } else {
    mant |= 1n << 52n
  }
  let digits: string
  let point: number
  if (exp >= 0) {
    digits = (mant << BigInt(exp)).toString()
    point = digits.length
  } else {
    digits = (mant * 5n ** BigInt(-exp)).toString()
    point = digits.length + exp
  }
  return { digits: digits.replace(/0+$/, ""), point }
}

function roundDecimal(d: Decimal, nd: number): Decimal {
  if (nd < 0) {
    return { digits: "", point: d.point }
  }
  if (nd >= d.digits.length) {
    return d
  }
  const next = d.digits.charCodeAt(nd) - 48
  let up = next > 5
  if (next === 5) {
    up =
      nd + 1 < d.digits.length ||
      (nd > 0 && (d.digits.charCodeAt(nd - 1) - 48) % 2 === 1)
  }
  let kept = d.digits.slice(0, nd)
  let point = d.point
  if (up) {
    let i = kept.length - 1
    while (i >= 0 && kept[i] === "9") {
      i--
    }
    if (i < 0) {
      kept = "1"
      point++
    } else {
      kept = kept.slice(0, i) + String.fromCharCode(kept.charCodeAt(i) + 1)
    }
  }
  return { digits: kept.replace(/0+$/, ""), point }
}

function shortestDecimal(v: number): Decimal {
  const [mantissa = "", exponent = "0"] = v.toExponential().split("e")
  return {
    digits: mantissa.replace(".", "").replace(/0+$/, ""),
    point: Number(exponent) + 1,
  }
}

function fmtE(d: Decimal, prec: number, sign: string): string {
  let out = sign + (d.digits[0] ?? "0")
  if (prec > 0) {
    out += "."
    for (let i = 1; i <= prec; i++) {
      out += d.digits[i] ?? "0"
    }
  }
  const exp = d.digits === "" ? 0 : d.point - 1
  const abs = Math.abs(exp)
  return out + "e" + (exp < 0 ? "-" : "+") + (abs < 10 ? "0" : "") + abs
}

function fmtF(d: Decimal, prec: number, sign: string): string {
  let out = sign
  if (d.point > 0) {
    for (let i = 0; i < d.point; i++) {
      out += d.digits[i] ?? "0"
    }
  } else {
    out += "0"
  }
  if (prec > 0) {
    out += "."
    for (let i = 0; i < prec; i++) {
      const j = d.point + i
      out += j < 0 ? "0" : (d.digits[j] ?? "0")
    }
  }
  return out
}

export function formatFloat(
  v: number,
  fmt: "e" | "f" | "g",
  prec: number
): string {
  if (Number.isNaN(v)) {
    return "NaN"
  }
  if (!Number.isFinite(v)) {
    return v > 0 ? "+Inf" : "-Inf"
  }
  const sign = v < 0 || Object.is(v, -0) ? "-" : ""
  const abs = Math.abs(v)
  const shortest = prec < 0
  let d: Decimal
  if (abs === 0) {
    d = { digits: "", point: 1 }
  } else if (shortest) {
    d = shortestDecimal(abs)
  } else {
    const exact = exactDecimal(abs)
    switch (fmt) {
      case "e":
        d = roundDecimal(exact, prec + 1)
        break
      case "f":
        d = roundDecimal(exact, exact.point + prec)
        break
      default:
        d = roundDecimal(exact, prec === 0 ? 1 : prec)
    }
  }
  const nd = d.digits.length
  switch (fmt) {
    case "e":
      return fmtE(d, shortest ? Math.max(nd - 1, 0) : prec, sign)
    case "f":
      return fmtF(d, shortest ? Math.max(nd - d.point, 0) : prec, sign)
  }
  let p = shortest ? nd : prec === 0 ? 1 : prec
  let eprec = p
  if (eprec > nd && nd >= d.point) {
    eprec = nd
  }
  if (shortest) {
    eprec = 6
  }
  const exp = d.point - 1
  if (exp < -4 || exp >= eprec) {
    if (p > nd) {
      p = nd
    }
    return fmtE(d, Math.max(p - 1, 0), sign)
  }
  if (p > d.point) {
    p = nd
  }
  return fmtF(d, Math.max(p - d.point, 0), sign)
}

const goDecimal =
  /^[+-]?(?:\d(?:_?\d)*(?:\.(?:\d(?:_?\d)*)?)?|\.\d(?:_?\d)*)(?:[eE][+-]?\d(?:_?\d)*)?$/
const goHex =
  /^([+-]?)0[xX]_?([0-9a-fA-F](?:_?[0-9a-fA-F])*)?(?:\.([0-9a-fA-F](?:_?[0-9a-fA-F])*)?)?[pP]([+-]?\d(?:_?\d)*)$/
const goSpecial = /^[+-]?(?:inf|infinity|nan)$/i

export function parseFloat(s: string): number | undefined {
  if (goSpecial.test(s)) {
    const lower = s.toLowerCase()
    if (lower.endsWith("nan")) {
      return lower === "nan" ? Number.NaN : undefined
    }
    return lower.startsWith("-") ? -Infinity : Infinity
  }
  if (goDecimal.test(s)) {
    const v = Number(s.replaceAll("_", ""))
    return Number.isFinite(v) ? v : undefined
  }
  const hex = goHex.exec(s)
  if (hex && (hex[2] !== undefined || hex[3] !== undefined)) {
    const whole = (hex[2] ?? "").replaceAll("_", "")
    const frac = (hex[3] ?? "").replaceAll("_", "")
    const scale = Number((hex[4] ?? "0").replaceAll("_", "")) - 4 * frac.length
    const v = Number.parseInt(whole + frac || "0", 16) * 2 ** scale
    if (!Number.isFinite(v)) {
      return undefined
    }
    return hex[1] === "-" ? -v : v
  }
  return undefined
}

const durationUnits: [string, number][] = [
  ["h", 3600e9],
  ["m", 60e9],
]

export function formatDuration(nanos: number): string {
  if (nanos === 0) {
    return "0s"
  }
  const sign = nanos < 0 ? "-" : ""
  let n = Math.abs(nanos)
  if (n < 1e9) {
    const [unit, scale] =
      n < 1e3 ? ["ns", 1] : n < 1e6 ? ["µs", 1e3] : ["ms", 1e6]
    return sign + formatFloat(n / scale, "f", -1) + unit
  }
  let out = ""
  for (const [unit, scale] of durationUnits) {
    const whole = Math.floor(n / scale)
    if (whole > 0 || out !== "") {
      out += whole + unit
      n -= whole * scale
    }
  }
  return sign + out + formatFloat(n / 1e9, "f", -1) + "s"
}
