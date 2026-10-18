import type { Selections } from "@/api/keys"
import type { CustomVariable, QueryVariable, Variable } from "@/spec"

import { ALL } from "./search"

export type ControlVariable = QueryVariable | CustomVariable

export function controlVariables(variables: Variable[]): ControlVariable[] {
  return variables.filter(
    (variable): variable is ControlVariable =>
      variable.type === "query" || variable.type === "custom"
  )
}

export class VariableLoadError extends Error {
  constructor(label: string) {
    super(`Could not load the values of ${label}`)
    this.name = "VariableLoadError"
  }
}

export type VariableOptions = Record<string, string[] | undefined>

export function sortSelections(selections: Selections): Selections {
  const out: Selections = {}
  for (const name of Object.keys(selections).sort()) {
    out[name] = selections[name]
  }
  return out
}

export function earlierSelections(
  variables: ControlVariable[],
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

export type ValuesData = { values: string[]; truncated: boolean }

export type LoadedValues = { data: ValuesData; parentKey: string }

export type VariableDisplay = {
  status: "loading" | "refreshing" | "ready" | "error"
  shown: ValuesData | undefined
  selectable: string[] | undefined
}

export function parentKeyOf(selections: Selections): string {
  return JSON.stringify(selections)
}

export function variableDisplay(
  variable: ControlVariable,
  request: {
    data: ValuesData | undefined
    last: LoadedValues | undefined
    parentKey: string
    isError: boolean
    isFetching: boolean
  }
): VariableDisplay {
  if (variable.type === "custom") {
    return {
      status: "ready",
      shown: { values: variable.options, truncated: false },
      selectable: variable.options,
    }
  }
  const { last } = request
  const lastValues =
    last?.parentKey === request.parentKey ? last.data.values : undefined
  if (request.isError && !request.isFetching) {
    return {
      status: "error",
      shown: request.data,
      selectable: request.data?.values ?? lastValues,
    }
  }
  if (request.data) {
    return {
      status: "ready",
      shown: request.data,
      selectable: request.data.values,
    }
  }
  if (last && request.isFetching) {
    return { status: "refreshing", shown: last.data, selectable: lastValues }
  }
  return { status: "loading", shown: undefined, selectable: undefined }
}

function fromUrl(variable: ControlVariable, values: string[]): string[] {
  if (variable.includeAll && values.includes(ALL)) return [ALL]
  const concrete = values.filter((value) => value !== ALL)
  if (concrete.length === 0) return []
  return variable.multi ? concrete : [concrete[0]]
}

export function resolveSelections(
  variables: ControlVariable[],
  urlVars: Record<string, string[]>,
  options: VariableOptions
): { selections: Selections; pending: boolean; waiting: string[] } {
  const selections: Selections = {}
  const waiting: string[] = []
  for (const variable of variables) {
    const explicit = fromUrl(variable, urlVars[variable.name] ?? [])
    if (explicit.length > 0) {
      selections[variable.name] = explicit
    } else if (variable.includeAll) {
      selections[variable.name] = [ALL]
    } else {
      const available = options[variable.name]
      if (available === undefined) waiting.push(variable.name)
      else if (available.length > 0) selections[variable.name] = [available[0]]
    }
  }
  return { selections, pending: waiting.length > 0, waiting }
}

export function applyVariableChange(
  variables: ControlVariable[],
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
