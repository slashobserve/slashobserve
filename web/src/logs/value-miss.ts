import { rowsOf } from "./builder"
import { isSearchFamily } from "./search"

const shownValues = 5

export interface ValueClause {
  field: string
  value: string
}

export interface SeenValues {
  values: readonly string[]
  complete: boolean
}

export function valueClauses(
  type: string,
  query: string | undefined
): ValueClause[] {
  if (query === undefined || !isSearchFamily(type)) return []
  return (rowsOf(query) ?? [])
    .filter((row) => row.op === "is" && row.field !== "")
    .map((row) => ({ field: row.field, value: row.value }))
}

export function firstValueMiss(
  clauses: readonly ValueClause[],
  seenByField: ReadonlyMap<string, SeenValues>
): string | null {
  for (const clause of clauses) {
    const seen = seenByField.get(clause.field)
    const text =
      seen === undefined
        ? null
        : valueMissText(clause, seen.values, seen.complete)
    if (text !== null) return text
  }
  return null
}

export function valueMissText(
  clause: ValueClause,
  seen: readonly string[],
  complete = true
): string | null {
  if (seen.length === 0 || seen.includes(clause.value)) return null
  const lower = clause.value.toLowerCase()
  const sameLetters = seen.find((value) => value.toLowerCase() === lower)
  if (sameLetters === undefined && !complete) return null
  const listed = seen.slice(0, shownValues).join(", ")
  const head = `No document has ${clause.field} = ${clause.value} in this range`
  return sameLetters === undefined
    ? `${head}; values seen: ${listed}.`
    : `${head}, but it has ${sameLetters}; values are case sensitive, so seen: ${listed}.`
}
