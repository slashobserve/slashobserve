import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import { compileNamePattern, type Match } from "@/spec"

import { reduceAny, type Reduced } from "../reducers"

type MatchSubject = {
  frame: PreparedFrame
  field: PreparedField
  seriesName: string
}

export type Matcher = (subject: MatchSubject) => boolean

const never: Matcher = () => false

type FieldsMatch = Extract<Match, { fields: unknown }>["fields"]

type ValueMatch = Extract<Match, { value: unknown }>["value"]

const comparisons: Record<ValueMatch["op"], (a: number, b: number) => boolean> =
  {
    eq: (a, b) => a === b,
    neq: (a, b) => a !== b,
    lt: (a, b) => a < b,
    lte: (a, b) => a <= b,
    gt: (a, b) => a > b,
    gte: (a, b) => a >= b,
  }

function asComparable(result: Reduced | undefined): number | null {
  if (typeof result === "boolean") return result ? 1 : 0
  return typeof result === "number" ? result : null
}

function compileValueMatch({ reducer, op, value }: ValueMatch): Matcher {
  const compare = comparisons[op]
  const reduced = new WeakMap<PreparedField, number | null>()
  return ({ field }) => {
    if (field.type !== "number") return false
    if (!reduced.has(field)) {
      reduced.set(field, asComparable(reduceAny(field.values, reducer)))
    }
    const result = reduced.get(field)
    return result !== null && result !== undefined && compare(result, value)
  }
}

const positional: Record<
  FieldsMatch,
  (frame: PreparedFrame, field: PreparedField) => boolean
> = {
  first: (frame, field) => frame.fields[0] === field,
  firstTime: (frame, field) =>
    frame.fields.find((candidate) => candidate.type === "time") === field,
  all: () => true,
}

export function compileMatch(match: Match): Matcher {
  if ("query" in match) {
    const ref = match.query
    return ({ frame }) => frame.ref === ref
  }
  if ("series" in match) {
    const name = match.series
    return ({ seriesName }) => seriesName === name
  }
  if ("seriesRegex" in match) {
    const pattern = compileNamePattern(match.seriesRegex)
    if (!pattern) return never
    return ({ seriesName }) => pattern.test(seriesName)
  }
  if ("field" in match) {
    const name = match.field
    return ({ field, seriesName }) => field.name === name || seriesName === name
  }
  if ("fieldType" in match) {
    const type = match.fieldType
    return ({ field }) =>
      (field.type === "bool" ? "boolean" : field.type) === type
  }
  if ("value" in match) return compileValueMatch(match.value)
  if ("fields" in match) {
    const select = positional[match.fields]
    return ({ frame, field }) => select(frame, field)
  }
  return never
}
