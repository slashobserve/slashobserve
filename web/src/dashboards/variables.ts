import type { Selections } from "@/api/keys"
import type { Variable } from "@/spec"

import { ALL } from "./search"

export type VariableOptions = Record<string, string[] | undefined>

export function sortSelections(selections: Selections): Selections {
  const out: Selections = {}
  for (const name of Object.keys(selections).sort()) {
    out[name] = selections[name]
  }
  return out
}

export function earlierSelections(
  variables: Variable[],
  index: number,
  urlVars: Record<string, string[]>
): Selections {
  const out: Selections = {}
  for (const variable of variables.slice(0, index)) {
    const values = urlVars[variable.name]
    if (values) out[variable.name] = values
  }
  return sortSelections(out)
}

function fromUrl(variable: Variable, values: string[]): string[] {
  if (variable.includeAll && values.includes(ALL)) return [ALL]
  const concrete = values.filter((value) => value !== ALL)
  if (concrete.length === 0) return []
  return variable.multi ? concrete : [concrete[0]]
}

export function resolveSelections(
  variables: Variable[],
  urlVars: Record<string, string[]>,
  options: VariableOptions
): { selections: Selections; pending: boolean } {
  const selections: Selections = {}
  let pending = false
  for (const variable of variables) {
    const explicit = fromUrl(variable, urlVars[variable.name] ?? [])
    if (explicit.length > 0) {
      selections[variable.name] = explicit
    } else if (variable.includeAll) {
      selections[variable.name] = [ALL]
    } else {
      const available = options[variable.name]
      if (available === undefined) pending = true
      else if (available.length > 0) selections[variable.name] = [available[0]]
    }
  }
  return { selections, pending }
}

export function applyVariableChange(
  variables: Variable[],
  urlVars: Record<string, string[]>,
  name: string,
  values: string[]
): Record<string, string[]> {
  const index = variables.findIndex((variable) => variable.name === name)
  const later = new Set(variables.slice(index + 1).map((v) => v.name))
  const next: Record<string, string[]> = {}
  for (const [key, value] of Object.entries(urlVars)) {
    if (key !== name && !later.has(key)) next[key] = value
  }
  if (values.length > 0) next[name] = values
  return next
}

export function normalizeMultiSelection(
  previous: string[],
  next: string[],
  includeAll: boolean
): string[] {
  if (!includeAll) return next
  if (next.includes(ALL) && !previous.includes(ALL)) return [ALL]
  return next.filter((value) => value !== ALL)
}
