import type { NumberRule } from "./types"

const decimal = /^-?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i
const whole = /^-?\d+$/
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/
const maxNameLength = 200

export function parseNumber(entry: string, rule: NumberRule): number | null {
  if (!(rule.integer ? whole : decimal).test(entry)) return null
  if (entry.startsWith("-") && rule.min >= 0) return null
  const value = Number(entry)
  if (!Number.isFinite(value)) return null
  if (rule.minExclusive ? value <= rule.min : value < rule.min) return null
  if (value > rule.max) return null
  return value === 0 ? 0 : value
}

function splitList(entry: string): string[] {
  return entry
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item !== "")
}

export function parseTags(entry: string): string[] | null {
  const tags = splitList(entry)
  if (!tags.every((tag) => tagPattern.test(tag))) return null
  return [...new Set(tags)]
}

export function parseNames(entry: string): string[] | null {
  const names = splitList(entry)
  if (names.some((name) => [...name].length > maxNameLength)) return null
  return [...new Set(names)]
}
