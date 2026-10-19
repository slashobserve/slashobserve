import { compileNamePattern, type Mapping } from "@/spec"

export type MappingHit = { mapping: Mapping; text?: string }

type SpecialKind = Extract<Mapping, { special: unknown }>["special"]

const compiled = new WeakMap<Mapping, RegExp | null>()

function patternOf(mapping: Mapping, pattern: string): RegExp | null {
  if (!compiled.has(mapping)) compiled.set(mapping, compileNamePattern(pattern))
  return compiled.get(mapping) ?? null
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value)
}

function parsedNumber(text: string): number | null {
  if (text.trim() === "") return null
  const parsed = Number(text)
  return Number.isFinite(parsed) ? parsed : null
}

function specialKindOf(value: unknown): SpecialKind | null {
  if (value === null || value === undefined) return "null"
  if (typeof value === "number" && Number.isNaN(value)) return "nan"
  if (value === "") return "empty"
  if (value === true || value === "true") return "true"
  if (value === false || value === "false") return "false"
  return null
}

function matchesValue(
  mapping: Extract<Mapping, { value: string }>,
  value: unknown
) {
  if (mapping.value === String(value)) return true
  if (!isFiniteNumber(value)) return false
  return parsedNumber(mapping.value) === value
}

function matchesRange(
  mapping: Extract<Mapping, { range: unknown[] }>,
  value: unknown
) {
  if (!isFiniteNumber(value)) return false
  const [from, to] = mapping.range
  if (isFiniteNumber(from) && value < from) return false
  if (isFiniteNumber(to) && value > to) return false
  return true
}

export function matchSpecial(
  mappings: readonly Mapping[],
  kind: SpecialKind
): Mapping | null {
  return (
    mappings.find(
      (mapping) => "special" in mapping && mapping.special === kind
    ) ?? null
  )
}

export function matchMapping(
  mappings: readonly Mapping[],
  value: unknown
): MappingHit | null {
  for (const mapping of mappings) {
    if ("value" in mapping) {
      if (matchesValue(mapping, value)) return { mapping, text: mapping.text }
    } else if ("range" in mapping) {
      if (matchesRange(mapping, value)) return { mapping, text: mapping.text }
    } else if ("regex" in mapping) {
      const pattern = patternOf(mapping, mapping.regex)
      const source = String(value)
      if (pattern && pattern.test(source)) {
        pattern.lastIndex = 0
        return {
          mapping,
          text:
            mapping.text === undefined
              ? undefined
              : source.replace(pattern, mapping.text),
        }
      }
    } else if (mapping.special === specialKindOf(value)) {
      return { mapping, text: mapping.text }
    }
  }
  return null
}
