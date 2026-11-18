export type TextRule = { max: number; lines: boolean }

export function lineRule(max: number): TextRule {
  return { max, lines: false }
}

export function textRule(max: number): TextRule {
  return { max, lines: true }
}

function isSpace(c: number): boolean {
  return (
    (c >= 0x09 && c <= 0x0d) ||
    c === 0x20 ||
    c === 0x85 ||
    c === 0xa0 ||
    c === 0x1680 ||
    (c >= 0x2000 && c <= 0x200a) ||
    c === 0x2028 ||
    c === 0x2029 ||
    c === 0x202f ||
    c === 0x205f ||
    c === 0x3000
  )
}

function isFiller(c: number): boolean {
  return [0x34f, 0x115f, 0x1160, 0x2800, 0x3164, 0xffa0].includes(c)
}

function isZeroWidth(c: number): boolean {
  return (
    c === 0x200b ||
    c === 0xfeff ||
    c === 0xad ||
    c === 0x180e ||
    (c >= 0x2060 && c <= 0x2064) ||
    (c >= 0x206a && c <= 0x206f) ||
    isFiller(c)
  )
}

function isEdge(c: number): boolean {
  return isSpace(c) || isZeroWidth(c) || c === 0x200c || c === 0x200d
}

const format = /^\p{Cf}$/u
const control = /^\p{Cc}$/u

function isInvisible(c: number): boolean {
  return isSpace(c) || isFiller(c) || format.test(String.fromCodePoint(c))
}

function isOverride(c: number): boolean {
  return (c >= 0x202a && c <= 0x202e) || (c >= 0x2066 && c <= 0x2069)
}

function isTag(c: number): boolean {
  return c >= 0xe0000 && c <= 0xe007f
}

function isControl(c: number, rule: TextRule): boolean {
  if (rule.lines && (c === 0x0a || c === 0x0d || c === 0x09)) return false
  return control.test(String.fromCodePoint(c))
}

function codePoints(text: string): number[] {
  return Array.from(text, (character) => character.codePointAt(0) ?? 0)
}

export function cleanText(raw: string): string {
  const points = codePoints(raw)
  let start = 0
  let end = points.length
  while (start < end && isEdge(points[start] ?? 0)) start++
  while (end > start && isEdge(points[end - 1] ?? 0)) end--
  return String.fromCodePoint(...points.slice(start, end))
}

function problemOf(field: string, raw: string, rule: TextRule): string | null {
  const points = codePoints(raw)
  if (!points.some((c) => !isInvisible(c))) {
    return points.some((c) => !isSpace(c))
      ? `${field} has no visible character; enter 1 to ${rule.max} characters of visible text`
      : `${field} must not be empty; enter 1 to ${rule.max} characters`
  }
  const text = codePoints(cleanText(raw))
  if (text.some((c) => isControl(c, rule))) {
    return rule.lines
      ? `${field} must not contain control characters other than line breaks and tabs; remove them`
      : `${field} must be one line without control characters; remove line breaks, tabs and other control characters`
  }
  if (text.some(isOverride)) {
    return `${field} must not contain bidirectional override characters (U+202A to U+202E, U+2066 to U+2069); remove them`
  }
  if (text.some((c) => isTag(c) || (!rule.lines && isZeroWidth(c)))) {
    return `${field} must not contain zero-width or hidden characters; remove them`
  }
  if (text.length > rule.max) {
    return `${field} must be at most ${rule.max} characters; shorten it`
  }
  return null
}

export function textError(
  field: string,
  raw: string,
  rule: TextRule
): string | null {
  const message = problemOf(field, raw, rule)
  return message === null
    ? null
    : `${message.charAt(0).toUpperCase()}${message.slice(1)}.`
}
