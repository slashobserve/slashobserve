import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import { compileNamePattern, type Match } from "@/spec"

export type MatchSubject = {
  frame: PreparedFrame
  field: PreparedField
  seriesName: string
  displayName: string
}

export type Matcher = (subject: MatchSubject) => boolean

const never: Matcher = () => false

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
    return ({ field, displayName }) =>
      field.name === name || displayName === name
  }
  if ("fieldType" in match) {
    const type = match.fieldType
    return ({ field }) =>
      (field.type === "bool" ? "boolean" : field.type) === type
  }
  return never
}
