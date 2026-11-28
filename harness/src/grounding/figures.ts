import { parseFloat } from "../brief/gofmt"

export type Figure = {
  text: string
  value: number
  step: number
  unit: string
  start: number
  end: number
}

type Unit = {
  written: string[]
  name: string
  scales: number[]
  attached?: boolean
}

const units: Unit[] = [
  {
    written: ["milliseconds", "millisecond", "msec", "ms"],
    name: "ms",
    scales: [1, 1e-3, 1e3, 1e6],
  },
  { written: ["µs", "μs"], name: "us", scales: [1, 1e-3, 1e-6, 1e3] },
  {
    written: ["us"],
    name: "us",
    scales: [1, 1e-3, 1e-6, 1e3],
    attached: true,
  },
  {
    written: ["seconds", "second", "secs", "sec", "s"],
    name: "s",
    scales: [1, 1e3, 1e6, 1e9],
  },
  { written: ["percent", "%"], name: "%", scales: [1, 1e-2] },
  { written: ["KiB", "KB", "kB"], name: "KB", scales: [1e3, 1 << 10, 1] },
  { written: ["MiB", "MB"], name: "MB", scales: [1e6, 1 << 20, 1] },
  { written: ["GiB", "GB"], name: "GB", scales: [1e9, 2 ** 30, 1] },
  { written: ["K", "k"], name: "K", scales: [1e3], attached: true },
  { written: ["M"], name: "M", scales: [1e6], attached: true },
  { written: ["x", "×"], name: "x", scales: [1], attached: true },
]

const plainScales = [1]

export const S = "[\\t\\n\\f\\r ]"
const notS = "[^\\t\\n\\f\\r ]"

function anyCase(word: string): string {
  return [...word]
    .map((c) =>
      /[a-z]/.test(c) ? `[${c}${c.toUpperCase()}]` : c.replace(".", "\\.")
    )
    .join("")
}

const numberRe = /\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?/gu

export const fencedCode = /(```|~~~)[\s\S]*?(```|~~~|$)/gu
const inlineCode = /`[^`]*`/gu
export const mdLink = /!?\[[^\]]*\]\([^)]*\)/gu
export const angleLink = /<[a-z]+:\/\/[^>]*>/gu
export const bareURL = new RegExp(
  `\\bhttps?://${notS}*|(?:^|[\\t\\n\\f\\r (])/[ocd]/${notS}*`,
  "giu"
)
const listMarker = new RegExp(`(?<![^\\n])${S}*\\d+[.)]${S}`, "gu")
export const linkTarget = /\]\([^)]*\)/gu

