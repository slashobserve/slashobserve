import { rowsOf } from "./builder"
import { isSearchFamily } from "./search"

const shownValues = 5

export function firstValueClause(
  type: string,
  query: string | undefined
): { field: string; value: string } | null {
  if (query === undefined || !isSearchFamily(type)) return null
  const row = rowsOf(query)?.find(
    (candidate) => candidate.op === "is" && candidate.field !== ""
  )
  return row === undefined ? null : { field: row.field, value: row.value }
}

export function valueMissText(
  clause: { field: string; value: string },
  seen: readonly string[]
): string | null {
  if (seen.length === 0 || seen.includes(clause.value)) return null
  const lower = clause.value.toLowerCase()
  const sameLetters = seen.find((value) => value.toLowerCase() === lower)
  const listed = seen.slice(0, shownValues).join(", ")
  const head = `No document has ${clause.field} = ${clause.value} in this range`
  return sameLetters === undefined
    ? `${head}; values seen: ${listed}.`
    : `${head}, but it has ${sameLetters}; values are case sensitive, so seen: ${listed}.`
}
