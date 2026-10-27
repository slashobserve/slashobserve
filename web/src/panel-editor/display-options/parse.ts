import type { NumberRule } from "./types"

const decimal = /^-?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i
const whole = /^-?\d+$/
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/

export function parseNumber(entry: string, rule: NumberRule): number | null {
  if (!(rule.integer ? whole : decimal).test(entry)) return null
  if (entry.startsWith("-") && rule.min >= 0) return null
  const value = Number(entry)
  if (!Number.isFinite(value)) return null
  if (rule.minExclusive ? value <= rule.min : value < rule.min) return null
  if (value > rule.max) return null
  return value === 0 ? 0 : value
}

export function parseTags(entry: string): string[] | null {
  const tags = entry
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => tag !== "")
  if (!tags.every((tag) => tagPattern.test(tag))) return null
  return [...new Set(tags)]
}