const isoTime =
  /\d{4}-\d{2}-\d{2}(?:[T ]\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?/gu
const slashDay = /\b\d{1,2}\/\d{1,2}\/\d{2,4}\b/gu
const clock = new RegExp(
  `\\b\\d{1,2}:\\d{2}(?::\\d{2}(?:\\.\\d+)?)?(?:${S}?[ap]\\.?m\\.?\\b)?`,
  "giu"
)
const months =
  "(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|june?|july?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)(?:\\.|\\b)"
const monthDay = new RegExp(
  `\\b${months}${S}+\\d{1,2}(?:st|nd|rd|th)?(?:,?${S}+\\d{4})?\\b|\\b\\d{1,2}(?:st|nd|rd|th)?${S}+${months}(?:,?${S}+\\d{4}\\b)?`,
  "giu"
)
const durationWords = [
  "minutes?",
  "mins?",
  "hours?",
  "hrs?",
  "days?",
  "weeks?",
  "months?",
  "years?",
].map(anyCase)
const duration = new RegExp(
  `\\b\\d+(?:\\.\\d+)?${S}?(?:${durationWords.join("|")})\\b|\\b\\d+(?:\\.\\d+)?[mhdw]\\b`,
  "gu"
)

const spansToSkip = [
  fencedCode,
  inlineCode,
  mdLink,
  angleLink,
  bareURL,
  listMarker,
  isoTime,
  slashDay,
  monthDay,
  clock,
  duration,
]

export function blank(text: string): string {
  return blankWith(text, ...spansToSkip)
}

export function blankWith(text: string, ...spans: RegExp[]): string {
  for (const re of spans) {
    text = text.replace(re, (s) => " ".repeat(s.length))
  }
  return text
}

export function valuesIn(text: string): number[] {
  return scan(blank(text)).map((f) => {
    switch (f.unit) {
      case "K":
        return f.value * 1e3
      case "M":
        return f.value * 1e6
    }
    return f.value
  })
}

export function scan(text: string): Figure[] {
  const out: Figure[] = []
  for (const m of text.matchAll(numberRe)) {
    const start = m.index
    const end = start + m[0].length
    if (!standsAlone(text, start)) {
      continue
    }
    const at = unitAt(text, end)
    if (!at) {
      continue
    }
    const parsed = parseNumber(m[0])
    if (!parsed) {
      continue
    }
    out.push({
      text: text.slice(start, at.after).replace(/^ +| +$/g, ""),
      value: parsed.value,
      step: parsed.step,
      unit: at.unit.name,
      start,
      end: at.after,
    })
  }
  return out
}

function lastRune(text: string, end: number): string {
  if (end <= 0) {
    return ""
  }
  const low = text.charCodeAt(end - 1)
  if (low >= 0xdc00 && low <= 0xdfff && end >= 2) {
    const high = text.charCodeAt(end - 2)
    if (high >= 0xd800 && high <= 0xdbff) {
      return text.slice(end - 2, end)
    }
  }
  return text.slice(end - 1, end)
}

export function isWord(r: string): boolean {
  return /^[\p{L}\p{Nd}_]$/u.test(r)
}

function standsAlone(text: string, start: number): boolean {
  if (start === 0) {
    return true
  }
  const prev = lastRune(text, start)
  const before = start - prev.length
  if (
    isWord(prev) ||
    prev === "#" ||
    (prev === "." && before > 0 && /^\p{Nd}$/u.test(lastRune(text, before)))
  ) {
    return false
  }
  if (prev === "-" || prev === "/") {
    return before === 0 || !isWord(lastRune(text, before))
  }
  return true
}

function firstRune(text: string): string {
  const cp = text.codePointAt(0)
  return cp === undefined ? "" : String.fromCodePoint(cp)
}

function wordEnds(rest: string): boolean {
  return rest === "" || !isWord(firstRune(rest))
}

function unitAt(
  text: string,
  end: number
): { unit: Unit; after: number } | undefined {
  const rest = text.slice(end)
  if (rest.startsWith(".") && rest.length > 1 && /[0-9]/.test(rest[1] ?? "")) {
    return undefined
  }
  const spaced = rest.startsWith(" ") ? rest.slice(1) : rest
  const gap = rest.length - spaced.length
  for (const u of units) {
    if (u.attached && gap > 0) {
      continue
    }
    for (const w of u.written) {
      if (spaced.startsWith(w) && wordEnds(spaced.slice(w.length))) {
        return { unit: u, after: end + gap + w.length }
      }
    }
  }
  if (gap === 0 && isWord(firstRune(rest))) {
    return undefined
  }
  return { unit: { written: [], name: "", scales: plainScales }, after: end }
}

function parseNumber(
  written: string
): { value: number; step: number } | undefined {
  const plain = written.replaceAll(",", "")
  const value = parseFloat(plain)
  if (value === undefined) {
    return undefined
  }
  const dot = plain.indexOf(".")
  if (dot >= 0) {
    return { value, step: 1 / 10 ** (plain.length - dot - 1) }
  }
  const zeros = plain.length - plain.replace(/0+$/, "").length
  if (zeros >= 2 && zeros < plain.length) {
    return { value, step: Number("1e" + zeros) }
  }
  return { value, step: 1 }
}

export function scalesOf(name: string): number[] {
  return units.find((u) => u.name === name)?.scales ?? plainScales
}
